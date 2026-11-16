package pdftag

import (
	"encoding/json"
	"slices"
	"strconv"
	"testing"
)

// A page is 612 × 792 bp. A glyph's baseline and a link's rectangle are both in the layout's
// frame: from the page's top left, y down.

// linkLine paints one line at top: a glyph at x, one bp apart; top is its distance from the page's
// top in the layout's frame.
func linkLine(text string, x, top float64) []paintedGlyph {
	var glyphs []paintedGlyph
	for _, r := range text {
		glyphs = append(glyphs, paintedGlyph{text: string(r), baseline: [2]float64{x, top}, advance: 1})
		x++
	}
	return glyphs
}

// box is the link rectangle (in the layout's frame) over the glyphs from first for count: its top
// above the line, its bottom below it, its x edges a little inside the line.
func box(glyphs []paintedGlyph, first, count int) bounds {
	x, top := glyphs[first].baseline[0], glyphs[first].baseline[1]
	return bounds{x - 0.5, top - 6, x + float64(count) - 0.5, top + 6}
}

// quadOf is a box as /QuadPoints writes it: four corners, each x then y, in the annotation's
// unrotated user space (y up), which is the layout's frame turned over the page's height. The
// numbers are json.Number as qpdf's JSON gives them.
func quadOf(b bounds, frame pageFrame) []any {
	number := func(value float64) json.Number { return json.Number(strconv.FormatFloat(value, 'f', -1, 64)) }
	var points []any
	for _, corner := range [][2]float64{{b[0], b[1]}, {b[2], b[1]}, {b[0], b[3]}, {b[2], b[3]}} {
		ux, uy := frame.user(corner[0], corner[1])
		points = append(points, number(ux), number(uy))
	}
	return points
}

// linkTagger is a tagger with one page of glyphs, each bound to the leaf the owners gave it, and
// the leaves the tree holds.
func linkTagger(painted []paintedGlyph, leaves []string, elements map[string]string) *tagger {
	held := map[glyphRef]string{}
	for ordinal, leaf := range leaves {
		if leaf != "" {
			held[glyphRef{1, ordinal}] = leaf
		}
	}
	b := &builder{leaves: map[string]*selem{}}
	for leaf, role := range elements {
		b.leaves[leaf] = &selem{role: role, leaf: leaf}
	}
	return &tagger{builder: b, painted: map[int][]paintedGlyph{1: painted}, binding: &binding{leafOf: held},
		linkCount: map[string]int{}, linkHosts: map[string]*selem{}}
}

// sameLeaves binds every glyph of the line to leaf.
func sameLeaves(glyphs []paintedGlyph, leaf string) []string {
	leaves := make([]string, len(glyphs))
	for index := range leaves {
		leaves[index] = leaf
	}
	return leaves
}

// A link over "[12]" of a paragraph's "see [12] now" becomes a Link kid of the P that holds exactly
// the four glyphs of the citation; the rest of the line stays the P's.
func TestLinkHoldsItsGlyphs(t *testing.T) {
	glyphs := linkLine("see [12] now", 10, 100)
	tagger := linkTagger(glyphs, sameLeaves(glyphs, "p"), map[string]string{"p": "P"})
	element := tagger.bindLink(1, []bounds{box(glyphs, 4, 4)})
	if element == nil || element.role != "Link" || element.leaf != "p#a0" {
		t.Fatalf("link %+v", element)
	}
	if got := tagger.builder.leaves["p"].kids; len(got) != 1 || got[0] != element {
		t.Fatalf("the P's kids %v", got)
	}
	if tagger.builder.leaves["p#a0"] != element {
		t.Fatalf("the link leaf %q is not in the tree", element.leaf)
	}
	for index, glyph := range glyphs {
		want := "p"
		if index >= 4 && index < 8 {
			want = "p#a0"
		}
		if got := tagger.binding.leafOf[glyphRef{1, index}]; got != want {
			t.Errorf("glyph %q %d: leaf %q, want %q", glyph.text, index, got, want)
		}
	}
	// A second link of the same host is the next, #a1. A rectangle away from the text is no link.
	away := bounds{glyphs[9].baseline[0] - 0.5, 40, glyphs[9].baseline[0] + 0.5, 50}
	if next := tagger.bindLink(1, []bounds{away}); next != nil {
		t.Fatalf("a rectangle away from the text holds a glyph: %+v", next)
	}
	second := tagger.bindLink(1, []bounds{box(glyphs, 10, 1)})
	if second == nil || second.leaf != "p#a1" {
		t.Fatalf("second link %+v", second)
	}
}

// A link over a glyph the owners bound to a formula leaves it there and makes no Link: the
// annotation keeps its OBJR alone (linkAnnotations). A link over no glyph of a text leaf is the same.
func TestLinkOverAFormulaIsOBJROnly(t *testing.T) {
	glyphs := linkLine("x=1", 10, 100)
	tagger := linkTagger(glyphs, []string{"p", "f", "f"}, map[string]string{"p": "P", "f": "Formula"})
	if element := tagger.bindLink(1, []bounds{box(glyphs, 1, 2)}); element != nil {
		t.Fatalf("a formula's link holds text: %+v", element)
	}
	if tagger.binding.leafOf[glyphRef{1, 1}] != "f" || len(tagger.builder.leaves["p"].kids) != 0 {
		t.Fatalf("the formula's glyphs moved: %v", tagger.binding.leafOf)
	}
	// The host is the leaf most of the rectangle's glyphs are bound to: here the formula, which is
	// no text leaf, so a paragraph's glyph beside it is not made a link.
	tagger.binding.leafOf[glyphRef{1, 0}] = "f"
	if element := tagger.bindLink(1, []bounds{box(glyphs, 0, 2)}); element != nil {
		t.Fatalf("a formula holds all the glyphs yet a link was made: %+v", element)
	}
}

// A link broken across two lines (two quads) is one Link holding both parts.
func TestLinkQuadsAcrossLines(t *testing.T) {
	first := linkLine("see [12", 10, 100)
	second := linkLine("ab] now", 10, 88)
	glyphs := append(slices.Clone(first), second...)
	tagger := linkTagger(glyphs, sameLeaves(glyphs, "p"), map[string]string{"p": "P"})
	frame := pageFrame{box: [4]float64{0, 0, 612, 792}}
	points := append(quadOf(box(first, 4, 3), frame), quadOf(box(second, 0, 3), frame)...)
	rects := rectangles(map[string]any{"/QuadPoints": points}, frame)
	if len(rects) != 2 {
		t.Fatalf("quads %v", rects)
	}
	element := tagger.bindLink(1, rects)
	if element == nil || element.leaf != "p#a0" {
		t.Fatalf("link %+v", element)
	}
	var held []int
	for index := range glyphs {
		if tagger.binding.leafOf[glyphRef{1, index}] == "p#a0" {
			held = append(held, index)
		}
	}
	if !slices.Equal(held, []int{4, 5, 6, 7, 8, 9}) {
		t.Fatalf("the link holds %v", held)
	}
}

// The order check reads the paragraph with its Link's text in /K order: "see[12]now" for the P, and
// no Link element of its own. collectLeaves lists the same elements the reader finds.
func TestOrderCheckReadsLinkTextInItsHost(t *testing.T) {
	glyphs := linkLine("see [12] now", 10, 100)
	tagger := linkTagger(glyphs, sameLeaves(glyphs, "p"), map[string]string{"p": "P"})
	link := tagger.bindLink(1, []bounds{box(glyphs, 4, 4)})
	tagger.leafText = leafExpectations([]piece{{leaf: "p", kind: pieceText, text: "see [12] now"}})
	// The P's own marked content: "see" and, after the link, "now"; the spaces are artifacts.
	p := &selem{role: "P", leaf: "p", mcrs: []mcr{{page: 1, mcid: 0, first: 0}, {page: 1, mcid: 2, first: 9}}}
	link.mcrs = []mcr{{page: 1, mcid: 1, first: 4}}
	p.kids = []*selem{link}
	var expected []*selem
	collectLeaves(&selem{role: "Document", kids: []*selem{p}}, &expected)
	if len(expected) != 1 || expected[0] != p {
		t.Fatalf("the order check lists %v", expected)
	}
	roles := newGlyphRoles(glyphs)
	texts := map[int]map[int]readText{1: {
		0: glyphText(glyphs, roles, []int{0, 1, 2}),
		1: glyphText(glyphs, roles, []int{4, 5, 6, 7}),
		2: glyphText(glyphs, roles, []int{9, 10, 11}),
	}}
	// The written /K: the P's first mark ("see"), then the Link (its MCID and its OBJR), then the
	// P's rest ("now"). Numbers are read back from qpdf's JSON, so they are json.Number as there.
	pdf := func(value int) json.Number { return json.Number(strconv.Itoa(value)) }
	tree := map[string]any{
		"/S": "/Document",
		"/K": []any{
			map[string]any{"/Type": "/StructElem", "/S": "/P", "/K": []any{
				map[string]any{"/Type": "/MCR", "/Pg": "6 0 R", "/MCID": pdf(0)},
				map[string]any{"/Type": "/StructElem", "/S": "/Link", "/K": []any{
					map[string]any{"/Type": "/MCR", "/Pg": "6 0 R", "/MCID": pdf(1)},
					map[string]any{"/Type": "/OBJR"},
				}},
				map[string]any{"/Type": "/MCR", "/Pg": "6 0 R", "/MCID": pdf(2)},
			}},
		},
	}
	reader := &treeReader{file: &pdfFile{}, pageIndex: map[string]int{"6 0 R": 1}, texts: texts}
	reader.walk(tree, 0)
	if len(reader.found) != 1 || reader.found[0].role != "P" {
		t.Fatalf("read back %+v", reader.found)
	}
	if got := reader.found[0].text.plain; got != "see[12]now" {
		t.Fatalf("the P reads %q, want %q", got, "see[12]now")
	}
	check := tagger.compare(reader.found, expected)
	if check.TextLeaves != 1 || check.TextEqual != 1 || check.Failure != "" || len(check.Mismatches) != 0 {
		t.Fatalf("order check %+v", check)
	}

	// A paragraph whose whole text is its link ("\href{...}{[12]}") has no MCID of its own: my
	// tree lists it all the same, and reads its text from the Link.
	cited := &selem{role: "P", leaf: "q", kids: []*selem{{role: "Link", leaf: "q#a0",
		mcrs: []mcr{{page: 1, mcid: 4, first: 0}}}}}
	var alone []*selem
	collectLeaves(&selem{role: "Document", kids: []*selem{cited}}, &alone)
	if len(alone) != 1 || alone[0] != cited {
		t.Fatalf("a paragraph holding only a link is listed %v", alone)
	}
	onlyLink := map[string]any{
		"/S": "/Document",
		"/K": []any{map[string]any{"/Type": "/StructElem", "/S": "/P", "/K": []any{
			map[string]any{"/Type": "/StructElem", "/S": "/Link", "/K": []any{
				map[string]any{"/Type": "/MCR", "/Pg": "6 0 R", "/MCID": pdf(4)},
			}},
		}}},
	}
	texts[1][4] = glyphText(glyphs, roles, []int{4, 5, 6, 7})
	reader = &treeReader{file: &pdfFile{}, pageIndex: map[string]int{"6 0 R": 1}, texts: texts}
	reader.walk(onlyLink, 0)
	if len(reader.found) != 1 || reader.found[0].role != "P" || reader.found[0].text.plain != "[12]" {
		t.Fatalf("a paragraph holding only a link reads back %+v", reader.found)
	}
}

// A second annotation over a link's text (hyperref may write two over one citation) finds the
// glyphs bound to the first Link: it goes into the Link's host, as before the links held their
// text, not to the end of the document. A Link holding text sorts by its first glyph.
func TestSecondLinkOverLinkedTextGoesToTheHost(t *testing.T) {
	glyphs := linkLine("see [12] now", 10, 100)
	tagger := linkTagger(glyphs, sameLeaves(glyphs, "p"), map[string]string{"p": "P"})
	frame := pageFrame{box: [4]float64{0, 0, 612, 792}}
	first := tagger.bindLink(1, []bounds{box(glyphs, 4, 4)})
	if first == nil || first.first != (glyphRef{1, 4}) {
		t.Fatalf("link %+v", first)
	}
	if again := tagger.bindLink(1, []bounds{box(glyphs, 4, 4)}); again != nil {
		t.Fatalf("a Link holds a Link: %+v", again)
	}
	b := box(glyphs, 4, 4)
	x0, y0 := frame.user(b[0], b[1])
	x1, y1 := frame.user(b[2], b[3])
	number := func(value float64) json.Number { return json.Number(strconv.FormatFloat(value, 'f', -1, 64)) }
	rect := []any{number(x0), number(y0), number(x1), number(y1)}
	root := &selem{role: "Document"}
	second := &selem{role: "Link"}
	(&treeWriter{t: tagger}).placeLink(second, 1, rect, frame, root)
	host := tagger.builder.leaves["p"]
	if len(root.kids) != 0 || len(host.kids) != 2 || host.kids[1] != second || second.first != (glyphRef{1, 4}) {
		t.Fatalf("the second link went to %v (root %v)", host.kids, root.kids)
	}
}
