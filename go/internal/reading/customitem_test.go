package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A class's own algorithm item (\refstepcounter{MYitem}\textbf{\theMYitem)}) records its counter's
// anchor: the label names the next paragraph of its context when that line prints "3)", after the
// algorithmic line number or not (papers5/1242 subalg, rule).
func TestCustomItemLabelNamesTheLinePrintingItsMarker(t *testing.T) {
	cases := map[string]struct {
		text string
		want int
	}{
		"a bare item":              {" 3) Based on Algorithm 2", 1},
		"after a line number":      {"15: 3) Update", 1},
		"a line numbered 3":        {"3: 1) Calculate", 0},
		"another item's marker":    {" 4) Based on Algorithm 2", 0},
		"the number inside a word": {"3)x", 0},
	}
	for name, test := range cases {
		line := textBlock("line", "paragraph", int64(99), test.text)
		line["owner"] = int64(8348)
		e := &Evidence{blocks: []rec.Map{line}, records: []rec.Map{
			{"kind": "flow_candidate", "seq": int64(20), "entity": int64(99), "group": "paragraph", "owner": int64(8348)},
		}}
		e.owners = e.inventory("flow_candidate", "owner")
		label := rec.Map{"name": "subalg", "counter": "MYitem", "number": "3", "anchor": "MYitem.1.3",
			"entity": int64(99), "target_entity": int64(0), "seq": int64(10)}
		if got := blockIDs(e.targetBlocks(label)); len(got) != test.want {
			t.Errorf("%s: item label targets %v, want %d blocks", name, got, test.want)
		}
	}
}

// A class's own algorithm environment captions "Algorithm 1." under its MYalgorithmic counter, beside
// a "Stage 1:" line: the label names the caption whose name its counter holds (papers5/1242 alg1).
func TestCustomAlgorithmLabelNamesItsCaption(t *testing.T) {
	caption := textBlock("caption", "paragraph", int64(99), "Algorithm 1. Road-traffic-aware allocation")
	stage := textBlock("stage", "paragraph", int64(99), "Stage 1: Long-term allocation")
	e := &Evidence{blocks: []rec.Map{caption, stage}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "alg1", "counter": "MYalgorithmic", "number": "1", "anchor": "MYalgorithmic.1",
		"entity": int64(99), "target_entity": int64(0), "seq": int64(10)}
	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "caption" {
		t.Fatalf("algorithm label targets %v, want its caption", got)
	}
}
