package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// jmlrHeading is a class whose own sectioning records no section context: the number box and
// the title of a run-in heading are separate candidates inside a paragraph context.
func jmlrHeading() (*Evidence, rec.Map, rec.Map) {
	section := textBlock("section", "heading", int64(8), "3. A Few Examples for Philomatics")
	section["owner"] = int64(148)
	subsection := textBlock("subsection", "paragraph", int64(9), "3.1 Relation of Context and Holism")
	subsection["owner"] = int64(150)
	records := []rec.Map{
		{"kind": "candidate", "seq": int64(595), "text": "3.1", "entity": int64(9), "candidate": int64(149), "group": "hbox"},
		{"kind": "flow_candidate", "seq": int64(596), "entity": int64(9), "group": "paragraph", "owner": int64(150)},
		{"kind": "candidate", "seq": int64(597), "text": "Relation of Context and Holism", "entity": int64(9),
			"candidate": int64(150), "group": "paragraph"},
	}
	e := &Evidence{blocks: []rec.Map{section, subsection}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	return e, section, subsection
}

func TestSectionLabelNamesTheHeadingThatPrintsItsNumber(t *testing.T) {
	e, section, subsection := jmlrHeading()
	label := rec.Map{"name": "sec:holism", "counter": "subsection", "number": "3.1", "entity": int64(9),
		"target_entity": int64(8), "seq": int64(598)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != rec.FetchString(subsection, "id") {
		t.Fatalf("subsection label targets %v, want the paragraph printing 3.1", got)
	}
	if got := blockIDs(e.targetBlocks(rec.Map{"name": "sec:examples", "counter": "section", "number": "3",
		"entity": int64(8), "target_entity": int64(8), "seq": int64(590)})); len(got) != 1 ||
		got[0] != rec.FetchString(section, "id") {
		t.Fatalf("section label targets %v, want the section heading printing 3", got)
	}
}

func TestSectionLabelKeepsATargetThatPrintsItsNumber(t *testing.T) {
	heading := textBlock("heading", "heading", int64(8), "3 Introduction")
	heading["owner"] = int64(148)
	e := &Evidence{blocks: []rec.Map{heading}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "sec:intro", "counter": "section", "number": "3", "entity": int64(8),
		"target_entity": int64(8), "seq": int64(590)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "heading" {
		t.Fatalf("section label with its own printed number targets %v, want the heading", got)
	}
}

func TestPrintedNumberNeedsTheWholeCounter(t *testing.T) {
	cases := map[string]struct {
		text, number string
		want         bool
	}{
		"the heading prints its number":       {"3. A Few Examples", "3", true},
		"another counter's heading":           {"3.1 Relation", "3", false},
		"a longer number after the same ones": {"31 Examples", "3", false},
		"the number alone":                    {"3", "3", true},
		"leading space":                       {"  3. Title", "3", true},
		"no number":                           {"Introduction", "3", false},
	}
	for name, test := range cases {
		if got := printedNumber(test.text, test.number); got != test.want {
			t.Errorf("%s: printedNumber(%q, %q) = %v, want %v", name, test.text, test.number, got, test.want)
		}
	}
}

// A label set after the unnumbered bibliography records the last numbered section (LastPage):
// it names the one heading that prints that number, and stays where it is when two texts open
// with the number.
func TestSectionLabelAfterTheBibliographyNamesTheHeadingThatPrintsItsNumber(t *testing.T) {
	conclusion := textBlock("conclusion", "paragraph", int64(30), "5. Conclusion")
	references := textBlock("references", "heading", int64(33), "References")
	conclusion["owner"], references["owner"] = int64(1), int64(2)
	records := []rec.Map{{"kind": "candidate", "seq": int64(10), "text": "5.", "entity": int64(30), "candidate": int64(1)},
		{"kind": "candidate", "seq": int64(20), "text": "[1] Alpha", "entity": int64(33), "candidate": int64(5)},
		{"kind": "candidate", "seq": int64(21), "text": "[2] Beta", "entity": int64(33), "candidate": int64(6)}}
	e := &Evidence{blocks: []rec.Map{conclusion, references}, records: records}
	label := rec.Map{"name": "LastPage", "counter": "section", "number": "5", "entity": int64(34), "target_entity": int64(33),
		"seq": int64(99)}
	e.owners = e.inventory("flow_candidate", "owner")
	if got := blockIDs(e.printedNumberBlocks(label, []rec.Map{references})); len(got) != 1 || got[0] != "conclusion" {
		t.Fatalf("label targets %v, want the conclusion", got)
	}
	e.records = append([]rec.Map{records[0], {"kind": "candidate", "seq": int64(11), "text": "5 Gyr", "entity": int64(31), "candidate": int64(2)}},
		records[1:]...)
	if got := e.printedNumberBlocks(label, []rec.Map{references}); len(got) != 0 {
		t.Fatalf("an ambiguous number named %v", blockIDs(got))
	}
}

// A \section*{...}\label{...} prints no number: the label keeps the counter of the subsection
// before it and names that heading, not the starred one it follows.
func TestSectionLabelAfterAStarredHeadingNamesTheHeadingThatPrintsItsNumber(t *testing.T) {
	numbered := textBlock("numbered", "heading", int64(1), "6.3 High-dimensional outcomes")
	numbered["owner"] = int64(101)
	starred := textBlock("starred", "heading", int64(2), "Supplementary material")
	starred["owner"] = int64(102)
	records := []rec.Map{
		{"kind": "candidate", "seq": int64(10), "text": "6.3 High-dimensional outcomes", "entity": int64(1), "candidate": int64(101), "group": "paragraph"},
		{"kind": "candidate", "seq": int64(20), "text": "Supplementary material", "entity": int64(2), "candidate": int64(102), "group": "paragraph"},
	}
	e := &Evidence{blocks: []rec.Map{numbered, starred}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "SM", "counter": "subsection", "number": "6.3", "entity": int64(2),
		"target_entity": int64(1), "seq": int64(21)}

	if got := blockIDs(e.precedingHeadingBlocks(label)); len(got) != 1 || got[0] != "numbered" {
		t.Fatalf("label after a starred heading targets %v, want the heading printing 6.3", got)
	}
}

// emulateapj records an empty counter for a section label: its anchor names the counter.
func TestSectionLabelWithAnEmptyCounterUsesItsAnchor(t *testing.T) {
	heading := textBlock("numbered", "heading", int64(1), "3.1 Relation of Context")
	heading["owner"] = int64(101)
	records := []rec.Map{{"kind": "candidate", "seq": int64(10), "text": "3.1 Relation of Context", "entity": int64(1),
		"candidate": int64(101), "group": "paragraph"}}
	e := &Evidence{blocks: []rec.Map{heading}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "sec:holism", "counter": "", "anchor": "subsection.3.1", "number": "3.1",
		"entity": int64(2), "target_entity": int64(0), "seq": int64(21)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "numbered" {
		t.Fatalf("label with an empty counter targets %v, want the heading printing 3.1", got)
	}
	if got := sectionCounterOf(rec.Map{"counter": "", "anchor": "figure.1"}); got != "" {
		t.Fatalf("a figure anchor is the section counter %q", got)
	}
}

// emulateapj sets the number "1." in an hbox of its own, then the title in a paragraph flow: the
// label names the heading block that flow reads, not the (block-less) number box.
func TestSectionLabelAfterANumberBoxNamesTheTitleBlock(t *testing.T) {
	heading := textBlock("title", "heading", int64(1), "1. INTRODUCTION")
	heading["owner"] = int64(69)
	records := []rec.Map{
		{"kind": "flow_candidate", "seq": int64(87), "entity": int64(1), "owner": int64(68), "group": "hbox"},
		{"kind": "candidate", "seq": int64(88), "text": "1.  ", "entity": int64(1), "candidate": int64(64), "group": "hbox"},
		{"kind": "flow_candidate", "seq": int64(89), "entity": int64(1), "owner": int64(69), "group": "paragraph"},
		{"kind": "candidate", "seq": int64(90), "text": "INTRODUCTION", "entity": int64(1), "candidate": int64(65), "group": "paragraph"},
	}
	e := &Evidence{blocks: []rec.Map{heading}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "sec:intro", "counter": "", "anchor": "section.1", "number": "1",
		"entity": int64(2), "target_entity": int64(0), "seq": int64(92)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "title" {
		t.Fatalf("label after a number box targets %v, want the title block", got)
	}
}

// A class's \label{LastBibItem}, set after the bibliography, names the last entry printing its
// number, not the heading of the section the bibliography follows.
func TestLastBibItemNamesTheLastBibliographyEntry(t *testing.T) {
	heading := textBlock("ack", "heading", int64(5), "Acknowledgements")
	first := textBlock("b12", "paragraph", int64(6), "[12] A. Author, Title (2006).")
	last := textBlock("b13", "paragraph", int64(7), "[13] B. Author, Title (1990).")
	e := &Evidence{blocks: []rec.Map{heading, first, last}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "LastBibItem", "counter": "enumiv", "number": "13", "entity": int64(5),
		"target_entity": int64(5), "seq": int64(900)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "b13" {
		t.Fatalf("LastBibItem targets %v, want the entry printing [13]", got)
	}
	label["number"] = "99"
	if got := blockIDs(e.lastBibliographyItem(label)); len(got) != 0 {
		t.Fatalf("a number no entry prints targets %v, want none", got)
	}
}

// MNRAS's \label{lastpage} after the unnumbered references records the last section's number and
// anchor (section.4): it names the heading an earlier label with that anchor names, not the
// unnumbered heading it sits under (fifth cohort 1461).
func TestLabelAfterUnnumberedEndMatterNamesItsSection(t *testing.T) {
	section := textBlock("h4", "section", int64(10), "4 Conclusions")
	references := textBlock("refs", "heading", int64(20), "References")
	e := &Evidence{blocks: []rec.Map{section, references}}
	e.owners = e.inventory("flow_candidate", "owner")
	own := rec.Map{"kind": "label", "name": "sec:conc", "counter": "section", "number": "4", "anchor": "section.4",
		"entity": int64(10), "target_entity": int64(10), "seq": int64(5)}
	last := rec.Map{"kind": "label", "name": "lastpage", "counter": "section", "number": "4", "anchor": "section.4",
		"entity": int64(20), "target_entity": int64(20), "seq": int64(50)}
	e.records = []rec.Map{own, last}
	if got := blockIDs(e.targetBlocks(last)); len(got) != 1 || got[0] != "h4" {
		t.Fatalf("lastpage targets %v, want the section heading h4", got)
	}
}

// An author-year style prints no number (REVTeX's rmp, third cohort 599): LastBibItem names the
// block of the entry recorded with its number.
func TestLastBibItemNamesTheEntryRecordedWithItsNumber(t *testing.T) {
	heading := textBlock("refs", "heading", int64(5), "REFERENCES")
	entry := func(id string, number int64, text string) rec.Map {
		return rec.Map{"id": id, "type": "paragraph", "entity": int64(6),
			"content": []any{rec.Map{"type": "text", "text": text, "bibliography_entry": number}}}
	}
	e := &Evidence{blocks: []rec.Map{heading, entry("b1", 1, "Abel, 2001."), entry("b2", 2, "Zygo, 2011.")},
		records: []rec.Map{{"code": "bibliography_anchor", "entry": int64(1), "number": "1"},
			{"code": "bibliography_anchor", "entry": int64(2), "number": "2"}}}
	label := rec.Map{"name": "LastBibItem", "counter": "enumiv", "number": "2"}
	if got := blockIDs(e.lastBibliographyItem(label)); len(got) != 1 || got[0] != "b2" {
		t.Fatalf("LastBibItem targets %v, want the entry recorded as 2", got)
	}
}

// pageBreakAtHeading: emulateapj's number box and title, then the page breaks: the running head
// (set aside as page decoration) is recorded between the title and the label (papers2/0372).
func pageBreakAtHeading(between rec.Map) (*Evidence, rec.Map) {
	heading := textBlock("title", "heading", int64(17), "4. QUALITY CHECK")
	heading["owner"] = int64(69)
	body := textBlock("body", "paragraph", int64(17), "4 Kowalewski et al.")
	body["owner"] = int64(70)
	records := []rec.Map{
		{"kind": "flow_candidate", "seq": int64(87), "entity": int64(17), "owner": int64(68), "group": "hbox"},
		{"kind": "candidate", "seq": int64(88), "text": "4.  ", "entity": int64(17), "candidate": int64(64), "group": "hbox"},
		{"kind": "flow_candidate", "seq": int64(89), "entity": int64(17), "owner": int64(69), "group": "paragraph"},
		{"kind": "candidate", "seq": int64(90), "text": "QUALITY CHECK", "entity": int64(17), "candidate": int64(65), "group": "paragraph"},
		{"kind": "flow_candidate", "seq": int64(91), "entity": int64(17), "owner": int64(70), "group": between["group"]},
		between,
	}
	if between["candidate"] == int64(66) && between["text"] == " 4 Kowalewski et al. " {
		for position, char := range []string{"4", "K"} {
			records = append(records, rec.Map{"kind": "diagnostic", "code": "excluded_page_decoration", "seq": int64(93 + position),
				"candidate": int64(66), "position": int64(position + 1), "text": char, "source": "latex-outputpage"})
		}
	}
	e := &Evidence{blocks: []rec.Map{heading, body}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	return e, rec.Map{"name": "sec:refinement", "counter": "", "anchor": "section.4", "number": "4",
		"entity": int64(18), "target_entity": int64(0), "seq": int64(99)}
}

func TestSectionLabelAfterARunningHeadNamesTheHeading(t *testing.T) {
	e, label := pageBreakAtHeading(rec.Map{"kind": "candidate", "seq": int64(92), "text": " 4 Kowalewski et al. ",
		"entity": int64(17), "candidate": int64(66), "group": "hbox"})
	if got := blockIDs(e.precedingHeadingBlocks(label)); len(got) != 1 || got[0] != "title" {
		t.Fatalf("label after a running head targets %v, want the heading", got)
	}
}

// The same text as a real paragraph (no page decoration) between the heading and the label still
// pushes the number box out of the window: the label does not reach back past it.
func TestSectionLabelAfterAParagraphKeepsItsWindow(t *testing.T) {
	e, label := pageBreakAtHeading(rec.Map{"kind": "candidate", "seq": int64(92), "text": " 4 Kowalewski et al. ",
		"entity": int64(17), "candidate": int64(67), "group": "paragraph"})
	if got := blockIDs(e.precedingHeadingBlocks(label)); len(got) != 1 || got[0] != "body" {
		t.Fatalf("label after a paragraph targets %v, want the paragraph the window ends on", got)
	}
}

// An appendix letter is a whole number: the paragraph before the heading that opens with the same
// letter ("Combining ...", once the folio between them is set aside) is not the heading (papers4/1126).
func TestSectionLabelLetterIsNotAWordOpening(t *testing.T) {
	paragraph := textBlock("para", "paragraph", int64(168), "Combining all contributions")
	paragraph["owner"] = int64(70)
	records := []rec.Map{
		{"kind": "flow_candidate", "seq": int64(10), "entity": int64(168), "owner": int64(70), "group": "paragraph"},
		{"kind": "candidate", "seq": int64(11), "text": "Combining all contributions", "entity": int64(168), "candidate": int64(71), "group": "paragraph"},
		{"kind": "candidate", "seq": int64(12), "text": "38", "entity": int64(168), "candidate": int64(72), "group": "hbox"},
		{"kind": "diagnostic", "code": "excluded_page_decoration", "seq": int64(13), "candidate": int64(72), "position": int64(1), "text": "3"},
		{"kind": "candidate", "seq": int64(14), "text": "Appendix C: Red-noise", "entity": int64(179), "candidate": int64(73), "group": "paragraph"},
	}
	e := &Evidence{blocks: []rec.Map{paragraph}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "sec:appendix_integral", "counter": "section", "number": "C", "entity": int64(180),
		"target_entity": int64(179), "seq": int64(20)}
	if got := blockIDs(e.precedingHeadingBlocks(label)); len(got) != 0 {
		t.Fatalf("appendix label targets %v, want none from the window", got)
	}
}

// runInHeading: a \subsubsection set into the paragraph it opens (papers4/0977): its context (30)
// has no block, and the label comes before the paragraph ships, after the citation boxes set within it.
func runInHeading(heading string) (*Evidence, rec.Map, rec.Map) {
	before := textBlock("before", "paragraph", int64(31), "The paragraph before.")
	before["owner"] = int64(370)
	runIn := textBlock("runin", "paragraph", int64(31), heading)
	runIn["owner"] = int64(381)
	records := []rec.Map{
		{"kind": "flow_candidate", "seq": int64(1300), "entity": int64(31), "group": "paragraph", "owner": int64(370)},
		{"kind": "candidate", "seq": int64(1301), "text": "The paragraph before.", "entity": int64(31),
			"candidate": int64(360), "group": "paragraph"},
		{"kind": "flow_candidate", "seq": int64(1402), "entity": int64(31), "group": "hbox", "owner": int64(378)},
		{"kind": "candidate", "seq": int64(1403), "text": "14", "entity": int64(31), "candidate": int64(365), "group": "hbox"},
		{"kind": "flow_candidate", "seq": int64(1413), "entity": int64(31), "group": "paragraph", "owner": int64(381)},
		{"kind": "candidate", "seq": int64(1414), "text": heading, "entity": int64(31), "candidate": int64(368),
			"group": "paragraph"},
	}
	e := &Evidence{blocks: []rec.Map{before, runIn}, records: records}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "sec:justice_conflict", "counter": "subsubsection", "number": "3.2.3", "entity": int64(31),
		"target_entity": int64(30), "anchor": "subsubsection.3.2.3", "seq": int64(1395)}
	return e, runIn, label
}

func TestRunInSectionLabelNamesTheNextParagraphPrintingItsNumber(t *testing.T) {
	e, runIn, label := runInHeading("3.2.3Resolving Tension between Shared Purpose and the Quiet Sense of Order.")

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != rec.FetchString(runIn, "id") {
		t.Fatalf("run-in subsubsection label targets %v, want the paragraph printing 3.2.3", got)
	}
}

func TestRunInSectionLabelStaysUnresolvedWhenTheNextHeadingPrintsAnotherNumber(t *testing.T) {
	for _, heading := range []string{"3.2.4Another Heading.", "3.2.31Another Heading.", "Plain prose."} {
		e, _, label := runInHeading(heading)
		if got := blockIDs(e.targetBlocks(label)); len(got) != 0 {
			t.Errorf("label 3.2.3 before a paragraph %q targets %v, want none", heading, got)
		}
	}
}
