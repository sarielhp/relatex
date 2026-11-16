package reading

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func floatContext(entity, seq int64) rec.Map {
	return rec.Map{"kind": "diagnostic", "code": "float_context", "entity": entity, "seq": seq}
}

// An algorithm float: its caption context (entity 2) prints "Algorithm 1", its lines (entity 1)
// open with their numbers.
func algorithmEvidence() *Evidence {
	return &Evidence{
		blocks: []rec.Map{
			textBlock("cap", "paragraph", int64(2), "Algorithm 1: Reduction"),
			textBlock("l1", "paragraph", int64(1), "1 if small then"),
			textBlock("l2", "paragraph", int64(1), "2 compute by brute force"),
			textBlock("l12", "paragraph", int64(1), "12: end while"),
		},
		records: []rec.Map{floatContext(1, 10), captionContext(2, 1, 11)},
	}
}

func TestAlgorithmLineLabelNamesItsLine(t *testing.T) {
	e := algorithmEvidence()
	cases := map[string]struct {
		number string
		want   []string
	}{
		"an algorithm2e line":               {"2", []string{"l2"}},
		"an algorithmicx line":              {"12", []string{"l12"}},
		"the caption's own number keeps it": {"1", []string{"cap"}},
		"no line prints the number":         {"7", []string{"cap"}},
	}
	for name, test := range cases {
		label := rec.Map{"name": "step", "number": test.number, "counter": "algocf", "entity": int64(1),
			"target_entity": int64(2), "seq": int64(20)}
		if got := blockIDs(e.targetBlocks(label)); !slices.Equal(got, test.want) {
			t.Errorf("%s: blocks %v, want %v", name, got, test.want)
		}
	}
}

// A \label after the subtables records the last subcaption; the float's own caption prints its number.
func TestFloatLabelAfterSubcaptionsNamesTheFloatCaption(t *testing.T) {
	e := &Evidence{
		blocks: []rec.Map{
			textBlock("main", "paragraph", int64(2), "Table III: Matching results"),
			textBlock("a", "paragraph", int64(3), "(a) Trigram"),
			textBlock("c", "paragraph", int64(4), "(c) Unigrams"),
		},
		records: []rec.Map{floatContext(1, 10), captionContext(2, 1, 11), captionContext(3, 1, 12), captionContext(4, 1, 13)},
	}
	label := rec.Map{"name": "table:ngram", "number": "III", "counter": "table", "entity": int64(1),
		"target_entity": int64(4), "seq": int64(20)}
	if got := blockIDs(e.targetBlocks(label)); !slices.Equal(got, []string{"main"}) {
		t.Errorf("blocks %v, want the float caption", got)
	}
	// A label outside any observed float keeps its recorded target.
	e.records = e.records[1:]
	if got := blockIDs(e.targetBlocks(label)); !slices.Equal(got, []string{"c"}) {
		t.Errorf("blocks %v, want the recorded caption", got)
	}
}

// subfig sets a \subfloat's picture, and the \label in it, before its subcaption: the label
// records the previous subcaption and names the block opening with its own mark.
func TestSubfloatLabelBeforeItsCaptionNamesItsMark(t *testing.T) {
	e := &Evidence{
		blocks: []rec.Map{
			textBlock("a", "paragraph", int64(2), "(a) DR"),
			textBlock("b", "paragraph", int64(3), "(b) LI"),
		},
		records: []rec.Map{floatContext(1, 10), captionContext(2, 1, 11), captionContext(3, 1, 13)},
	}
	label := rec.Map{"name": "diagram-b", "number": "1b", "counter": "subfigure", "entity": int64(1),
		"target_entity": int64(2), "seq": int64(12)}
	if got := blockIDs(e.targetBlocks(label)); !slices.Equal(got, []string{"b"}) {
		t.Errorf("blocks %v, want the block printing (b)", got)
	}
}

// A longtable's \label records the previous float's caption as its target; its own caption row
// carries the label's anchor, and the block holding that cell prints the number.
func TestLongtableLabelNamesItsCaptionRow(t *testing.T) {
	cell := rec.Map{"type": "text", "text": "Table 6: All features"}
	grid := rec.Map{"id": "grid", "type": "paragraph", "entity": int64(5), "content": []any{cell}}
	e := &Evidence{
		blocks:  []rec.Map{textBlock("prev", "paragraph", int64(4), "Table 5: Pareto front"), grid},
		origins: map[uintptr]rec.Map{rec.Identity(cell): {"cell": int64(9)}},
		records: []rec.Map{
			captionContext(4, 3, 10),
			{"kind": "diagnostic", "code": "rich_table_caption", "source": "native-array-cells", "anchor": "table.6",
				"cell": int64(9), "seq": int64(30)},
		},
	}
	label := rec.Map{"name": "tab:feature_list", "number": "6", "counter": "table", "anchor": "table.6", "entity": int64(5),
		"target_entity": int64(4), "seq": int64(31)}
	if got := blockIDs(e.targetBlocks(label)); !slices.Equal(got, []string{"grid"}) {
		t.Errorf("blocks %v, want the caption row's block", got)
	}
}
