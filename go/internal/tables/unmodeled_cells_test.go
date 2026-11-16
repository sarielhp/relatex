package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Stretched glue (tabularx, \extracolsep{\fill}) leaves a cell a fractional width; the grid holds
// whole scaled points (second cohort 245, 448).
func TestWholeSPRoundsFractionalSizes(t *testing.T) {
	if got := wholeSP(canonjson.Float(10033519.6)); got != int64(10033520) {
		t.Fatalf("got %v", got)
	}
	if got := wholeSP(int64(7)); got != int64(7) {
		t.Fatalf("integer changed to %v", got)
	}
}

// A cell no native row holds: a repeated head the table never printed is dropped with the grid; the
// caption a class sets in a table cell (deluxetable) stays in the flow (second cohort 351, 352).
func TestSelectedSpansSetsUncapturedCellsApart(t *testing.T) {
	span := func() rec.Map { return rec.Map{"type": "text", "text": "x"} }
	held, head, caption, plain := span(), span(), span(), span()
	origins := map[uintptr]rec.Map{
		rec.Identity(held):    {"table": int64(1), "cell": int64(1)},
		rec.Identity(head):    {"table": int64(1), "cell": int64(2)},
		rec.Identity(caption): {"table": int64(1), "cell": int64(3)},
		rec.Identity(plain):   {"table": int64(1)},
	}
	e := &rich{origins: origins, blocks: []rec.Map{{"content": []any{held, head, caption, plain}}},
		roles: map[string]string{key(int64(2)): "repeat_head"}}
	rows := []rec.Map{{"cells": []any{rec.Map{"cell": int64(1)}}}}
	selected, unprinted := e.selectedSpans(int64(1), rows)
	if len(selected) != 2 || len(unprinted) != 1 || rec.Identity(unprinted[0]) != rec.Identity(head) {
		t.Fatalf("selected %d, unprinted %d", len(selected), len(unprinted))
	}
}

// A formula in an unprinted head cell leaves with its grid: the span is returned apart, and the
// table reports its occurrence as dropped so the document builds no node for it.
func TestUnprintedFormulaIsReturnedApart(t *testing.T) {
	formula := rec.Map{"type": "math", "formula": int64(1), "occurrence": "o1"}
	e := &rich{origins: map[uintptr]rec.Map{rec.Identity(formula): {"table": int64(1), "cell": int64(2)}},
		blocks: []rec.Map{{"content": []any{formula}}}, roles: map[string]string{key(int64(2)): "repeat_head"}}
	if _, unprinted := e.selectedSpans(int64(1), nil); len(unprinted) != 1 {
		t.Fatalf("the formula of an unprinted cell was not set apart: %v", unprinted)
	}
}

// A one-cell tabular without rules (an Inkscape pdf_tex label) reads as text and is not reported.
func TestPlainLabelIsOneCellWithoutRules(t *testing.T) {
	cell := func(table int64) rec.Map {
		return rec.Map{"code": "rich_table_cell", "cell": rec.Map{"table": table}}
	}
	presence := func(table, rules int64) rec.Map {
		return rec.Map{"code": "rich_table_presence", "table": table, "counts": rec.Map{"rules": rules, "row_rules": int64(0)}}
	}
	e := &rich{events: []rec.Map{cell(1), presence(1, 0), cell(2), cell(2), presence(2, 0), cell(3), presence(3, 2)}}
	if !e.plainLabel(int64(1)) || e.plainLabel(int64(2)) || e.plainLabel(int64(3)) {
		t.Fatal("plainLabel wrong")
	}
}

// A fractional \multirow count (bigdelim's \rdelim\}{2.2}{3mm}; tamarin lecture 1) spans the
// nearest whole number of rows, at least one; an upward count stays upward.
func TestFractionalRowspanSpansWholeRows(t *testing.T) {
	cases := map[canonjson.Float]int64{2.2: 2, 2.6: 3, 0.4: 1, -2.2: -2}
	for fraction, want := range cases {
		cell := wholeRowspan(rec.Map{"rowspan": fraction})
		if cell["rowspan"] != want {
			t.Fatalf("%v: got %v, want %v", fraction, cell["rowspan"], want)
		}
	}
	if cell := wholeRowspan(rec.Map{"rowspan": int64(3)}); cell["rowspan"] != int64(3) {
		t.Fatalf("an integer count changed to %v", cell["rowspan"])
	}
}

// The declaration of a fractional span is accepted (it failed the whole reading before).
func TestDeclareAcceptsFractionalRowspan(t *testing.T) {
	e := &rich{cells: map[string]rec.Map{}}
	cell := rec.Map{"id": int64(9), "table": int64(1), "row": int64(3), "column": int64(3), "kind": "l", "colspan": int64(1),
		"rowspan": canonjson.Float(2.2), "vertical_align": "middle", "padding_sp": int64(0)}
	e.declare(rec.Map{"cell": cell, "code": "rich_table_cell", "kind": "diagnostic", "seq": int64(1), "source": RichSource})
	if got := e.cell(int64(9))["rowspan"]; got != int64(2) {
		t.Fatalf("rowspan %v", got)
	}
}
