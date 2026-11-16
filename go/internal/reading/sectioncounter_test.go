package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A class that records no counter for its \section (emulateapj) still names it in the anchor:
// the label runs in a paragraph context after the heading, whose number box precedes it.
func TestSectionLabelWithoutACounterReadsItFromItsAnchor(t *testing.T) {
	heading := textBlock("heading", "heading", int64(1), "1. INTRODUCTION")
	heading["owner"] = int64(68)
	records := []rec.Map{
		{"kind": "flow_candidate", "seq": int64(87), "entity": int64(1), "group": "hbox", "owner": int64(68)},
		{"kind": "candidate", "seq": int64(88), "text": "1.  ", "entity": int64(1), "candidate": int64(64), "group": "hbox"},
		{"kind": "candidate", "seq": int64(90), "text": "INTRODUCTION", "entity": int64(1), "candidate": int64(65),
			"group": "paragraph"},
	}
	e := &Evidence{blocks: []rec.Map{heading}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "sec:intro", "counter": "", "anchor": "section.1", "number": "1",
		"entity": int64(2), "target_entity": int64(0), "seq": int64(92)}

	if got := sectionCounterOf(label); got != "section" {
		t.Fatalf("sectionCounterOf = %q, want section", got)
	}
	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "heading" {
		t.Fatalf("counterless section label targets %v, want the heading", got)
	}
	label["anchor"] = "equation.1"
	if got := blockIDs(e.targetBlocks(label)); len(got) != 0 {
		t.Fatalf("a label anchored on another counter targets %v, want nothing", got)
	}
}

// An appendix section's label (hyperref anchor appendix.A, no counter: papers4/1072) is a section label,
// and its letter names the heading that prints it, not one that merely starts with that letter.
func TestAppendixLabelIsASectionLabelPrintingItsLetter(t *testing.T) {
	label := rec.Map{"name": "app:bin", "counter": "", "anchor": "appendix.A", "number": "A"}
	if got := sectionCounterOf(label); got != "section" {
		t.Fatalf("sectionCounterOf(appendix.A) = %q, want section", got)
	}
	cases := map[string]bool{
		" A. BINNING OF STELLAR PARTICLES": true,
		"A BINNING":                        true,
		" ACKNOWLEDGMENTS":                 false,
		"AUTHOR CONTRIBUTIONS":             false,
	}
	for text, want := range cases {
		if got := printedNumber(text, "A"); got != want {
			t.Errorf("printedNumber(%q, A) = %v, want %v", text, got, want)
		}
	}
	if !printedNumber("3. Title", "3") || printedNumber("3.1 Title", "3") {
		t.Fatalf("numeric section numbers changed")
	}
}
