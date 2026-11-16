package pdftag

import (
	"slices"
	"testing"
)

// longGrid is a longtable: its caption, first head, the \endhead copy of both, a body row and the
// \endfoot row.
func longGrid() *tableGrid {
	grid := &tableGrid{}
	add := func(id, role string, row int) {
		source := len(grid.Cells)
		grid.Cells = append(grid.Cells, tableCell{ID: id, Row: row, Role: role, Colspan: 1, Rowspan: 1, SourceCell: &source})
	}
	add("caption", "caption", 0)
	add("caption2", "caption", 1)
	add("head", "first_head", 2)
	add("head2", "repeat_head", 3)
	add("body", "body", 4)
	add("foot", "repeat_foot", 5)
	return grid
}

func TestLongtableRepeatedRowsAreArtifacts(t *testing.T) {
	b := newBuilder(&document{})
	table := b.table(longGrid())
	var leaves []string
	for _, row := range table.kids {
		for _, cell := range row.kids {
			leaves = append(leaves, cell.leaf)
		}
	}
	if want := []string{"caption", "head", "body"}; !slices.Equal(leaves, want) {
		t.Fatalf("cells %v, want %v", leaves, want)
	}
	for _, id := range []string{"caption2", "head2", "foot"} {
		if !b.artifacts[id] {
			t.Errorf("%s is no artifact", id)
		}
	}
	if len(b.artifacts) != 3 || b.artifacts["head"] {
		t.Errorf("artifacts %v", b.artifacts)
	}
}

// Without a first head the repeat head is the table's head: it stays.
func TestLongtableHeadWithoutFirstHeadStays(t *testing.T) {
	grid := &tableGrid{Cells: []tableCell{
		{ID: "head", Role: "repeat_head", Row: 0, Colspan: 1, Rowspan: 1},
		{ID: "body", Role: "body", Row: 1, Colspan: 1, Rowspan: 1}}}
	if got := repeatedCells(grid); len(got) != 0 {
		t.Fatalf("repeated %v", got)
	}
}

// A repeated cell's glyphs lie in an artifact cell: cellLeaf names it, and cutBlock holds it.
func TestLongtableArtifactCellsOfACutBlock(t *testing.T) {
	b := newBuilder(&document{})
	b.markCut(b.table(longGrid()), "p")
	tg := &tagger{builder: b}
	if got := tg.cellLeaf(map[int]string{0: "head2"}, map[int]int{0: 0}, 0, "p"); got != "head2" || !b.artifacts[got] {
		t.Fatalf("cell %q", got)
	}
}
