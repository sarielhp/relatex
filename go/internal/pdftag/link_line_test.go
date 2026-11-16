package pdftag

import (
	"slices"
	"testing"
)

// A one-line link rectangle marks the line whose baseline sits near its bottom: glyphs of another
// line that overlap the rectangle vertically are not its text, even when they are more numerous.
// Two table cells overlap here: leaf b's citation at 379.8 is the line the box marks, while leaf
// a's lines at 373.2 and 383.1 (inside the box's +2 slack) are other cells' text.
func TestLinkMarksTheLineItsRectangleCovers(t *testing.T) {
	above := linkLine("ained o", 10, 373.2)
	line := linkLine("Pavlou", 10, 379.8)
	below := linkLine("xpand", 10, 383.1)
	glyphs := append(append(slices.Clone(above), line...), below...)
	leaves := make([]string, 0, len(glyphs))
	for range above {
		leaves = append(leaves, "a")
	}
	for range line {
		leaves = append(leaves, "b")
	}
	for range below {
		leaves = append(leaves, "a")
	}
	tagger := linkTagger(glyphs, leaves, map[string]string{"a": "P", "b": "TD"})
	rect := bounds{9.5, 372.74, 15.5, 382.75}
	element := tagger.bindLink(1, []bounds{rect})
	if element == nil || element.leaf != "b#a0" {
		t.Fatalf("link %+v, want the leaf of the line the box covers", element)
	}
	for index, glyph := range glyphs {
		want := "a"
		if index >= len(above) && index < len(above)+len(line) {
			want = "b#a0"
		}
		if got := tagger.binding.leafOf[glyphRef{1, index}]; got != want {
			t.Errorf("glyph %q %d: leaf %q, want %q", glyph.text, index, got, want)
		}
	}
}

// A box whose bottom stops just above the text's baseline holds no baseline itself: it is no
// one-line box and keeps the line it sits above, in its +2 slack (the case the slack exists for).
func TestLinkKeepsTheLineItsBoxSitsAbove(t *testing.T) {
	glyphs := linkLine("Pavlou", 10, 100)
	tagger := linkTagger(glyphs, sameLeaves(glyphs, "p"), map[string]string{"p": "P"})
	element := tagger.bindLink(1, []bounds{{9.5, 92, 15.5, 99}})
	if element == nil || element.leaf != "p#a0" || element.first != (glyphRef{1, 0}) {
		t.Fatalf("link %+v, want the line in the box's slack", element)
	}
	for index, glyph := range glyphs {
		if got := tagger.binding.leafOf[glyphRef{1, index}]; got != "p#a0" {
			t.Errorf("glyph %q %d: leaf %q, want the link", glyph.text, index, got)
		}
	}
}
