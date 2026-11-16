package pdftag

import (
	"strings"
	"testing"
)

// A link broken over lines: its clip text object holds two leaves, each line after a Tm. It is written
// once per leaf, each copy with its own q, state operators, ET and layer fills, so the clips union.
func TestClipTextObjectWithTwoLeavesIsWrittenOncePerLeaf(t *testing.T) {
	items := interpretTest(t, "q 7 Tr BT /F1 10 Tf 1 0 0 1 0 0 Tm [(a)]TJ 1 0 0 1 0 20 Tm [(b)]TJ ET "+
		"/OC /oc1 BDC 0 0 5 5 re f EMC 0 Tr Q")
	content, marks := rewrite(items, clipTags([]string{"l1", "l2"}))
	text := string(content)
	if len(marks) != 2 || marks[0].leaf != "l1" || marks[1].leaf != "l2" {
		t.Fatalf("marks %+v:\n%s", marks, text)
	}
	if strings.Count(text, "BT\n") != 2 || strings.Count(text, "/OC /oc1 BDC") != 2 || strings.Count(text, "q\n7 Tr\n") != 2 {
		t.Fatalf("not written once per leaf:\n%s", text)
	}
	if strings.Count(text, "/F1 10 Tf") != 2 || strings.Contains(text, "BT\n/F1 10 Tf\n1 0 0 1 0 0 Tm\n/Link") {
		t.Fatalf("state operators not repeated, or sequences inside BT:\n%s", text)
	}
}

// A glyph whose origin lies left of a link's box but whose middle lies inside it belongs to the link.
func TestLinkHoldsAGlyphStartingLeftOfItsBox(t *testing.T) {
	glyphs := linkLine("Wei", 10, 100)
	glyphs[0].advance = 4
	tagger := linkTagger(glyphs, sameLeaves(glyphs, "p"), map[string]string{"p": "P"})
	left := glyphs[0].baseline[0]
	element := tagger.bindLink(1, []bounds{{left + 0.3, 92, left + 40, 101}})
	if element == nil || element.first != (glyphRef{1, 0}) {
		t.Fatalf("link %+v, want it to begin at the first glyph", element)
	}
}
