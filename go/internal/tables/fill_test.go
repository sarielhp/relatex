package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// colortbl's \rowcolor fills a cell with a leader as tall as the row: an area inside the row,
// not a border; a booktabs rule or a | rule is a line (third cohort 639).
func TestAreaFillIsACellBackground(t *testing.T) {
	fill := rec.Map{"in_row": true, "rect_sp": []any{int64(0), int64(0), int64(1_277_952), int64(786_432)}}
	vertical := rec.Map{"in_row": true, "rect_sp": []any{int64(0), int64(0), int64(26_214), int64(786_432)}}
	border := rec.Map{"in_row": false, "rect_sp": []any{int64(0), int64(0), int64(3_787_981), int64(52_430)}}
	if !tablemodel.AreaFill(fill) || tablemodel.AreaFill(vertical) || tablemodel.AreaFill(border) {
		t.Fatalf("fill %v, vertical rule %v, border %v", tablemodel.AreaFill(fill), tablemodel.AreaFill(vertical), tablemodel.AreaFill(border))
	}
}

// An empty cell under a \multirow prints no tagged node, so its native row has no entry for
// column 0: the row's left edge is its first grid cell less that cell's column offset, not that
// cell's own left (third cohort 789, a \cmidrule{2-8} under a \multirow).
func TestRowLeftWithoutAFirstColumnEntry(t *testing.T) {
	cell := rec.Map{"id": "cell-7", "source_cell": int64(7), "row": int64(1), "column": int64(1),
		"rowspan": int64(1), "colspan": int64(1)}
	l := &layout{cells: map[string]rec.Map{"cell-7": cell}, columnWidths: []any{int64(300), int64(500)}}
	native := rec.Map{"origin_sp": []any{int64(1000), int64(0)},
		"cells": []any{rec.Map{"cell": int64(7), "left": int64(300)}}}
	if got := l.rowLeft(native); got != 1000 {
		t.Fatalf("row left %v, want 1000", got)
	}
}
