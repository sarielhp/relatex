package alttext

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcealt"
)

func span(document rec.Map, node string) rec.Map {
	return rec.AsMap(rec.FetchList(rec.AsMap(rec.FetchMap(document, "nodes")[node]), "content")[0])
}

// TestApplyKeepsTheSourcesDescription: a figure the source describes (decision 096) keeps the
// source's text over any sidecar entry, which still counts as matched; the drafter and the review
// leave it alone.
func TestApplyKeepsTheSourcesDescription(t *testing.T) {
	document := figureDocument()
	sourcealt.Apply(span(document, "p2"), sourcealt.Description{Alt: "From the source.", Long: "A longer text."})
	sidecar, err := figdesc.Parse("[figs.disk]\nalt = \"Reviewed text.\"\nprovenance = \"reviewed\"\n")
	if err != nil {
		t.Fatal(err)
	}
	summary := figdesc.Apply(document, sidecar)
	if summary.Source != 1 || summary.Reviewed != 0 || len(summary.Unmatched) != 0 || summary.Applied != 0 {
		t.Errorf("summary %+v", summary)
	}
	disk := span(document, "p2")
	if disk["alt"] != "From the source." || disk["alt_provenance"] != figdesc.Source || disk["long_description"] != "A longer text." {
		t.Errorf("disk %v", disk)
	}
	states := States(figdesc.Figures(document), sidecar)
	if states[0].Class != "done" || !strings.Contains(states[0].Label, "source") {
		t.Errorf("review state %+v", states[0])
	}
	outcome, err := (&Drafter{Command: []string{"/bin/false"}, Limit: 1}).Run(document, sidecar)
	if err != nil || len(outcome.Kept) != 1 || outcome.Kept[0] != "disk" {
		t.Errorf("drafter %+v %v", outcome, err)
	}
}

// TestApplyCarriesLongAndDecorativeEntries: the sidecar's long text becomes the long description
// and decorative = true an empty alternative.
func TestApplyCarriesLongAndDecorativeEntries(t *testing.T) {
	document := figureDocument()
	sidecar, err := figdesc.Parse("[figs.disk]\nalt = \"A disk.\"\nlong = \"A disk of radius r\\n around p.\"\n" +
		"[figs.plot]\ndecorative = true\n")
	if err != nil {
		t.Fatal(err)
	}
	if summary := figdesc.Apply(document, sidecar); summary.Author != 2 || summary.Applied != 2 {
		t.Errorf("summary %+v", summary)
	}
	disk, plot := span(document, "p2"), span(document, "p5")
	if disk["alt"] != "A disk." || disk["long_description"] != "A disk of radius r around p." || disk["alt_provenance"] != figdesc.Author {
		t.Errorf("disk %v", disk)
	}
	if plot["decorative"] != true || plot["alt"] != nil || plot["alt_provenance"] != figdesc.Author {
		t.Errorf("plot %v", plot)
	}
	if _, err := figdesc.Parse("[figs.disk]\ndecorative = \"yes\"\n"); err == nil || !strings.Contains(err.Error(), "true or false") {
		t.Errorf("a decorative string: %v", err)
	}
}

// TestADerivedChessDescriptionYieldsToTheAuthorsEntry: a chess diagram's derived description
// (decision 163) is kept over a model draft, which is neither applied nor made, and the author's
// entry replaces it.
func TestADerivedChessDescriptionYieldsToTheAuthorsEntry(t *testing.T) {
	derived := sourcealt.Description{Alt: "Chess diagram, position in FEN: 8/8/8/8/8/8/8/K7", Inferred: true}
	for _, test := range []struct{ entry, alt, provenance string }{
		{"[figs.disk]\nalt = \"Draft.\"\nprovenance = \"model\"\n", derived.Alt, figdesc.Inferred},
		{"[figs.disk]\nalt = \"A lone king.\"\n", "A lone king.", figdesc.Author},
	} {
		document := figureDocument()
		sourcealt.Apply(span(document, "p2"), derived)
		sidecar, err := figdesc.Parse(test.entry)
		if err != nil {
			t.Fatal(err)
		}
		figdesc.Apply(document, sidecar)
		disk := span(document, "p2")
		if disk["alt"] != test.alt || disk["alt_provenance"] != test.provenance {
			t.Errorf("entry %q: %v", test.entry, disk)
		}
		if test.provenance != figdesc.Inferred {
			continue
		}
		states := States(figdesc.Figures(document), sidecar)
		if states[0].Class != "done" || !strings.Contains(states[0].Label, "chess") {
			t.Errorf("review state %+v", states[0])
		}
		outcome, err := (&Drafter{Command: []string{"/bin/false"}, Limit: 1}).Run(document, sidecar)
		if err != nil || len(outcome.Kept) != 1 || outcome.Kept[0] != "disk" {
			t.Errorf("drafter %+v %v", outcome, err)
		}
	}
}

// TestRelatexAltYieldsToTheAuthorsEntry: a figure's \relatexalt (decision 125, amendment 2) is its
// description until the author's or a reviewed entry replaces it, details included; a model draft
// does not, and the drafter and the review leave the figure alone while the macro describes it.
func TestRelatexAltYieldsToTheAuthorsEntry(t *testing.T) {
	macro := sourcealt.Description{Alt: "A disk.", Long: "A disk of radius r.", Details: "p is its centre.", Macro: true}
	cases := []struct {
		entry, alt, provenance, details string
		source                          int
	}{
		{"", "A disk.", figdesc.SourceMacro, "p is its centre.", 1},
		{"[figs.disk]\nalt = \"Draft.\"\nprovenance = \"model\"\n", "A disk.", figdesc.SourceMacro, "p is its centre.", 1},
		{"[figs.disk]\nalt = \"Mine.\"\n", "Mine.", figdesc.Author, "", 0},
		{"[figs.disk]\nalt = \"Checked.\"\nprovenance = \"reviewed\"\n", "Checked.", figdesc.Reviewed, "", 0},
	}
	for _, test := range cases {
		document := figureDocument()
		sourcealt.Apply(span(document, "p2"), macro)
		sidecar, err := figdesc.Parse(test.entry)
		if err != nil {
			t.Fatal(err)
		}
		summary := figdesc.Apply(document, sidecar)
		disk := span(document, "p2")
		if disk["alt"] != test.alt || disk["alt_provenance"] != test.provenance || rec.ToS(disk[figdesc.DetailsField]) != test.details ||
			summary.Source != test.source || len(summary.Drafts) != 0 {
			t.Errorf("entry %q: %v, summary %+v", test.entry, disk, summary)
		}
		if test.source == 0 {
			continue
		}
		states := States(figdesc.Figures(document), sidecar)
		if states[0].Class != "done" || !strings.Contains(states[0].Label, `\relatexalt`) {
			t.Errorf("entry %q: review state %+v", test.entry, states[0])
		}
		outcome, err := (&Drafter{Command: []string{"/bin/false"}, Limit: 1}).Run(document, sidecar)
		if err != nil || len(outcome.Kept) != 1 || outcome.Kept[0] != "disk" {
			t.Errorf("entry %q: drafter %+v %v", test.entry, outcome, err)
		}
	}
}
