package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// The fragment's baselines are TeX's own, but a math cell's painted glyphs do not always sit where
// TeX recorded them: the recorder keeps the math list's positions, the page applies the italic
// corrections and subscript metrics, and a letter of a math cell lies up to 0.44 bp from its
// baseline (papers5 1370: TD "1" read "21"). The owners' matching allows ownerTolerance between
// TeX's origin and the painted one; a table's placement must allow the same.

// driftedFragment is cellGrid's 2×2 fragment with a "N" and a "2" whose cells are math.
func driftedFragment() nativeFragment {
	glyph := func(x, y float64, text string, cell int) nativeGlyph {
		return nativeGlyph{Baseline: [2]float64{x * spPerBP, y * spPerBP}, Text: text, Cell: &cell}
	}
	return nativeFragment{Page: 1, Glyphs: []nativeGlyph{
		glyph(0, 10, "a", 7), glyph(6, 10, "N", 8), glyph(40, 10, "c", 7),
		glyph(0, 22, "1", 9), glyph(40, 22, "2", 10),
	}}
}

// driftedPage puts the fragment's glyphs at (x, y) bp from the table's reference point, shifted:
// TeX's math metrics move the painted "N" and "2" right, within the tolerance.
func driftedPage(shift map[string]float64) glyphowner.Page {
	page := glyphowner.Page{Number: 1}
	positions := [][2]float64{{0, 10}, {6, 10}, {40, 10}, {0, 22}, {40, 22}}
	texts := []string{"a", "N", "c", "1", "2"}
	for index, text := range texts {
		page.X = append(page.X, round2(positions[index][0]+72+shift[text]))
		page.Y = append(page.Y, round2(positions[index][1]+72))
		page.Text = append(page.Text, text)
		page.Leaf = append(page.Leaf, 1)
	}
	return page
}

// A fragment whose math glyphs lie 0.4 bp from their recorded baselines still places: each glyph
// takes its own cell, so a cell reads its own text and not a neighbour's. (The block keeps no
// fragments, so the sequence fallback cannot place the glyphs instead.)
func TestTableCellsAllowTheOwnersTolerance(t *testing.T) {
	grid := cellGrid()
	grid.NativeFragments = []nativeFragment{driftedFragment()}
	page := driftedPage(map[string]float64{"N": 0.4, "2": 0.3})
	tagger := &tagger{builder: newBuilder(&document{})}
	tagger.builder.table(grid)
	report := TableCells{}
	cells := tagger.tableCells(&glyphowner.Owners{Leaves: []string{"p"}, Pages: []glyphowner.Page{page}}, &report)
	if report.Fragments != 1 || report.Unplaced != 0 || len(cells[1]) != 5 {
		t.Fatalf("report %+v, cells %v", report, cells[1])
	}
	want := map[int]string{0: "h1", 1: "h2", 2: "h1", 3: "b1", 4: "b2"}
	for at, leaf := range want {
		if cells[1][at] != leaf {
			t.Errorf("glyph %d in %q, want %q", at, cells[1][at], leaf)
		}
	}
}

// find takes the owners' glyph of the glyph's text within the owners' tolerance, and none beyond
// it: a table's glyph must not take a neighbour's glyph 0.6 bp away.
func TestTableCellsBeyondTheOwnersTolerance(t *testing.T) {
	page := newPositionIndex(driftedPage(map[string]float64{}))
	at := placement{scale: [2]float64{1, 1}, offset: [2]float64{72, 72}}
	for _, shift := range []float64{0, 0.4} {
		glyph := nativeGlyph{Baseline: [2]float64{6 + shift, 10}, Text: "N"}
		if found, same := page.find(glyph, at, nil); found != 1 || !same {
			t.Errorf("glyph %.1f bp off: found %d same %v", shift, found, same)
		}
	}
	glyph := nativeGlyph{Baseline: [2]float64{6.6, 10}, Text: "N"}
	if found, same := page.find(glyph, at, nil); same {
		t.Errorf("glyph 0.6 bp off took the owners' %d", found)
	}
}

// The 90% match of a placement stands: a fragment two of whose ten glyphs lie beyond the tolerance
// misses the owners and is no placement, however near the rest are.
func TestTableCellsPlacementStillNeedsNinetyPercent(t *testing.T) {
	owners := glyphowner.Page{Number: 1}
	for index := range 10 {
		owners.X = append(owners.X, 72+float64(index)*6)
		owners.Y = append(owners.Y, 82)
		owners.Text = append(owners.Text, string(rune('a'+index)))
	}
	var glyphs []nativeGlyph
	for index := range 10 {
		glyphs = append(glyphs, nativeGlyph{Baseline: [2]float64{float64(index) * 6, 10}, Text: string(rune('a' + index))})
	}
	at := placement{scale: [2]float64{1, 1}, offset: [2]float64{72, 72}}
	page := newPositionIndex(owners)
	if !page.covers(glyphs, at, nil) {
		t.Error("placement of ten glyphs on their owners did not cover")
	}
	for index := 8; index < 10; index++ {
		glyphs[index].Baseline[0] += 0.9
	}
	if page.covers(glyphs, at, nil) {
		t.Error("placement with two missed glyphs of ten covered")
	}
}

// Two owners' glyphs of the same text within the tolerance: find takes the nearer one, not the one
// the bucket lists first (overprinted digits must not trade cells).
func TestTableCellsFindTakesTheNearestSpelledGlyph(t *testing.T) {
	owners := glyphowner.Page{Number: 1, X: []float64{78, 78.45}, Y: []float64{82, 82}, Text: []string{"N", "N"}}
	page := newPositionIndex(owners)
	at := placement{scale: [2]float64{1, 1}, offset: [2]float64{72, 72}}
	glyph := nativeGlyph{Baseline: [2]float64{6.4, 10}, Text: "N"}
	if found, same := page.find(glyph, at, nil); found != 1 || !same {
		t.Errorf("found %d same %v, want the nearer owners' glyph 1", found, same)
	}
}
