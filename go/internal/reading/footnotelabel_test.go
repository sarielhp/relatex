package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A \label in a footnote's text records the section's counter but the footnote's number and anchor
// (equation.11-footnote.4): it names the footnote, not the section heading (papers2/0355 fraction).
func TestFootnoteLabelNamesItsNote(t *testing.T) {
	heading := textBlock("heading", "section", int64(36), "B. The nanoflares")
	note := textBlock("note", "paragraph", int64(41), "4A more proper estimation")
	records := []rec.Map{
		{"kind": "diagnostic", "code": "note_body", "seq": int64(1), "entity": int64(41), "number": "4", "note": int64(4)},
	}
	e := &Evidence{blocks: []rec.Map{heading, note}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "fraction", "counter": "subsection", "number": "4", "anchor": "equation.11-footnote.4",
		"entity": int64(41), "target_entity": int64(36), "seq": int64(10)}
	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "note" {
		t.Fatalf("footnote label targets %v, want its note", got)
	}
	other := rec.Merge(label, rec.Map{"anchor": "subsection.2.2", "number": "2.2"})
	if got := blockIDs(e.targetBlocks(other)); len(got) != 1 || got[0] != "heading" {
		t.Fatalf("section label targets %v, want the heading", got)
	}
}
