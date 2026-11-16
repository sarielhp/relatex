package pdftag

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// A cell that holds only drawn pictures leaves its glyphs to the Figures: a plot's ticks and
// exponents are the picture's, and what TeX recorded in the cell must not be placed on the same
// text elsewhere on the page (papers4 1081: a plot's "10" read as the empty TD's).
func TestTableCellOfAPictureTakesNoGlyphs(t *testing.T) {
	grid := &tableGrid{Cells: []tableCell{pictureCellOf("td", 11)}}
	grid.NativeFragments = []nativeFragment{exponentFragment()}
	owners := &glyphowner.Owners{Leaves: []string{"p"}, Pages: []glyphowner.Page{exponentOwners()}}
	tagger := taggerWith(grid)
	report := TableCells{}
	if cells := tagger.tableCells(owners, &report); len(cells[1]) != 0 {
		t.Errorf("picture cell's glyphs placed: %v", cells[1])
	}
	// The same fragment with text in its cell does place: the picture alone is what stays unplaced.
	text := &tableGrid{Cells: []tableCell{pictureCellOf("td", 11)}}
	text.Cells[0].Content = []contentItem{{Type: "text", Text: "10"}}
	text.NativeFragments = []nativeFragment{exponentFragment()}
	tagger = taggerWith(text)
	report = TableCells{}
	if cells := tagger.tableCells(owners, &report); len(cells[1]) != 2 {
		t.Errorf("text cell's glyphs not placed: %v (report %+v)", cells[1], report)
	}
}

// pictureCellOf is a grid's cell id of the given source cell, holding a drawn picture only.
func pictureCellOf(id string, source int) tableCell {
	return tableCell{ID: id, SourceCell: &source, Content: []contentItem{{Type: "image", ID: "fig", Source: "plot.pdf"}}}
}

// exponentFragment is the two glyphs "1" "0" a plot's exponent paints in cell 11, at the
// picture's own position in the table.
func exponentFragment() nativeFragment {
	glyph := func(x, y float64, text string, cell int) nativeGlyph {
		return nativeGlyph{Baseline: [2]float64{x * spPerBP, y * spPerBP}, Text: text, Cell: &cell}
	}
	return nativeFragment{Page: 1, Glyphs: []nativeGlyph{glyph(50, 50, "1", 11), glyph(53, 50, "0", 11)}}
}

// exponentOwners are the same "1" "0" painted elsewhere on the page (a page number), where a
// placement of the fragment can find them.
func exponentOwners() glyphowner.Page {
	return glyphowner.Page{Number: 1, X: []float64{140, 143}, Y: []float64{150, 150},
		Text: []string{"1", "0"}, Leaf: []int{1, 1}}
}

// A longtable's repeated head is pagination: what its cell holds, a formula as much as its text,
// is no element of the structure and its glyphs are not bound (papers5 1210: "Eu" read as a P).
func TestLongtableRepeatedCellContentIsPagination(t *testing.T) {
	grid := longGrid()
	for index := range grid.Cells {
		switch grid.Cells[index].ID {
		case "head":
			grid.Cells[index].Content = []contentItem{{Type: "math", Node: "m0"}}
		case "head2":
			grid.Cells[index].Content = []contentItem{{Type: "math", Node: "m1"}}
		}
	}
	first := node{ID: "m0", Type: "math", MathML: "<math><mi>E</mi></math>"}
	repeated := node{ID: "m1", Type: "math", MathML: "<math><mi>E</mi></math>"}
	b := newBuilder(&document{Nodes: map[string]node{"m0": first, "m1": repeated}})
	b.table(grid)
	var leaves []string
	for _, piece := range b.gridPieces(grid) {
		leaves = append(leaves, piece.leaf)
	}
	if slices.Contains(leaves, "m1") {
		t.Errorf("repeated cell's formula is a piece: %v", leaves)
	}
	if !slices.Contains(leaves, "m0") {
		t.Errorf("first head's formula is no piece: %v", leaves)
	}
	if b.leaves["m1"] != nil {
		t.Error("element for a repeated cell's formula")
	}
}
