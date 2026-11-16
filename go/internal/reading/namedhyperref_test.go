package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// \hyperref[sec:methods]{Section~II} to an unnumbered heading's label links its text; a \hyperref to a
// numbered label still has to print that number (papers2/0385, papers5/1187).
func TestHyperrefToAnUnnumberedLabelLinksItsText(t *testing.T) {
	e := &Evidence{records: []rec.Map{
		{"kind": "reference", "reference": int64(1), "selector": `\hyperref`, "target": "sec:methods", "seq": int64(1)},
		{"kind": "reference", "reference": int64(2), "selector": `\hyperref`, "target": "sec:two", "seq": int64(2)},
	}}
	e.references = e.inventory("reference", "reference")
	targets := map[any]rec.Map{
		"sec:methods": {"label": "sec:methods", "number": "", "block": "b13"},
		"sec:two":     {"label": "sec:two", "number": "2", "block": "b20"},
	}
	unnumbered := rec.Map{"type": "reference", "id": "r1", "reference": int64(1), "target": "sec:methods", "text": "Section II"}
	e.resolveReference(targets, unnumbered, false)
	if unnumbered["target_block"] != "b13" {
		t.Fatalf("unnumbered hyperref span %v, want a link to b13", unnumbered)
	}
	numbered := rec.Map{"type": "reference", "id": "r2", "reference": int64(2), "target": "sec:two", "text": "Section 3"}
	e.resolveReference(targets, numbered, false)
	if _, linked := numbered["target_block"]; linked {
		t.Fatalf("numbered hyperref printing another number links: %v", numbered)
	}
}
