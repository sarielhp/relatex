package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// A header cell whose first glyph the fallbacks could not find (a small capital "A" the owners
// spell "a") has its other glyphs placed on a prose word of the same text. The block's order puts
// the glyphs of the cell on the table's own, the one that ends the cell's run included: the
// correction takes the glyph that follows the cell's last in the block, not only as many glyphs as
// the fallbacks had bound (books/0006's "Approximate"). A glyph of the cell that the block spells
// farther on (the "3" of a formula "y(3)" the cell's text does not hold) stays out.
func TestSequencePlaceGivesTheCellItsLastGlyph(t *testing.T) {
	cells := map[int]string{1: "head", 2: "data"}
	var glyphs []nativeGlyph
	for index, text := range []string{"A", "p", "p", "e", "y", "3"} {
		glyphs = append(glyphs, fragmentGlyph(float64(index)*6, 0, text, "head", 1))
	}
	points := []ownerPoint{
		{0, 10, "A"}, {6, 10, "p"}, {12, 10, "p"}, {18, 10, "e"}, {24, 10, "1"}, {30, 10, "3"},
	}
	for index := range 10 {
		glyphs = append(glyphs, fragmentGlyph(float64(index)*6, 20, "5", "data", 2))
		points = append(points, ownerPoint{float64(index) * 6, 30, "5"})
	}
	points = append(points, ownerPoint{100, 50, "p"}, ownerPoint{106, 50, "p"}, ownerPoint{112, 50, "e"})
	page := ownersAt(points...)
	for at := 16; at < 19; at++ {
		page.Leaf[at] = 2 // the prose word
	}
	owners := &glyphowner.Owners{Leaves: []string{"block", "prose"}, Pages: []glyphowner.Page{page}}
	placed := map[int]string{16: "head", 17: "head", 18: "head"}
	fix := sequenceFix{owners: owners, block: "block", glyphs: glyphs, cells: cells, page: newPositionIndex(page),
		lost: map[string]bool{"head": true}}
	placeSequence(placed, fix)
	want := map[int]string{0: "head", 1: "head", 2: "head", 3: "head"}
	if len(placed) != len(want) {
		t.Fatalf("glyphs bound %v, want %v", placed, want)
	}
	for at, leaf := range want {
		if placed[at] != leaf {
			t.Errorf("glyph %d in %q, want %q: %v", at, placed[at], leaf, placed)
		}
	}
}
