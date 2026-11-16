package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// fillEvent is a colortbl fill leader of table 3 as the collector records it: an area of the
// row, with its cell and colour.
func fillEvent(cell int64, color string) rec.Map {
	return rec.Map{"code": "rich_table_rule", "table": int64(3), "page": int64(1), "in_row": true, "native_leader": true,
		"rect_sp": []any{int64(0), int64(0), int64(2_000_000), int64(800_000)}, "cell": cell, "color": color,
		"height": int64(runningSize), "depth": int64(runningSize)}
}

// A cell takes the colour of the last fill painted over it (colortbl paints a column's, a row's
// and then a cell's own colour); a thin rule, another table's fill, a fill without a colour or
// one outside the rows colour nothing, and the fragments keep no cell or colour of their rules.
func TestCellFillsAreTheLastPaintedFill(t *testing.T) {
	thin := fillEvent(7, "#ff0000")
	thin["rect_sp"] = []any{int64(0), int64(0), int64(2_000_000), int64(26_214)}
	other := fillEvent(8, "#00ff00")
	other["table"] = int64(4)
	outside := fillEvent(8, "#0000ff")
	outside["in_row"] = false
	highlight := fillEvent(8, "#ffff00") // soul's \hl behind a word: a leader of fixed height and depth (slides 0018)
	highlight["height"], highlight["depth"] = int64(558_155), int64(239_210)
	uncoloured := fillEvent(8, "#0000ff")
	delete(uncoloured, "color")
	e := &rich{events: []rec.Map{fillEvent(7, "#ececec"), fillEvent(7, "#ffff00"), thin, other, outside, highlight, uncoloured,
		fillEvent(9, "#333333")}}
	fills := e.cellFills(int64(3))
	if len(fills) != 2 || fills[key(int64(7))] != "#ffff00" || fills[key(int64(9))] != "#333333" {
		t.Fatalf("fills = %v", fills)
	}
	cell := withFill(rec.Map{"source_cell": int64(7)}, fills)
	if cell["fill"] != "#ffff00" {
		t.Errorf("cell = %v", cell)
	}
	if plain := withFill(rec.Map{"source_cell": int64(8)}, fills); plain["fill"] != nil {
		t.Errorf("a cell without a fill has one: %v", plain)
	}
	fragments := e.fragments(int64(3), []rec.Map{{"page": int64(1), "logical_row": int64(1), "cells": []any{}}}, []any{int64(1)})
	rules := rec.FetchList(rec.AsMap(fragments[0]), "rules")
	if len(rules) == 0 || rec.AsMap(rules[0])["cell"] != nil || rec.AsMap(rules[0])["color"] != nil {
		t.Errorf("fragment rules = %v", rules)
	}
}
