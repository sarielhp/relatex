package a11ylint

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// labelledExport is the export fixture with a label on the figure and the sha the report gives its
// caption (decision 188).
func labelledExport(t *testing.T, voided bool) (Paper, string) {
	const sha = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	document := exportDocument()
	rec.MapOf(document["labels"])["fig:chart"] = rec.Map{"target": "figp", "number": "1.1"}
	report := exportReport()
	item := rec.MapOf(rec.SliceOf(rec.MapOf(rec.SliceOf(report["structural"])[1])["items"])[0])
	item["sha256"] = sha
	if voided {
		item["attestation"] = "voided: the caption changed since it was attested"
	}
	paper := Paper{Files: oneFile(t, "main.tex", exportSource), SourceMap: exportWords()}
	return paper.WithExport(report, document), sha
}

// The colour item of an export gives the wording to say it with more than colour and the exact
// attestation line, with the caption's sha and the float's label; it never writes the line itself
// (no suggestion), so nothing is attested without the author.
func TestColourItemGivesTheAttestationLine(t *testing.T) {
	paper, sha := labelledExport(t, false)
	for _, item := range Lint(paper, Options{}) {
		if item.Kind != "colour" {
			continue
		}
		for _, want := range []string{"shown as dashed red line, red circles", `"colour_captions": {"fig:chart": "` + sha + `"}`,
			`%% relatex colour_captions: fig:chart = "` + sha + `"`, "voids it"} {
			if !strings.Contains(item.Fix, want) {
				t.Errorf("fix lacks %q: %s", want, item.Fix)
			}
		}
		if item.Suggestion != "" {
			t.Errorf("an attestation is never suggested for patching: %q", item.Suggestion)
		}
		return
	}
	t.Fatal("no colour item")
}

// A voided attestation is said to be void, and the new sha replaces the old one.
func TestVoidedAttestationIsSaid(t *testing.T) {
	paper, _ := labelledExport(t, true)
	for _, item := range Lint(paper, Options{}) {
		if item.Kind == "colour" && !strings.Contains(item.Fix, "attestation is void") {
			t.Errorf("fix: %s", item.Fix)
		}
	}
}

// A table inside a figure gets the exact hint line, key and position (decision 188): the float's
// label, else its number.
func TestFigureTableGetsAHintLine(t *testing.T) {
	paper, _ := labelledExport(t, false)
	for _, item := range Lint(paper, Options{}) {
		if item.Kind == "table" && strings.Contains(item.Fix, "Figure") || item.Kind == "table" && item.Line == 4 {
			if !strings.Contains(item.Fix, `%% relatex tables: fig:chart#1 = "header_rows=1"`) {
				t.Errorf("fix: %s", item.Fix)
			}
		}
	}
}

// A hint keyed label#N is quiet when the paper defines the label; one whose label is missing is an item.
func TestFigureTableHintKeyIsChecked(t *testing.T) {
	paper := hintPaper(t, "%% relatex tables:\n%%   fig:a#2 = \"header_rows=1\"\n%%   fig:gone#1 = \"header_rows=1\"\n"+
		"%%   3#1 = \"header_rows=1\"\n\\begin{figure}\\caption{x}\\label{fig:a}\\end{figure}\n")
	items := Lint(paper, Options{})
	if hintOf(t, items, `"fig:a#2"`) != nil || hintOf(t, items, `"3#1"`) != nil {
		t.Error("a valid figure table hint was flagged")
	}
	if hintOf(t, items, `table hint for "fig:gone#1", neither a label`) == nil {
		t.Error("no item for a position of an undefined label")
	}
}
