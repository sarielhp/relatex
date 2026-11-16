package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A label after the bibliography records the last numbered section (MNRAS's lastpage: number E,
// the appendix's), while folios and table cells also open with the number: it names the one
// heading printing it, after its name in an appendix (papers2/0422 last, papers3/0669 lastpage).
func TestLabelAfterTheBibliographyNamesTheOneHeadingPrintingItsNumber(t *testing.T) {
	cases := map[string]struct{ heading, number string }{
		"a numbered section":   {"4 Discussion", "4"},
		"an appendix's letter": {"APPENDIX E: THE CENTRAL DENSITY", "E"},
	}
	for name, test := range cases {
		heading := textBlock("heading", "section", int64(27), test.heading)
		references := textBlock("references", "section", int64(29), "References")
		cell := textBlock("cell", "paragraph", int64(40), test.number)
		records := []rec.Map{
			{"kind": "candidate", "seq": int64(1), "text": test.heading, "entity": int64(27), "candidate": int64(1), "group": "paragraph"},
			{"kind": "candidate", "seq": int64(2), "text": test.number, "entity": int64(40), "candidate": int64(2), "group": "paragraph"},
			{"kind": "candidate", "seq": int64(3), "text": test.number + " Tilden et al.", "entity": int64(41), "candidate": int64(3), "group": "paragraph"},
		}
		heading["owner"], references["owner"], cell["owner"] = int64(11), int64(12), int64(13)
		e := &Evidence{blocks: []rec.Map{heading, references, cell}, records: records}
		e.owners = e.inventory("flow_candidate", "owner")
		label := rec.Map{"name": "lastpage", "counter": "section", "number": test.number, "anchor": "section.4",
			"entity": int64(30), "target_entity": int64(29), "seq": int64(50)}
		if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "heading" {
			t.Errorf("%s: label targets %v, want the heading printing %s", name, got, test.number)
		}
	}
}

// emulateapj sets the title, the affiliations and the first section in one context: the section's
// label names its heading "1. Introduction", not the title, though an affiliation mark opens
// "1Meridian Southern Observatory" (papers5/1245 intro).
func TestSectionLabelInTheFrontMatterContextNamesItsHeading(t *testing.T) {
	title := textBlock("title", "section", int64(1), " Properties of submillimeter galaxies")
	affiliation := textBlock("affiliation", "section", int64(1), "1Meridian Southern Observatory")
	heading := textBlock("heading", "section", int64(1), "1. Introduction")
	e := &Evidence{blocks: []rec.Map{title, affiliation, heading}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "intro", "counter": "section", "number": "1", "anchor": "section.1",
		"entity": int64(2), "target_entity": int64(1), "seq": int64(413)}
	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "heading" {
		t.Fatalf("intro label targets %v, want the heading 1. Introduction", got)
	}
}
