package pdftag

import (
	"cmp"
	"slices"
	"strconv"
)

// Links hold their text (decision 121 item 2): a link annotation's glyphs, the citation or URL the
// link marks, become the marked content of the Link element itself, so a reader announces them as
// the link. They are bound to a leaf of the host before the pages are rewritten, so the show
// operators are split there as for any other leaf.

// linkLeaf is the leaf of a host's link: the host's leaf and the link's number.
func linkLeaf(host string, number int) string { return host + "#a" + strconv.Itoa(number) }

// bindLinks binds each link annotation's glyphs to a Link element of the text leaf that holds them.
// It runs after the owners and the labels and before the pages are prepared: the rewritten text
// must hold the link leaves already. A link without glyphs of a text leaf (over a formula or a
// figure, or a Beamer navigation symbol) is left to linkAnnotations, which gives it its OBJR alone.
func (t *tagger) bindLinks() {
	t.linkElements, t.linkCount, t.linkHosts = map[string]*selem{}, map[string]int{}, map[string]*selem{}
	for index, p := range t.file.pages() {
		frame := t.file.frameOf(p)
		for _, value := range t.file.array(p.dict["/Annots"]) {
			ref, _ := value.(string)
			annot := t.file.dict(ref)
			if annot == nil || !isRef(ref) || annot["/Subtype"] != "/Link" {
				continue
			}
			if element := t.bindLink(index+1, rectangles(annot, frame)); element != nil {
				t.linkElements[ref] = element
				t.report.LinksWithText++
			}
		}
	}
}

// bindLink makes the Link element of an annotation whose rectangles hold the glyphs of one text
// leaf, or returns nil when it holds none.
func (t *tagger) bindLink(number int, rects []bounds) *selem {
	held := t.linkGlyphs(number, rects)
	host := t.linkHost(held)
	if host == nil {
		return nil
	}
	run := t.linkRun(number, host.leaf, held[host.leaf])
	element := &selem{role: "Link", leaf: linkLeaf(host.leaf, t.linkCount[host.leaf]),
		first: glyphRef{number, run[0]}}
	t.linkCount[host.leaf]++
	t.builder.leaves[element.leaf] = element
	t.linkHosts[element.leaf] = host
	host.kids = append(host.kids, element)
	for _, ordinal := range run {
		t.binding.leafOf[glyphRef{number, ordinal}] = element.leaf
	}
	return element
}

// linkHost is the text leaf most of the glyphs are bound to; nil when the glyphs name none.
func (t *tagger) linkHost(held map[string][]int) *selem {
	var hosts []string
	for leaf := range held {
		if element := t.builder.leaves[leaf]; element != nil && textRoles[element.role] {
			hosts = append(hosts, leaf)
		}
	}
	slices.SortFunc(hosts, func(a, b string) int {
		if len(held[a]) != len(held[b]) {
			return cmp.Compare(len(held[b]), len(held[a]))
		}
		return cmp.Compare(a, b)
	})
	if len(hosts) == 0 {
		return nil
	}
	return t.builder.leaves[hosts[0]]
}

// linkGlyphs are the page's painted glyphs bound to a structure leaf whose baseline lies on a line
// one of the rectangles marks, by host, then by paint index. Glyphs bound to no leaf, or to one of
// another kind (a Formula, another paragraph), are left alone.
//
// A rectangle marks text on its own line: text of another cell can overlap it vertically, with the
// baseline of a line above it inside the box or of a line below it inside the +2 slack. A rectangle
// that holds a baseline within linkLineDepth of its bottom therefore marks the glyphs of that
// depth alone. When it holds none (a box drawn just above the baseline), it keeps every glyph it
// holds, so the slack still catches the line the box sits above.
func (t *tagger) linkGlyphs(number int, rects []bounds) map[string][]int {
	marks := make([]bool, len(rects))
	for ordinal, glyph := range t.painted[number] {
		if t.binding.leafOf[glyphRef{number, ordinal}] == "" {
			continue
		}
		for index, rect := range rects {
			if onLinkLine(glyph.snapped(rect), rect) {
				marks[index] = true
			}
		}
	}
	held := map[string][]int{}
	for ordinal, glyph := range t.painted[number] {
		leaf := t.binding.leafOf[glyphRef{number, ordinal}]
		if leaf == "" {
			continue
		}
		for index, rect := range rects {
			at := glyph.snapped(rect)
			if !insideRect(at, rect) || (marks[index] && !onLinkLine(at, rect)) {
				continue
			}
			held[leaf] = append(held[leaf], ordinal)
			break
		}
	}
	for _, ordinals := range held {
		slices.Sort(ordinals)
	}
	return held
}

// linkRun is the run of the host's glyphs a Link holds: every glyph bound to the host from the
// first held glyph to the last, in the host's content order — the order its glyphs are read, which
// on a page is their paint order. A Link element holds one run of its host's text, so a glyph of
// the host between two the annotation's rectangles hold is the Link's even when it lies outside
// them (a caption's lines run wider than the single box drawn over the block); were it left to the
// host, its text would be read before or after the Link as a whole.
func (t *tagger) linkRun(number int, leaf string, held []int) []int {
	run := make([]int, 0, held[len(held)-1]-held[0]+1)
	for ordinal := held[0]; ordinal <= held[len(held)-1]; ordinal++ {
		if t.binding.leafOf[glyphRef{number, ordinal}] == leaf {
			run = append(run, ordinal)
		}
	}
	return run
}

// rectangles are an annotation's rectangles in the layout's frame: its quads' boxes when it has
// /QuadPoints (8 numbers each), else its rectangle.
func rectangles(annot map[string]any, frame pageFrame) []bounds {
	var rects []bounds
	quads, _ := annot["/QuadPoints"].([]any)
	for at := 0; at+7 < len(quads); at += 8 {
		box := emptyBounds
		for corner := 0; corner < 4; corner++ {
			box.add(frame.layout(numberOf(quads[at+2*corner]), numberOf(quads[at+2*corner+1])))
		}
		rects = append(rects, box)
	}
	if len(rects) > 0 {
		return rects
	}
	return []bounds{rectBounds(annot["/Rect"], frame)}
}

// rectBounds is a rectangle in the layout's frame, its corners in any order; emptyBounds when it
// is no rectangle.
func rectBounds(value any, frame pageFrame) bounds {
	rect, _ := value.([]any)
	if len(rect) != 4 {
		return emptyBounds
	}
	left, top := frame.layout(numberOf(rect[0]), numberOf(rect[1]))
	right, bottom := frame.layout(numberOf(rect[2]), numberOf(rect[3]))
	return bounds{min(left, right), min(top, bottom), max(left, right), max(top, bottom)}
}

// linkLineDepth is a one-line link rectangle's depth below its text's baseline, as a share of its
// height: the baseline sits near the rectangle's bottom, not at its middle or above it.
const linkLineDepth = 0.35

// onLinkLine reports whether a glyph's baseline lies on the line a rectangle marks: inside it and
// within linkLineDepth of its bottom. A baseline in the +2 slack below the rectangle is not on it.
func onLinkLine(at [2]float64, rect bounds) bool {
	return at[0] >= rect[0] && at[0] <= rect[2] && at[1] >= rect[1] && at[1] <= rect[3] &&
		rect[3]-at[1] <= linkLineDepth*(rect[3]-rect[1])
}

// insideRect reports whether a glyph's baseline lies in one rectangle, within the slack of a link
// whose box stops just above the text's baseline.
func insideRect(at [2]float64, rect bounds) bool {
	return at[0] >= rect[0] && at[0] <= rect[2] && at[1] >= rect[1] && at[1] <= rect[3]+2
}
