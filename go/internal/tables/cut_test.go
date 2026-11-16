package tables

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func cutRow(logical int64, cell string) rec.Map {
	return rec.Map{"logical_row": logical, "cells": []any{rec.Map{"cell": cell}}}
}

// A longtable prints its foot and repeated head between two body rows at a page break; a \multirow
// cell spanning that break is cut there, so the table can hold the inserted rows (paper 161).
func TestMultirowIsCutAtAPageBreakInterruption(t *testing.T) {
	e := &rich{roles: map[string]string{key("f"): "repeat_foot", key("h"): "repeat_head"}}
	// Display order: body 0, body 1, foot 9, head 8, body 2, body 3 (logical order: head, body, foot).
	rows := []rec.Map{cutRow(0, "b0"), cutRow(1, "b1"), cutRow(9, "f"), cutRow(8, "h"), cutRow(2, "b2"), cutRow(3, "b3")}
	logical := []any{int64(8), int64(0), int64(1), int64(2), int64(3), int64(9)}
	cuts := e.interruptions(rows, logical)
	if !slices.Equal(cuts, []int64{2}) {
		t.Fatalf("cuts %v, want [2] (after the second body row)", cuts)
	}
	across := rec.Map{"row": int64(1), "rowspan": int64(4)}
	within := rec.Map{"row": int64(3), "rowspan": int64(2)}
	cutAtInterruptions([]rec.Map{across, within}, cuts)
	if got := fetchInt(across, "rowspan"); got != 2 {
		t.Fatalf("spanning cell rowspan %d, want 2", got)
	}
	if got := fetchInt(within, "rowspan"); got != 2 {
		t.Fatalf("cell after the break changed to %d", got)
	}
}

func TestNoInterruptionWithoutExtraRows(t *testing.T) {
	e := &rich{roles: map[string]string{}}
	rows := []rec.Map{cutRow(0, "a"), cutRow(1, "b")}
	if cuts := e.interruptions(rows, []any{int64(0), int64(1)}); len(cuts) != 0 {
		t.Fatalf("cuts %v", cuts)
	}
}

func occupiedCell(row, rowspan int64, text string) rec.Map {
	return rec.Map{"row": row, "column": int64(0), "rowspan": rowspan, "colspan": int64(1),
		"content": []any{rec.Map{"type": "text", "text": text}}}
}

// \multirow reserves nothing: a \multirow{2}{*}{} header above a body row whose first cell has
// text, or a \multirow{3} in a two-row section, ends before the row that fills its column
// (papers 624, 698).
func TestMultirowEndsBeforeAnOccupiedRow(t *testing.T) {
	header := occupiedCell(0, 2, "")
	below := occupiedCell(1, 1, "Obj. 1")
	group := occupiedCell(2, 3, "Verification")
	free := occupiedCell(3, 1, " ")
	next := occupiedCell(4, 1, "Translation")
	cells := []rec.Map{header, below, group, free, next}
	cutAtOccupiedRows(cells)
	if got := fetchInt(header, "rowspan"); got != 1 {
		t.Fatalf("header rowspan %d, want 1", got)
	}
	if got := fetchInt(group, "rowspan"); got != 2 {
		t.Fatalf("group rowspan %d, want 2", got)
	}
	if got := len(collapseCoveredCells(cells)); got != 4 {
		t.Fatalf("%d cells after collapsing, want 4", got)
	}
}
