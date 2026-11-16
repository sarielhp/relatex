package pdftag

import (
	"encoding/json"
	"strconv"
	"testing"
)

// A Link element holds one run of its host's text: the glyphs it binds go from the first it holds
// to the last, even where the annotation's rectangles miss host glyphs between them (a caption's
// lines run wider than the one box drawn over the block). Were those glyphs the host's, the
// reader would read them before or after the Link as a whole, out of place.

// A rectangle over a and b of the first line and over d and e of the second, missing c, holds the
// whole run a..e; the order check reads "abcde".
func TestLinkHoldsTheRunBetweenItsGlyphs(t *testing.T) {
	first := linkLine("abc", 10, 100)
	second := linkLine("de", 36, 88)
	glyphs := append(first, second...)
	tagger := linkTagger(glyphs, sameLeaves(glyphs, "p"), map[string]string{"p": "P"})
	rects := []bounds{box(first, 0, 2), box(second, 0, 2)}
	if insideRect(glyphs[2].baseline, rects[0]) || insideRect(glyphs[2].baseline, rects[1]) {
		t.Fatalf("c lies in the rectangles: %v", rects)
	}
	element := tagger.bindLink(1, rects)
	if element == nil || element.leaf != "p#a0" || element.first != (glyphRef{1, 0}) {
		t.Fatalf("link %+v", element)
	}
	for index, glyph := range glyphs {
		if got := tagger.binding.leafOf[glyphRef{1, index}]; got != "p#a0" {
			t.Errorf("glyph %q %d: leaf %q, want the link", glyph.text, index, got)
		}
	}
	// The P holds no marked content of its own: it reads its text from the Link, "abcde".
	element.mcrs = []mcr{{page: 1, mcid: 0, first: 0}}
	tagger.leafText = leafExpectations([]piece{{leaf: "p", kind: pieceText, text: "abcde"}})
	p := &selem{role: "P", leaf: "p", kids: []*selem{element}}
	var expected []*selem
	collectLeaves(&selem{role: "Document", kids: []*selem{p}}, &expected)
	if len(expected) != 1 || expected[0] != p {
		t.Fatalf("the order check lists %v", expected)
	}
	pdf := func(value int) json.Number { return json.Number(strconv.Itoa(value)) }
	texts := map[int]map[int]readText{1: {0: glyphText(glyphs, newGlyphRoles(glyphs), []int{0, 1, 2, 3, 4})}}
	tree := map[string]any{
		"/S": "/Document",
		"/K": []any{map[string]any{"/Type": "/StructElem", "/S": "/P", "/K": []any{
			map[string]any{"/Type": "/StructElem", "/S": "/Link", "/K": []any{
				map[string]any{"/Type": "/MCR", "/Pg": "6 0 R", "/MCID": pdf(0)},
			}},
		}}},
	}
	reader := &treeReader{file: &pdfFile{}, pageIndex: map[string]int{"6 0 R": 1}, texts: texts}
	reader.walk(tree, 0)
	if len(reader.found) != 1 || reader.found[0].role != "P" || reader.found[0].text.plain != "abcde" {
		t.Fatalf("the P reads back %+v, want %q", reader.found, "abcde")
	}
	check := tagger.compare(reader.found, expected)
	if check.TextLeaves != 1 || check.TextEqual != 1 || check.Failure != "" || len(check.Mismatches) != 0 {
		t.Fatalf("order check %+v", check)
	}
}

// A link over "b c" only, with d outside the rectangle, holds b and c alone: the run ends at the
// last glyph the annotation holds, so the host keeps "a" before the Link and "de" after it.
func TestLinkHoldsOnlyTheRunItCovers(t *testing.T) {
	glyphs := linkLine("abcde", 10, 100)
	tagger := linkTagger(glyphs, sameLeaves(glyphs, "p"), map[string]string{"p": "P"})
	element := tagger.bindLink(1, []bounds{box(glyphs, 1, 2)})
	if element == nil || element.leaf != "p#a0" || element.first != (glyphRef{1, 1}) {
		t.Fatalf("link %+v", element)
	}
	for index := range glyphs {
		want := "p"
		if index == 1 || index == 2 {
			want = "p#a0"
		}
		if got := tagger.binding.leafOf[glyphRef{1, index}]; got != want {
			t.Errorf("glyph %q %d: leaf %q, want %q", glyphs[index].text, index, got, want)
		}
	}
	if kids := tagger.builder.leaves["p"].kids; len(kids) != 1 || kids[0] != element {
		t.Fatalf("the P's kids %v", kids)
	}
}
