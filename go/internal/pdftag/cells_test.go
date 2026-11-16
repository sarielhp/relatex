package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// spPerBP converts the test's bp positions to the fragments' sp.
const spPerBP = 65536 * 72.27 / 72

// cellGrid is a 2×2 table: header "ab" | "c", body "1" | "2", set by source cells 7 to 10.
func cellGrid() *tableGrid {
	grid := &tableGrid{}
	for index, id := range []string{"h1", "h2", "b1", "b2"} {
		source := 7 + index
		grid.Cells = append(grid.Cells, tableCell{ID: id, Row: index / 2, Column: index % 2, SourceCell: &source})
	}
	return grid
}

// fragmentAt lists the grid's glyphs: "a" "b" in cell 7, "c" in 8, then a body row whose figures
// are given (cells 9 and 10), in the table's own box, its top-left corner at TeX's reference point.
func fragmentAt(page int, body [2]string) nativeFragment {
	glyph := func(x, y float64, text string, cell int) nativeGlyph {
		return nativeGlyph{Baseline: [2]float64{x * spPerBP, y * spPerBP}, Text: text, Cell: &cell}
	}
	return nativeFragment{Page: page, Glyphs: []nativeGlyph{
		glyph(0, 10, "a", 7), glyph(6, 10, "b", 7), glyph(40, 10, "c", 8),
		glyph(0, 22, body[0], 9), glyph(40, 22, body[1], 10),
	}}
}

// ownersPage puts each glyph of the tables at (x, y) bp + offset, scaled about the reference point.
func ownersPage(tables [][]string, offsets [][2]float64, scale float64) glyphowner.Page {
	page := glyphowner.Page{Number: 1}
	positions := [][2]float64{{0, 10}, {6, 10}, {40, 10}, {0, 22}, {40, 22}}
	for table, texts := range tables {
		for index, text := range texts {
			page.X = append(page.X, round2(positions[index][0]*scale+offsets[table][0]))
			page.Y = append(page.Y, round2(positions[index][1]*scale+offsets[table][1]))
			page.Text = append(page.Text, text)
			page.Leaf = append(page.Leaf, 1)
		}
	}
	return page
}

func taggerWith(grid *tableGrid) *tagger {
	b := newBuilder(&document{})
	b.markCut(b.table(grid), "p")
	return &tagger{builder: b}
}

// Two tables of the same columns on one page, the second 40bp lower: by position alone the
// fragment of the second also lies on the first; their figures differ, and it takes its own
// (paper 0668).
func TestTableCellsTakeTheTableOfTheirText(t *testing.T) {
	grid := cellGrid()
	grid.NativeFragments = []nativeFragment{fragmentAt(1, [2]string{"3", "4"})}
	page := ownersPage([][]string{{"a", "b", "c", "1", "2"}, {"a", "b", "c", "3", "4"}}, [][2]float64{{72, 72}, {72, 112}}, 1)
	report := TableCells{}
	cells := taggerWith(grid).tableCells(&glyphowner.Owners{Leaves: []string{"p"}, Pages: []glyphowner.Page{page}}, &report)
	want := map[int]string{5: "h1", 6: "h1", 7: "h2", 8: "b1", 9: "b2"}
	if report.Fragments != 1 || report.Unplaced != 0 || len(cells[1]) != len(want) {
		t.Fatalf("report %+v, cells %v", report, cells[1])
	}
	for at, leaf := range want {
		if cells[1][at] != leaf {
			t.Errorf("glyph %d in %q, want %q", at, cells[1][at], leaf)
		}
	}
}

// A table in \resizebox: its fragment keeps the unscaled box, the page shows it at 0.8 (paper 0237).
func TestTableCellsOfAResizedTable(t *testing.T) {
	grid := cellGrid()
	grid.NativeFragments = []nativeFragment{fragmentAt(1, [2]string{"1", "2"})}
	page := ownersPage([][]string{{"a", "b", "c", "1", "2"}}, [][2]float64{{90, 300}}, 0.8)
	report := TableCells{}
	cells := taggerWith(grid).tableCells(&glyphowner.Owners{Leaves: []string{"p"}, Pages: []glyphowner.Page{page}}, &report)
	if report.Fragments != 1 || cells[1][0] != "h1" || cells[1][2] != "h2" || cells[1][4] != "b2" {
		t.Fatalf("report %+v, cells %v", report, cells[1])
	}
}

// A fragment whose glyphs are not on the page places nothing and is counted.
func TestTableCellsUnplacedFragment(t *testing.T) {
	grid := cellGrid()
	grid.NativeFragments = []nativeFragment{fragmentAt(1, [2]string{"1", "2"})}
	page := ownersPage([][]string{{"x", "y", "z", "1", "2"}}, [][2]float64{{72, 72}}, 1)
	report := TableCells{}
	cells := taggerWith(grid).tableCells(&glyphowner.Owners{Leaves: []string{"p"}, Pages: []glyphowner.Page{page}}, &report)
	if report.Unplaced != 1 || report.Fragments != 0 || len(cells[1]) != 0 {
		t.Fatalf("report %+v, cells %v", report, cells)
	}
}

// A placed glyph takes its cell when the owners gave it the table's block or no node (a footnote
// mark in a cell, paper 0565), never when they gave it another node.
func TestCellLeafKeepsOtherOwners(t *testing.T) {
	tagger := taggerWith(cellGrid())
	cells, pairs := map[int]string{3: "b1"}, map[int]int{0: 3, 1: 4}
	if leaf := tagger.cellLeaf(cells, pairs, 0, "p"); leaf != "b1" {
		t.Errorf("block's glyph in %q", leaf)
	}
	if leaf := tagger.cellLeaf(cells, pairs, 0, ""); leaf != "b1" {
		t.Errorf("ownerless glyph in %q", leaf)
	}
	if leaf := tagger.cellLeaf(cells, pairs, 0, "q"); leaf != "" {
		t.Errorf("another block's glyph in %q", leaf)
	}
	if leaf := tagger.cellLeaf(cells, pairs, 1, "p"); leaf != "" {
		t.Errorf("glyph outside the cells in %q", leaf)
	}
}
