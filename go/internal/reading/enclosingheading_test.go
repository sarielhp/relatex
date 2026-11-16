package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// unnumberedSections: a chapter whose heading prints no number, with a paragraph context under it,
// and a section whose heading prints its own number.
func unnumberedSections() *Evidence {
	chapter := textBlock("chapter", "section", int64(3), "Introduction")
	body := textBlock("body", "paragraph", int64(4), "Some text.")
	numbered := textBlock("numbered", "section", int64(5), "2 Methods")
	records := []rec.Map{
		{"kind": "context", "seq": int64(1), "entity": int64(3), "parent": int64(0), "role": "section"},
		{"kind": "diagnostic", "code": "heading_context", "seq": int64(2), "entity": int64(3), "counter": "chapter"},
		{"kind": "context", "seq": int64(3), "entity": int64(4), "parent": int64(3), "role": "paragraph"},
		{"kind": "context", "seq": int64(4), "entity": int64(5), "parent": int64(0), "role": "section"},
		{"kind": "diagnostic", "code": "heading_context", "seq": int64(5), "entity": int64(5), "counter": "section"},
	}
	e := &Evidence{blocks: []rec.Map{chapter, body, numbered}, records: records, parents: map[any]any{}}
	for _, row := range records {
		if row["kind"] == "context" {
			e.parents[row["entity"]] = row["parent"]
		}
	}
	e.owners = e.inventory("flow_candidate", "owner")
	return e
}

func TestUnnumberedSectionLabelNamesItsEnclosingHeading(t *testing.T) {
	e := unnumberedSections()
	cases := map[string]struct {
		label rec.Map
		want  []string
	}{
		"a starred chapter's label in its paragraph": {rec.Map{"name": "intro", "counter": "", "number": "",
			"anchor": "chapter*.2", "entity": int64(4), "target_entity": int64(0), "seq": int64(10)}, []string{"chapter"}},
		"an appendix letter its heading does not print": {rec.Map{"name": "app", "counter": "", "number": "A",
			"anchor": "chapter.1", "entity": int64(4), "target_entity": int64(0), "seq": int64(10)}, []string{"chapter"}},
		"another counter than the heading's": {rec.Map{"name": "sub", "counter": "", "number": "",
			"anchor": "section*.2", "entity": int64(4), "target_entity": int64(0), "seq": int64(10)}, []string{}},
		"a page anchor": {rec.Map{"name": "FirstPage", "counter": "", "number": "",
			"anchor": "Doc-Start", "entity": int64(4), "target_entity": int64(0), "seq": int64(10)}, []string{}},
		"a heading printing another number": {rec.Map{"name": "other", "counter": "", "number": "3",
			"anchor": "section.3", "entity": int64(5), "target_entity": int64(0), "seq": int64(10)}, []string{}},
	}
	for name, test := range cases {
		got := blockIDs(e.targetBlocks(test.label))
		if len(got) != len(test.want) || (len(got) == 1 && got[0] != test.want[0]) {
			t.Errorf("%s: targets %v, want %v", name, got, test.want)
		}
	}
}

// An appendix table's label records the appendix's anchor and the table's number: it does not name
// the appendix heading, which prints its own letter (papers2/0372 tab:nightlog).
func TestTableLabelInAnAppendixDoesNotNameItsHeading(t *testing.T) {
	appendix := textBlock("appendix", "section", int64(73), " A. NIGHT LOG OF OBSERVATIONS")
	records := []rec.Map{
		{"kind": "context", "seq": int64(1), "entity": int64(73), "parent": int64(0), "role": "section"},
		{"kind": "diagnostic", "code": "heading_context", "seq": int64(2), "entity": int64(73), "counter": "section"},
		{"kind": "context", "seq": int64(3), "entity": int64(74), "parent": int64(73), "role": "paragraph"},
	}
	e := &Evidence{blocks: []rec.Map{appendix}, records: records, parents: map[any]any{int64(73): int64(0), int64(74): int64(73)}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "tab:nightlog", "counter": "", "number": "4", "anchor": "appendix.A",
		"entity": int64(74), "target_entity": int64(0), "seq": int64(10)}
	if got := blockIDs(e.targetBlocks(label)); len(got) != 0 {
		t.Fatalf("appendix table label targets %v, want none", got)
	}
}
