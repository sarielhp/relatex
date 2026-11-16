package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// The owners' glyphs no node owns are the page's furniture (a Beamer head or footline, a running
// header, the page number): a table cell's glyph is the text of a model node, so a fragment's
// leftover run must never be placed on furniture (cellruns.go). slides/0029's header cell read
// "proposizioneproposizion": its run lay on the frame's footline, which no node owned, so the run
// counted the footline as placed and the fragment's block never placed the cell.

// furniture marks the owners' glyphs from index on as owned by no node (glyphowner's leaf 0).
func furniture(page glyphowner.Page, index int) glyphowner.Page {
	for at := index; at < len(page.Leaf); at++ {
		page.Leaf[at] = 0
	}
	return page
}

// unknownAt marks the owners' glyphs whose position TeX does not know: no placement finds them.
func unknownAt(page glyphowner.Page, index ...int) glyphowner.Page {
	page.Unknown = append(page.Unknown, index...)
	return page
}

// A header cell the owners do not place, its run lying by chance on the frame's footline, binds none
// of the footline's glyphs: nothing of the fragment is placed, and its block's glyphs in TeX's order
// place the cell instead (slides/0029).
func TestTableCellsDoNotPlaceARunOnPageFurniture(t *testing.T) {
	cells := map[int]string{1: "c1", 2: "c2"}
	texts := map[int]string{1: "proposizione", 2: "ab"}
	var glyphs []nativeGlyph
	var leaves []string
	var block []int
	// The header cell's glyphs lie on the footline's baseline; the second cell's do not, so the
	// fragment as a whole has no placement (11 of its 14 glyphs at most would meet the footline).
	for _, cell := range []int{1, 2} {
		for offset, letter := range texts[cell] {
			y := 600.0
			if cell == 1 {
				y = 500
			}
			glyphs = append(glyphs, fragmentGlyph(100+float64(offset)*6, y, string(letter), "plain", cell))
			leaves = append(leaves, cells[cell])
			block = append(block, len(block))
		}
	}
	// The block's glyphs are the cells' own, at the positions TeX does not record.
	owners := ownersAt()
	for offset, glyph := range glyphs {
		owners.X = append(owners.X, 300+float64(offset)*6)
		owners.Y = append(owners.Y, 700)
		owners.Text = append(owners.Text, glyph.Text)
		owners.Leaf = append(owners.Leaf, 1)
	}
	// The footline spells the start of the header cell's text on the header cell's baseline.
	footline := len(owners.X)
	for offset, letter := range "proposizionale" {
		owners.X = append(owners.X, 100+float64(offset)*6)
		owners.Y = append(owners.Y, 500)
		owners.Text = append(owners.Text, string(letter))
		owners.Leaf = append(owners.Leaf, 1)
	}
	page := newPositionIndex(furniture(unknownAt(owners, block...), footline))
	placed := map[int]string{}
	if placeFragment(glyphs, page, cells, placed) {
		t.Fatalf("the fragment's run was placed on the footline: %v", placed)
	}
	if len(placed) != 0 {
		t.Errorf("furniture bound to a cell: %v", placed)
	}
	sequence := sequencePlace(glyphs, leaves, block, page.text)
	if sequence == nil {
		t.Fatalf("the block placed nothing after the fragment's run was refused")
	}
	if len(sequence) != len(glyphs) {
		t.Fatalf("the block placed %d of the fragment's %d glyphs: %v", len(sequence), len(glyphs), sequence)
	}
	read := map[string]string{}
	for _, at := range block {
		leaf := sequence[at]
		if leaf != leaves[at] {
			t.Errorf("block glyph %d in %q, want %q: %v", at, leaf, leaves[at], sequence)
		}
		read[leaf] += page.text[at]
	}
	for cell, want := range texts {
		if got := read[cells[cell]]; got != want {
			t.Errorf("cell %d reads %q, want %q", cell, got, want)
		}
	}
}
