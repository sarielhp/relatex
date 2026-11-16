package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// The glyphs a fragment's own placement leaves over are placed by their cell and run (cellruns.go),
// as the tables of the papers these tests come from show.

// ownerPoint is an owners' glyph at bp on the page.
type ownerPoint struct {
	x, y float64
	text string
}

func ownersAt(points ...ownerPoint) glyphowner.Page {
	page := glyphowner.Page{Number: 1}
	for _, point := range points {
		page.X = append(page.X, point.x)
		page.Y = append(page.Y, point.y)
		page.Text = append(page.Text, point.text)
		page.Leaf = append(page.Leaf, 1)
	}
	return page
}

// fragmentGlyph is a fragment glyph at bp in a source cell, its font and text.
func fragmentGlyph(x, y float64, text, font string, cell int) nativeGlyph {
	return nativeGlyph{Baseline: [2]float64{x, y}, Text: text, Font: font, Cell: &cell}
}

// A raised note mark the placement misses (1.2bp off) binds to the owner-less glyph of its
// spelling nearest where the placement puts it, not to the far one or an owned one (papers6/0744's
// author-table stars).
func TestTableCellsBindAMissedNoteMarkToTheNearestOwnerlessGlyph(t *testing.T) {
	cells := map[int]string{1: "c1"}
	var glyphs []nativeGlyph
	var owners []ownerPoint
	for index := range 19 {
		glyphs = append(glyphs, fragmentGlyph(10+float64(index)*6, 20, string(rune('a'+index)), "plain", 1))
		owners = append(owners, ownerPoint{110 + float64(index)*6, 20, string(rune('a' + index))})
	}
	glyphs = append(glyphs, fragmentGlyph(130, 20, "*", "mark", 1))
	owners = append(owners, ownerPoint{230, 21.2, "*"}, ownerPoint{230, 300, "*"}, ownerPoint{231, 20, "*"})
	page := ownersAt(owners...)
	page.Leaf[19], page.Leaf[20] = 0, 0 // the near and the far star have no owner; the third is owned
	placed := map[int]string{}
	if !placeFragment(glyphs, newPositionIndex(page), cells, placed) {
		t.Fatalf("fragment placed nothing: %v", placed)
	}
	if placed[19] != "c1" {
		t.Errorf("near star in %q, want c1: %v", placed[19], placed)
	}
	if _, taken := placed[20]; taken {
		t.Errorf("far star taken: %v", placed)
	}
	if _, taken := placed[21]; taken {
		t.Errorf("owned star taken: %v", placed)
	}
}

// A styled run whose font's metrics set it 0.2bp off the table's placement is placed at its own
// glyphs, not at another occurrence of its text: the comma at the table's offset puts the run
// 0.2bp off, and another "CP" earlier on the page must not take it (papers4 1003).
func TestTableCellsPlaceAStyledRunAtItsOwnGlyphs(t *testing.T) {
	cells := map[int]string{1: "c1", 2: "c2"}
	glyphs := []nativeGlyph{
		fragmentGlyph(27.8, 20, "C", "styled", 1),
		fragmentGlyph(33.5145, 20, "P", "styled", 1),
		fragmentGlyph(38.66, 20, ",", "plain", 1),
	}
	for index := range 19 {
		glyphs = append(glyphs, fragmentGlyph(58+float64(index)*6, 20, string(rune('d'+index)), "plain", 2))
	}
	owners := []ownerPoint{
		{50, 300, "C"}, {55.7145, 300, "P"}, // another "CP" earlier in the page
		{100, 20, "C"}, {105.7145, 20, "P"}, {110.66, 20, ","},
	}
	for index := range 19 {
		owners = append(owners, ownerPoint{130 + float64(index)*6, 20, string(rune('d' + index))})
	}
	placed := map[int]string{}
	if !placeFragment(glyphs, newPositionIndex(ownersAt(owners...)), cells, placed) {
		t.Fatalf("fragment placed nothing: %v", placed)
	}
	if placed[2] != "c1" || placed[3] != "c1" {
		t.Errorf("styled run took %v, want glyphs 2 and 3", placed)
	}
	if _, taken := placed[0]; taken {
		t.Errorf("another CP on the page took a glyph: %v", placed)
	}
	if _, taken := placed[1]; taken {
		t.Errorf("another CP on the page took a glyph: %v", placed)
	}
	if placed[4] != "c1" {
		t.Errorf("comma in %q, want c1: %v", placed[4], placed)
	}
}

// A table whose columns TeX stretched has no placement for the whole fragment, but each cell's own
// glyphs still lie together: they are placed by their cell (papers2 0390).
func TestTableCellsPlaceAStretchedCellsGlyphs(t *testing.T) {
	cells := map[int]string{1: "c1", 2: "c2"}
	glyphs := []nativeGlyph{
		fragmentGlyph(28, 20, "a", "plain", 1),
		fragmentGlyph(34, 20, "b", "plain", 1),
		fragmentGlyph(98, 20, "c", "plain", 2),
		fragmentGlyph(104, 20, "d", "plain", 2),
	}
	owners := ownersAt(
		ownerPoint{100, 20, "a"}, ownerPoint{106, 20, "b"},
		ownerPoint{200, 20, "c"}, ownerPoint{206, 20, "d"},
	)
	placed := map[int]string{}
	if !placeFragment(glyphs, newPositionIndex(owners), cells, placed) {
		t.Fatalf("fragment placed nothing: %v", placed)
	}
	for at, leaf := range map[int]string{0: "c1", 1: "c1", 2: "c2", 3: "c2"} {
		if placed[at] != leaf {
			t.Errorf("glyph %d in %q, want %q: %v", at, placed[at], leaf, placed)
		}
	}
}

// A cell's run set sideways is placed by its own turn, the rest of the table by theirs: the whole
// fragment has no one placement (papers4 0942).
func TestTableCellsPlaceASidewaysRun(t *testing.T) {
	cells := map[int]string{1: "c1", 2: "c2"}
	glyphs := []nativeGlyph{
		fragmentGlyph(28, 20, "a", "plain", 1),
		fragmentGlyph(34, 20, "b", "plain", 1),
		fragmentGlyph(40, 20, "c", "plain", 1),
		fragmentGlyph(28, 200, "d", "plain", 2),
		fragmentGlyph(34, 200, "e", "plain", 2),
	}
	owners := ownersAt(
		ownerPoint{100, 20, "a"}, ownerPoint{100, 26, "b"}, ownerPoint{100, 32, "c"},
		ownerPoint{200, 200, "d"}, ownerPoint{206, 200, "e"},
	)
	placed := map[int]string{}
	if !placeFragment(glyphs, newPositionIndex(owners), cells, placed) {
		t.Fatalf("fragment placed nothing: %v", placed)
	}
	for at, leaf := range map[int]string{0: "c1", 1: "c1", 2: "c1", 3: "c2", 4: "c2"} {
		if placed[at] != leaf {
			t.Errorf("glyph %d in %q, want %q: %v", at, placed[at], leaf, placed)
		}
	}
}

// A table in \resizebox whose run lies on one baseline has no span in y: the table's scale is the
// x one, and the whole run is placed by it (papers5 1246).
func TestTableCellsPlaceARunScaledOnOneBaseline(t *testing.T) {
	cells := map[int]string{1: "c1"}
	glyphs := []nativeGlyph{
		fragmentGlyph(0, 20, "d", "plain", 1),
		fragmentGlyph(6, 20, "d", "plain", 1),
		fragmentGlyph(12, 20, "p", "plain", 1),
		fragmentGlyph(18, 20, "a", "plain", 1),
		fragmentGlyph(24, 20, "e", "plain", 1),
	}
	owners := ownersAt(
		ownerPoint{100, 20, "d"}, ownerPoint{104.98, 20, "d"}, ownerPoint{109.96, 20, "p"},
		ownerPoint{114.94, 20, "a"}, ownerPoint{119.92, 20, "e"},
	)
	placed := map[int]string{}
	if !placeFragment(glyphs, newPositionIndex(owners), cells, placed) {
		t.Fatalf("fragment placed nothing: %v", placed)
	}
	for at := range 5 {
		if placed[at] != "c1" {
			t.Errorf("glyph %d in %q, want c1: %v", at, placed[at], placed)
		}
	}
}

// A cell's glyphs set alone in a font of their own have no span either: they are placed where the
// cell's own run (the "(gcm" beside them) put the cell, not by their text anywhere on the page
// (papers2 0390's "()").
func TestTableCellsPlaceASingleGlyphRunOfAPlacedCell(t *testing.T) {
	cells := map[int]string{1: "c1"}
	glyphs := []nativeGlyph{
		fragmentGlyph(28, 20, "(", "plain", 1),
		fragmentGlyph(34, 20, "g", "math", 1),
		fragmentGlyph(40, 20, "c", "math", 1),
		fragmentGlyph(46, 20, "m", "math", 1),
		fragmentGlyph(52, 20, ")", "plain", 1),
		fragmentGlyph(58, 17, "-", "super", 1), // 0.3bp off: the whole cell has no placement
	}
	owners := ownersAt(
		ownerPoint{100, 20, "("}, ownerPoint{106, 20, "g"}, ownerPoint{112, 20, "c"},
		ownerPoint{118, 20, "m"}, ownerPoint{124, 20, ")"}, ownerPoint{130.3, 89, "-"},
	)
	placed := map[int]string{}
	if !placeFragment(glyphs, newPositionIndex(owners), cells, placed) {
		t.Fatalf("fragment placed nothing: %v", placed)
	}
	for at, leaf := range map[int]string{0: "c1", 1: "c1", 2: "c1", 3: "c1", 4: "c1"} {
		if placed[at] != leaf {
			t.Errorf("glyph %d in %q, want %q: %v", at, placed[at], leaf, placed)
		}
	}
	if _, taken := placed[5]; taken {
		t.Errorf("the superscript was taken by the cell: %v", placed)
	}
}

// A glyph set alone in a cell with no placed run has no position to be placed by: any occurrence
// of its text would do, so it stays unplaced (the caution of cellruns.go).
func TestTableCellsLeaveALoneGlyphOfAnUnplacedCell(t *testing.T) {
	cells := map[int]string{1: "c1", 2: "c2"}
	glyphs := []nativeGlyph{
		fragmentGlyph(28, 20, "a", "plain", 1),
		fragmentGlyph(34, 20, "b", "plain", 1),
		fragmentGlyph(0, 50, "x", "plain", 2),
	}
	owners := ownersAt(
		ownerPoint{100, 20, "a"}, ownerPoint{106, 20, "b"}, ownerPoint{300, 120, "x"},
	)
	placed := map[int]string{}
	placeFragment(glyphs, newPositionIndex(owners), cells, placed)
	if _, taken := placed[2]; taken {
		t.Errorf("a lone glyph took an occurrence of its text: %v", placed)
	}
}

// A fragment must not take the glyphs another table's cell already has: the same figures in two
// fragments of equal text keep each cell's own (papers3 0829, papers 0166).
func TestTableCellsKeepAnotherFragmentsGlyphs(t *testing.T) {
	first := []nativeGlyph{
		fragmentGlyph(28, 20, "1", "plain", 1),
		fragmentGlyph(34, 20, "2", "plain", 1),
	}
	second := []nativeGlyph{
		fragmentGlyph(28, 20, "1", "plain", 2),
		fragmentGlyph(34, 20, "2", "plain", 2),
	}
	owners := newPositionIndex(ownersAt(ownerPoint{100, 20, "1"}, ownerPoint{106, 20, "2"}))
	placed := map[int]string{}
	if !placeFragment(first, owners, map[int]string{1: "c1"}, placed) {
		t.Fatalf("first fragment placed nothing: %v", placed)
	}
	placeFragment(second, owners, map[int]string{2: "c2"}, placed)
	for at := range 2 {
		if placed[at] != "c1" {
			t.Errorf("glyph %d taken by %q: %v", at, placed[at], placed)
		}
	}
}
