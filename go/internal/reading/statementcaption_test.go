package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A remark and an example of one number in the label's context: the label names the statement of
// its counter (books/0006 remark:deltaderivofjump).
func TestStatementLabelNamesTheCaptionOfItsCounter(t *testing.T) {
	remark := textBlock("remark", "paragraph", int64(7), "Remark 6.4.1: The jump.")
	example := textBlock("example", "paragraph", int64(7), "Example 6.4.1: A step.")
	e := &Evidence{blocks: []rec.Map{remark, example}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "remark:jump", "counter": "remark", "number": "6.4.1", "anchor": "remark.6.4.1",
		"entity": int64(7), "target_entity": int64(0), "seq": int64(10)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "remark" {
		t.Fatalf("remark label targets %v, want the remark", got)
	}
}

// A remark opening with a list sets its head into the first item's paragraph, another context: the
// label names the one block of the document opening with its caption line (papers2/0414).
func TestStatementLabelFindsItsHeadInAnotherContext(t *testing.T) {
	head := textBlock("head", "paragraph", int64(61), "Remark 1 (computational details). In step 1, we")
	other := textBlock("other", "paragraph", int64(70), "Lemma 1. Every graph")
	e := &Evidence{blocks: []rec.Map{head, other}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "rem:details", "counter": "remark", "number": "1", "anchor": "remark.1",
		"entity": int64(48), "target_entity": int64(0), "seq": int64(10)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "head" {
		t.Fatalf("remark label targets %v, want the item paragraph printing its head", got)
	}
	e.blocks = append(e.blocks, textBlock("again", "paragraph", int64(80), "Remark 1. Again"))
	if got := blockIDs(e.targetBlocks(label)); len(got) != 0 {
		t.Fatalf("remark label with two heads printing its number targets %v, want none", got)
	}
}
