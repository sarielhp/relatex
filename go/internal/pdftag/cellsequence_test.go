package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// A fragment whose baselines no placement brings onto the page (each label turned on its side in
// its cell, papers3 0615) is placed by its glyphs' text in TeX's order: the owners' glyphs of the
// block and the fragment's are the same glyphs.
func TestSequencePlace(t *testing.T) {
	glyphs := []nativeGlyph{{Text: "a"}, {Text: "b"}, {Text: "b"}}
	leaves := []string{"c1", "c2", "c2"}
	block := []int{0, 1, 2}
	placed := sequencePlace(glyphs, leaves, block, []string{"a", "b", "b"})
	want := map[int]string{0: "c1", 1: "c2", 2: "c2"}
	if len(placed) != len(want) {
		t.Fatalf("placed %v, want %v", placed, want)
	}
	for at, leaf := range want {
		if placed[at] != leaf {
			t.Fatalf("placed %v, want %v", placed, want)
		}
	}
}

// The block ships a glyph the fragment's cell does not (or the fragment a glyph of none): the
// matching skips it and keeps the order, as long as the fragment's glyphs mostly agree.
func TestSequencePlaceSkipsUnsharedGlyphs(t *testing.T) {
	glyphs := []nativeGlyph{{Text: "a"}, {Text: "b"}, {Text: "c"}}
	leaves := []string{"c1", "c2", "c3"}
	placed := sequencePlace(glyphs, leaves, []int{0, 1, 2, 3}, []string{"a", "x", "b", "c"})
	if len(placed) != 3 || placed[0] != "c1" || placed[2] != "c2" || placed[3] != "c3" {
		t.Fatalf("placed %v", placed)
	}
}

// A fragment whose text is not the block's on the page places nothing: the owners' glyphs there are
// another block's or the page is tagged as artifact, not guessed.
func TestSequencePlaceRefusesOtherText(t *testing.T) {
	glyphs := []nativeGlyph{{Text: "a"}, {Text: "b"}, {Text: "c"}, {Text: "d"}, {Text: "e"}}
	leaves := []string{"c1", "c1", "c2", "c2", "c3"}
	if placed := sequencePlace(glyphs, leaves, []int{0, 1, 2, 3, 4}, []string{"a", "x", "y", "z", "w"}); placed != nil {
		t.Fatalf("placed %v, want nil", placed)
	}
}

// A repeated letter with a block glyph between its copies binds each copy to its own cell: the second
// "a" is the block's third glyph and the second cell's, not the first cell's.
func TestSequencePlaceRepeatedLetters(t *testing.T) {
	glyphs := []nativeGlyph{{Text: "a"}, {Text: "a"}, {Text: "b"}}
	leaves := []string{"c1", "c2", "c2"}
	placed := sequencePlace(glyphs, leaves, []int{0, 1, 2, 3}, []string{"a", "x", "a", "b"})
	if len(placed) != 3 || placed[0] != "c1" || placed[2] != "c2" || placed[3] != "c2" {
		t.Fatalf("placed %v", placed)
	}
	if _, ok := placed[1]; ok {
		t.Fatalf("block glyph x placed: %v", placed)
	}
}

// Near miss: 8 of 10 fragment glyphs agree with the block, under minimumCellMatch, so nothing is
// placed; an empty block places nothing either.
func TestSequencePlaceNearMissRefused(t *testing.T) {
	var glyphs []nativeGlyph
	var leaves []string
	for _, letter := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		glyphs = append(glyphs, nativeGlyph{Text: letter})
		leaves = append(leaves, "c1")
	}
	text := []string{"a", "b", "c", "d", "e", "f", "g", "h", "Y", "Z"}
	block := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	if placed := sequencePlace(glyphs, leaves, block, text); placed != nil {
		t.Fatalf("placed %v, want nil", placed)
	}
	if placed := sequencePlace(glyphs, leaves, nil, text); placed != nil {
		t.Fatalf("empty block placed %v, want nil", placed)
	}
}

// blockGlyphs lists the owners' glyph indices of one block only, in the page's (TeX's) order.
func TestBlockGlyphs(t *testing.T) {
	owners := &glyphowner.Owners{Leaves: []string{"p", "q"}, Pages: []glyphowner.Page{{Number: 2,
		X: []float64{1, 2, 3, 4}, Y: []float64{1, 1, 1, 1}, Text: []string{"a", "b", "c", "d"}, Leaf: []int{1, 2, 1, 0}}}}
	page := owners.Pages[0]
	if got := blockGlyphs(owners, page, "p"); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("block p: %v", got)
	}
	if got := blockGlyphs(owners, page, "q"); len(got) != 1 || got[0] != 1 {
		t.Fatalf("block q: %v", got)
	}
}

// gridBlock is the block a grid's cells were cut from, and none for a grid no block holds.
func TestGridBlock(t *testing.T) {
	b := &builder{cutBlock: map[string]string{"c1": "p", "c2": "p"}}
	if got := gridBlock(b, map[int]string{0: "c1", 1: "c2"}); got != "p" {
		t.Fatalf("grid block %q", got)
	}
	if got := gridBlock(b, map[int]string{0: "d"}); got != "" {
		t.Fatalf("grid block %q", got)
	}
}
