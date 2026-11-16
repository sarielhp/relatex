package pdftag

import (
	"math"
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// A header cell set at 60 degrees (a landscape table's header under adjustbox) has no placement the
// table's offset and scale can follow, and its digits, split into runs by a citation's link, take a
// data cell's glyphs of the same text while the data cell's digits take the header's. The block's
// glyphs in TeX's order give each digit its own cell (papers5 1187).
func TestSequencePlaceKeepsRotatedHeaderDigitsInTheirCells(t *testing.T) {
	cells := map[int]string{1: "header", 2: "data"}
	header := []string{"A", "[", "5", "1", "]"}
	angle := math.Pi / 3
	var glyphs []nativeGlyph
	var points []ownerPoint
	for index, text := range header {
		glyphs = append(glyphs, fragmentGlyph(float64(index)*4, 0, text, "header", 1))
		points = append(points, ownerPoint{
			300 + float64(index)*4*math.Cos(angle),
			100 - float64(index)*4*math.Sin(angle),
			text,
		})
	}
	for index, text := range []string{"5", "1"} {
		glyphs = append(glyphs, fragmentGlyph(float64(index)*6, 20, text, "data", 2))
		points = append(points, ownerPoint{100 + float64(index)*6, 300, text})
	}
	owners := &glyphowner.Owners{Leaves: []string{"block"}, Pages: []glyphowner.Page{ownersAt(points...)}}
	page := newPositionIndex(owners.Pages[0])
	// What the table's geometry left: both cells unplaced, then the fallbacks swapped the digits of
	// one cell with the other's (header "5", "1" on the data's owners, data "5", "1" on the
	// header's) while each cell's other glyphs stayed its own.
	placed := map[int]string{
		0: "header", 1: "header", 4: "header",
		5: "header", 6: "header",
		2: "data", 3: "data",
	}
	fix := sequenceFix{owners: owners, block: "block", glyphs: glyphs, cells: cells, page: page,
		lost: map[string]bool{"header": true, "data": true}}
	placeSequence(placed, fix)
	for at, leaf := range map[int]string{
		0: "header", 1: "header", 2: "header", 3: "header", 4: "header",
		5: "data", 6: "data",
	} {
		if placed[at] != leaf {
			t.Errorf("glyph %d in %q, want %q: %v", at, placed[at], leaf, placed)
		}
	}
}

// Two cells of one repeated label (papers3 0615's "Total3D") are each set with the label's glyphs of
// a different occurrence: the fragment's placement read each cell's text already, though not the
// block's own glyphs of the cell. Correcting them would only rebind glyphs and change how many the
// cells hold, so where every lost cell reads its text the sequence leaves them alone.
func TestSequencePlaceLeavesCellsThatAlreadyReadTheirText(t *testing.T) {
	cells := map[int]string{1: "first", 2: "second"}
	label := []string{"T", "o", "t", "a", "l", "3", "D"}
	var glyphs []nativeGlyph
	var points []ownerPoint
	for run := range 3 {
		for index, text := range label {
			points = append(points, ownerPoint{float64(run*100 + index*6), float64(run * 20), text})
		}
	}
	for index, text := range label {
		glyphs = append(glyphs, fragmentGlyph(float64(index)*6, 40, text, "label", 1))
	}
	for index, text := range label {
		glyphs = append(glyphs, fragmentGlyph(float64(index)*6, 60, text, "label", 2))
	}
	owners := &glyphowner.Owners{Leaves: []string{"block"}, Pages: []glyphowner.Page{ownersAt(points...)}}
	page := newPositionIndex(owners.Pages[0])
	placed := map[int]string{}
	for at := range 7 {
		placed[at] = "second"   // the first occurrence, read as cell 2
		placed[at+14] = "first" // the third occurrence, read as cell 1
	}
	want := map[int]string{}
	for at, leaf := range placed {
		want[at] = leaf
	}
	fix := sequenceFix{owners: owners, block: "block", glyphs: glyphs, cells: cells, page: page,
		lost: map[string]bool{"first": true, "second": true}}
	placeSequence(placed, fix)
	if len(placed) != len(want) {
		t.Fatalf("glyphs bound %d, want %d: %v", len(placed), len(want), placed)
	}
	for at, leaf := range want {
		if placed[at] != leaf {
			t.Errorf("glyph %d in %q, want %q unchanged: %v", at, placed[at], leaf, placed)
		}
	}
}

// The block's order spells less of a cell than the fallbacks bound there (a cell whose TeX glyphs
// are not all in the block, or a repeated label with more occurrences than the block has): the
// sequence's binding would leave that cell with fewer glyphs, so the original bindings are
// restored whole rather than dropping glyphs.
func TestSequencePlaceKeepsTheBindingsWhenTheMatchIsShort(t *testing.T) {
	cells := map[int]string{1: "header", 2: "data"}
	// The header's "[" is not in the block; the data cell's nine digits are.
	glyphs := []nativeGlyph{
		fragmentGlyph(0, 0, "A", "header", 1),
		fragmentGlyph(4, 0, "[", "header", 1),
	}
	for index, digit := range "123456789" {
		glyphs = append(glyphs, fragmentGlyph(float64(index)*6, 20, string(digit), "data", 2))
	}
	texts := []string{"A", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	points := make([]ownerPoint, len(texts))
	for index, text := range texts {
		points[index] = ownerPoint{float64(index) * 6, float64(index % 2 * 20), text}
	}
	owners := &glyphowner.Owners{Leaves: []string{"block"}, Pages: []glyphowner.Page{ownersAt(points...)}}
	page := newPositionIndex(owners.Pages[0])
	// The fallbacks gave the header the block's first two glyphs ("A", "1", which spell "A1", not
	// the cell's "A[") and the data cell one digit; the block's order has only the header's "A".
	placed := map[int]string{0: "header", 1: "header", 2: "data"}
	want := map[int]string{0: "header", 1: "header", 2: "data"}
	fix := sequenceFix{owners: owners, block: "block", glyphs: glyphs, cells: cells, page: page,
		lost: map[string]bool{"header": true, "data": true}}
	placeSequence(placed, fix)
	if len(placed) != len(want) {
		t.Fatalf("glyphs bound %d, want the original %d: %v", len(placed), len(want), placed)
	}
	for at, leaf := range want {
		if placed[at] != leaf {
			t.Errorf("glyph %d in %q, want %q restored: %v", at, placed[at], leaf, placed)
		}
	}
}
