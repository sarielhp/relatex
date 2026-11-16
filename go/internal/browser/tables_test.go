package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func tableDocument(ids ...string) rec.Map {
	nodes := rec.Map{}
	for _, id := range ids {
		nodes["n-"+id] = rec.Map{"content": []any{rec.Map{"type": "table_grid", "id": id}}}
	}
	return rec.Map{"nodes": nodes}
}

// A parsed document holds its nodes in key order; the page lists its tables in reading order,
// so each is matched to its span by id.
func TestTableSpansAreFoundByIDWhateverTheNodeOrder(t *testing.T) {
	spans, byID := tableSpans(tableDocument("b", "a", "c"))
	if len(spans) != 3 || len(byID) != 3 {
		t.Fatalf("spans %d, by id %d, want 3", len(spans), len(byID))
	}
	for _, id := range []string{"c", "a", "b"} {
		if byID[id]["id"] != id {
			t.Errorf("span %q not found by its id", id)
		}
	}
}

// A cell the browser sets shorter than its native height is a recorded loss, not a stop.
func TestReportListsShrunkCells(t *testing.T) {
	shrunkCells = []any{rec.Map{"table": "a", "native_px": 143.462, "browser_px": 143.4375}}
	defer func() { shrunkCells = nil }()
	document := rec.Map{"nodes": rec.Map{}, "labels": rec.Map{}, "note_order": []any{}, "block_order": []any{}, "unverified_requirements": []any{}, "assets": rec.Map{}}
	got := report(rec.Map{"blocks": []any{}, "math": []any{}}, document)
	if cells, _ := got["shrunk_cells"].([]any); len(cells) != 1 {
		t.Fatalf("shrunk_cells = %v", got["shrunk_cells"])
	}
}

func TestValidateTablesRejectsATableTheDocumentDoesNotHold(t *testing.T) {
	inventory := rec.Map{"tables": []any{rec.Map{"id": "zz"}}}
	defer func() {
		if recover() == nil {
			t.Error("a page table without a span was accepted")
		}
	}()
	validateTables(inventory, tableDocument("a"))
}
