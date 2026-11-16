package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A table's caption runs on the other side of its caption context from the label: the label in the
// float (papers2/0372 tab:zp_check, entity 13; caption context 14 of float 13) or targeting the caption
// context while the caption block is the float's (papers5/1210 t:*, target 60; caption block entity 59).
func TestATableLabelFindsItsCaptionAcrossTheCaptionContext(t *testing.T) {
	records := []rec.Map{
		{"kind": "diagnostic", "code": "caption_context", "entity": int64(14), "float": int64(13), "seq": int64(5)},
		{"kind": "diagnostic", "code": "caption_context", "entity": int64(60), "float": int64(59), "seq": int64(50)},
	}
	inFloat := textBlock("b47", "paragraph", int64(14), "Table 1. Zero points.")
	inCaption := textBlock("b1069", "paragraph", int64(59), "Table 2: Line data.")
	other := textBlock("b9", "paragraph", int64(14), "Table 10. Another table.")
	e := &Evidence{blocks: []rec.Map{inFloat, inCaption, other}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")

	unowned := rec.Map{"name": "tab:zp_check", "counter": "table", "number": "1", "entity": int64(13),
		"target_entity": int64(0), "seq": int64(10)}
	if got := blockIDs(e.targetBlocks(unowned)); len(got) != 1 || got[0] != "b47" {
		t.Fatalf("label in the float targets %v, want the caption b47", got)
	}
	targeted := rec.Map{"name": "t:data", "counter": "table", "number": "2", "entity": int64(59),
		"target_entity": int64(60), "seq": int64(55)}
	if got := blockIDs(e.targetBlocks(targeted)); len(got) != 1 || got[0] != "b1069" {
		t.Fatalf("label on the caption context targets %v, want the float's caption b1069", got)
	}
	// A number the caption does not print names nothing.
	unowned["number"] = "3"
	if got := blockIDs(e.targetBlocks(unowned)); len(got) != 0 {
		t.Fatalf("an unprinted number targets %v, want nothing", got)
	}
}
