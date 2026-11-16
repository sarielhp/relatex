package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A deluxetable's label finds no block before the table is projected; after the projection it names
// the one block opening with its caption line, its unresolved diagnostic goes, and its uses link to
// that block (papers2/0372 tab:nightlog).
func TestTableLabelResolvesAfterTheProjection(t *testing.T) {
	label := rec.Map{"name": "tab:nightlog", "counter": "table", "number": "4", "anchor": "table.4",
		"entity": int64(74), "target_entity": int64(0), "seq": int64(10)}
	use := rec.Map{"type": "reference", "id": "r1", "reference": int64(1), "target": "tab:nightlog", "text": "4"}
	paragraph := rec.Map{"id": "paragraph", "type": "paragraph", "entity": int64(5), "content": []any{use}}
	e := &Evidence{blocks: []rec.Map{paragraph}, targets: []rec.Map{}, records: []rec.Map{
		{"kind": "reference", "reference": int64(1), "selector": `\@firstoffive`, "target": "tab:nightlog", "seq": int64(20)},
	}}
	e.owners = e.inventory("flow_candidate", "owner")
	e.references = e.inventory("reference", "reference")
	e.resolveLabel("tab:nightlog", label)
	e.resolveReferences()
	if len(e.targets) != 0 || len(e.diagnostics) != 2 {
		t.Fatalf("before the projection: targets %v, diagnostics %v", e.targets, e.diagnostics)
	}
	e.blocks = append(e.blocks, textBlock("caption", "paragraph", int64(74), "Table 4 (continued)"))
	e.resolveProjectedTargets()
	if len(e.targets) != 1 || e.targets[0]["block"] != "caption" || len(e.diagnostics) != 0 {
		t.Fatalf("after the projection: targets %v, diagnostics %v", e.targets, e.diagnostics)
	}
	if use["target_block"] != "caption" {
		t.Fatalf("the label's use %v, want a link to the caption", use)
	}
}
