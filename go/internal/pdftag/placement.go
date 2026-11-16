package pdftag

import (
	"maps"
	"math"

	"github.com/sarielhp/relatex/internal/pdffonts"
)

// A page prepared for marking: its items, its glyph mapping and what its objects belong to.

// prepared is a page interpreted and checked.
type prepared struct {
	number  int
	items   []item
	mapper  *glyphMapper
	glyphs  int
	failure string
	objects map[int]string          // item index → leaf, for images and forms
	codes   map[string]map[int]bool // the codes shown, by font (a composite font's two bytes as one number)
	advance map[string]map[int]bool // those a later glyph is placed after, by font
}

// prepare interprets one page and maps its glyphs to the layout's.
func (t *tagger) prepare(number int, p page) *prepared {
	result := &prepared{number: number, objects: map[int]string{}}
	data, err := t.file.contentData(p)
	var paints tracePaints
	if err == nil {
		var operations []operation
		if operations, err = parseContent(data); err == nil {
			in := newInterpreter(t.file, p.resources)
			result.items, err = in.run(operations)
			result.glyphs, paints = in.glyphs, in.trace
			result.codes, result.advance = in.codes, in.advanced
		}
	}
	if err == nil {
		result.mapper, err = mapPaints(result.items, t.painted[number], t.file.frameOf(p), paints)
	}
	if err != nil {
		result.failure = err.Error()
	}
	return result
}

// usedCodes are the codes every page shows, by font, with those a later glyph is placed
// after. When every page was interpreted, every show operator of the pages and of their forms is
// rewritten (compensate), so a width change is compensated where the glyph after it is placed.
func (t *tagger) usedCodes() map[string]pdffonts.Use {
	used := map[string]pdffonts.Use{}
	compensated := true
	for _, page := range t.prepared {
		compensated = compensated && page.items != nil
	}
	for _, page := range t.prepared {
		for font, codes := range page.codes {
			use, ok := used[font]
			if !ok {
				use = pdffonts.Use{Codes: map[int]bool{}, Advanced: map[int]bool{}, Compensated: compensated}
				used[font] = use
			}
			maps.Copy(use.Codes, codes)
			maps.Copy(use.Advanced, page.advance[font])
		}
	}
	return used
}

// leafOf is the leaf of a content ordinal's glyph, "" for an artifact.
func (t *tagger) leafOf(page *prepared, ordinal int) string {
	if page.failure != "" || ordinal >= len(page.mapper.mapping) || page.mapper.mapping[ordinal] < 0 {
		return ""
	}
	return t.binding.leafOf[glyphRef{page.number, page.mapper.mapping[ordinal]}]
}

// pageSpans are the content-ordinal ranges of the leaves whose paths are theirs (formulas,
// labels, figures).
func (t *tagger) pageSpans(page *prepared, leafOf func(int) string) []span {
	var spans []span
	index := map[string]int{}
	for ordinal := range page.glyphs {
		leaf := leafOf(ordinal)
		if leaf == "" || !t.ownsPaths(leaf) {
			continue
		}
		at, seen := index[leaf]
		if !seen {
			index[leaf] = len(spans)
			spans = append(spans, span{leaf: leaf, first: ordinal, last: ordinal})
			continue
		}
		spans[at].last = ordinal
	}
	return spans
}

// placeImages gives each image of the model the first unclaimed image or form placement whose
// size agrees within 1 bp, on the page its block was bound on first, then on the nearest page. A
// drawn picture has no such placement: its box places it (pictures.go).
func placeImages(pages []*prepared, images []imagePiece, drawn map[string]bool) {
	for _, image := range images {
		if image.size[0] <= 0 || drawn[image.leaf] {
			continue
		}
		for _, page := range pagesByDistance(pages, image.page) {
			if index, ok := page.claim(image.size); ok {
				page.objects[index] = image.leaf
				break
			}
		}
	}
}

// pagesByDistance orders the pages by their distance from a page.
func pagesByDistance(pages []*prepared, number int) []*prepared {
	ordered := make([]*prepared, 0, len(pages))
	for distance := 0; len(ordered) < len(pages) && distance <= len(pages); distance++ {
		for _, candidate := range []int{number - distance, number + distance} {
			if candidate >= 1 && candidate <= len(pages) && (distance > 0 || candidate == number) {
				ordered = append(ordered, pages[candidate-1])
			}
		}
	}
	return ordered
}

// claim finds an unclaimed image or form placement of the size.
func (page *prepared) claim(size [2]float64) (int, bool) {
	if page.failure != "" {
		return 0, false
	}
	for index, entry := range page.items {
		if _, taken := page.objects[index]; taken || entry.kind != kindObject {
			continue
		}
		if entry.xobject != "Image" && entry.xobject != "Form" {
			continue
		}
		if math.Abs(size[0]-entry.size[0]) <= 1 && math.Abs(size[1]-entry.size[1]) <= 1 {
			return index, true
		}
	}
	return 0, false
}

// formObjects gives a form XObject that paints glyphs, and is no image, to the leaf most of its
// glyphs belong to.
func (t *tagger) formObjects(page *prepared, leafOf func(int) string) {
	for index, entry := range page.items {
		if _, placed := page.objects[index]; placed || entry.kind != kindObject || entry.count == 0 {
			continue
		}
		votes := map[string]int{}
		best := ""
		for ordinal := entry.first; ordinal < entry.first+entry.count; ordinal++ {
			leaf := leafOf(ordinal)
			votes[leaf]++
			if votes[leaf] > votes[best] {
				best = leaf
			}
		}
		page.objects[index] = best
	}
}

// tagPage rewrites a prepared page with its marked content.
func (t *tagger) tagPage(page *prepared) pageResult {
	result := pageResult{glyphs: page.glyphs, failure: page.failure}
	if page.mapper != nil {
		result.checked = page.mapper.checked
	}
	if page.items == nil {
		return result
	}
	tags := &pageTags{number: page.number, roleOf: t.roleOf, objects: page.objects, moved: t.moved,
		drawn: t.drawn, owns: t.ownsPaths, leafOf: func(ordinal int) string { return t.leafOf(page, ordinal) },
		stretched: t.stretched}
	if page.failure == "" {
		tags.spelled = func(ordinal int) string { return t.spelledAt(page, ordinal) }
		tags.pictures = t.pagePictures(page.number, page.mapper.frame)
		tags.leafOf = func(ordinal int) string {
			if leaf := t.glyphPicture(page, tags.pictures, ordinal); leaf != "" {
				return leaf
			}
			return t.leafOf(page, ordinal)
		}
		tags.spans = t.pageSpans(page, tags.leafOf)
		t.formObjects(page, tags.leafOf)
	}
	result.content, result.marks = rewrite(page.items, tags)
	return result
}
