package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// An author's \thanks in a title tabular: the rich table gathers the call into the block where
// the table starts, and the note's call site follows it (third cohort 565).
func TestCallsiteFollowsItsSpanIntoATableCell(t *testing.T) {
	call := rec.Map{"type": "note_reference", "id": "fn1", "text": "*"}
	grid := rec.Map{"schema_version": tablemodel.Version, "cells": []any{
		rec.Map{"id": int64(1), "row": int64(0), "column": int64(0), "content": []any{rec.Map{"type": "text", "text": "Author"}, call}},
	}}
	blocks := []rec.Map{{"id": int64(7), "content": []any{rec.Map{"type": "table_grid", "grid": grid}}}}
	callsite := rec.Map{"id": "fn1", "mark": int64(1), "block": int64(3)}
	report := rec.Map{"notes": []any{rec.Map{"id": int64(1), "callsites": []any{callsite}}}}
	relocateCallsites(report, blocks)
	if callsite["block"] != int64(7) {
		t.Fatalf("call site block = %v, want 7", callsite["block"])
	}
}
