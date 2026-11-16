package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// itemEvidence: the book's \item in an enumitem list the collector does not observe (algorithm
// lines "1:" and "2:" in compactenumA): each marker box, then the label, then the item's paragraph.
func itemEvidence() *Evidence {
	text := func(value string) []any { return []any{rec.Map{"type": "text", "text": value}} }
	return &Evidence{
		records: []rec.Map{
			{"kind": "flow_candidate", "group": "hbox", "entity": int64(818), "owner": int64(50), "seq": int64(10)},
			{"kind": "label", "name": "line:a", "number": "{{1:}}", "anchor": "Item.104", "entity": int64(818), "seq": int64(11)},
			{"kind": "flow_candidate", "group": "math", "entity": int64(818), "owner": int64(51), "seq": int64(12)},
			{"kind": "flow_candidate", "group": "paragraph", "entity": int64(818), "owner": int64(52), "seq": int64(13)},
			{"kind": "flow_candidate", "group": "hbox", "entity": int64(818), "owner": int64(53), "seq": int64(14)},
			{"kind": "label", "name": "line:b", "number": "{{2:}}", "anchor": "Item.105", "entity": int64(818), "seq": int64(15)},
			{"kind": "flow_candidate", "group": "paragraph", "entity": int64(818), "owner": int64(54), "seq": int64(16)},
		},
		blocks: []rec.Map{
			{"id": "b1", "owner": int64(52), "content": text("1: Pick a random point.")},
			{"id": "b2", "owner": int64(54), "content": text("12: Return.")},
		},
	}
}

// TestUnobservedItemLabelNamesTheParagraphItsMarkerOpens: the label names the next paragraph of
// its context when that paragraph opens with the label's marker; another marker ("12:" for
// "2:") names nothing, and a label without an item anchor is not an item.
func TestUnobservedItemLabelNamesTheParagraphItsMarkerOpens(t *testing.T) {
	e := itemEvidence()
	label := func(name string) rec.Map {
		for _, row := range e.records {
			if row["name"] == name {
				return row
			}
		}
		return nil
	}
	if blocks := e.unobservedItemBlocks(label("line:a")); len(blocks) != 1 || blocks[0]["id"] != "b1" {
		t.Errorf("line:a %v", blocks)
	}
	if blocks := e.unobservedItemBlocks(label("line:b")); len(blocks) != 0 {
		t.Errorf("line:b %v", blocks)
	}
	other := rec.Map{"name": "x", "number": "1:", "anchor": "section.1", "entity": int64(818), "seq": int64(11)}
	if blocks := e.unobservedItemBlocks(other); len(blocks) != 0 {
		t.Errorf("section label %v", blocks)
	}
}

// TestInlineItemLabelNamesTheParagraphPrintingItsMarkerBox: an inline list (enumerate*,
// inparaenum) sets each marker in a box of the running paragraph, where the item's label follows
// it: the label names that paragraph, which prints the marker as a word even when it also
// prints it elsewhere ("Lemma 3.4 (ii)"). A box printing another marker, a box that is no
// marker of the label's number, or a paragraph that prints the marker glued to a word names nothing.
func TestInlineItemLabelNamesTheParagraphPrintingItsMarkerBox(t *testing.T) {
	box := func(owner, seq int64, text string) rec.Map {
		return rec.Map{"kind": "flow_candidate", "group": "hbox", "entity": int64(42), "owner": owner, "seq": seq,
			"items": []any{rec.Map{"type": "text", "text": text, "space": ""}}}
	}
	evidence := func(marker, printed string) *Evidence {
		return &Evidence{
			records: []rec.Map{box(1, 1, "Remark 3.5."), box(2, 2, marker),
				{"kind": "flow_candidate", "group": "paragraph", "entity": int64(42), "owner": int64(3), "seq": int64(4)}},
			blocks: []rec.Map{{"id": "b1", "owner": int64(3),
				"content": []any{rec.Map{"type": "text", "text": printed}, rec.Map{"type": "math", "occurrence": "m1"}}}},
		}
	}
	label := rec.Map{"name": "it:b", "number": "{{(ii)}}", "anchor": "Item.18", "entity": int64(42), "seq": int64(3)}
	paragraph := "Remark 3.5. (i) Lemma 3.4 (ii) holds. (ii) Another, by Lemma 3.4 (ii)."
	if blocks := evidence("(ii)", paragraph).unobservedItemBlocks(label); len(blocks) != 1 || blocks[0]["id"] != "b1" {
		t.Errorf("inline item %v", blocks)
	}
	roman := rec.Map{"name": "it:c", "number": "ii", "anchor": "Item.2", "entity": int64(42), "seq": int64(3)}
	if blocks := evidence("(ii)", "Two facts: (i) one, and (ii) two.").unobservedItemBlocks(roman); len(blocks) != 1 {
		t.Errorf("paralist item %v", blocks)
	}
	for marker, printed := range map[string]string{"(iii)": paragraph, "Lemma": paragraph, "(ii)": "Remark 3.5. (i) one x(ii)two"} {
		if blocks := evidence(marker, printed).unobservedItemBlocks(label); len(blocks) != 0 {
			t.Errorf("%q in %q: %v", marker, printed, blocks)
		}
	}
}
