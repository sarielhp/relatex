package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A tcolorbox titled "Focus Box 2: ..." runs its label with no number and a stale anchor: the label
// names the box's title block; a page anchor stays unowned.
func TestUnnumberedBoxLabelNamesTheBoxTitle(t *testing.T) {
	title := textBlock("title", "paragraph", int64(49), "Focus Box 2: Evaluating Sorting Classic metrics")
	body := textBlock("body", "paragraph", int64(49), "Step 1 is to see.")
	e := &Evidence{blocks: []rec.Map{title, body}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "box:banos", "counter": "", "number": "", "anchor": "section*.16",
		"entity": int64(49), "target_entity": int64(0), "seq": int64(10)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "title" {
		t.Fatalf("box label targets %v, want the box title", got)
	}
	page := rec.Merge(label, rec.Map{"name": "LastPage", "anchor": "page.12"})
	if got := blockIDs(e.targetBlocks(page)); len(got) != 0 {
		t.Fatalf("page anchor targets %v, want none", got)
	}
}
