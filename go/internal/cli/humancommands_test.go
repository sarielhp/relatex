package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
)

// --report is an option of every command that reports on a thing, and records that the JSON is asked for;
// the commands that already print text (lint, doctor, synctex, bibclean) do not have it.
func TestReportIsAnOptionOfTheReportingCommands(t *testing.T) {
	for _, arguments := range [][]string{{"export", "D", "--output", "o", "--report"}, {"reconstruct", "D", "-o", "o", "--report"},
		{"validate", "D", "--report"}, {"inspect", "B", "--report"}, {"sync", "B", "--report"}, {"capabilities", "--report"},
		{"layers", "A", "--report"}, {"config", "--report"}, {"tag-pdf", "--report"}, {"alt", "draft", "--report"},
		{"alt", "review", "--report"}, {"alt", "accept", "--report"}, {"alt", "import", "f", "--report"}, {"alt", "set", "n", "--report"}} {
		current, err := parseLine(arguments...)
		if err != nil || !current.asReport {
			t.Errorf("relatex %v: report %v, %v", arguments, current != nil && current.asReport, err)
		}
	}
	for _, arguments := range [][]string{{"lint", "--accessibility", "--report"}, {"doctor", "--report"}, {"synctex", "--for", "p", "--report"}} {
		if _, err := parseLine(arguments...); err == nil {
			t.Errorf("relatex %v is read", arguments)
		}
	}
}

// Without --report a command prints its short text; with it, the JSON report as before.
func TestCapabilitiesPrintsTextUnlessReport(t *testing.T) {
	var text, report bytes.Buffer
	if status := Run([]string{"capabilities"}, &text, os.Stderr); status != 0 {
		t.Fatalf("status %d", status)
	}
	if strings.HasPrefix(text.String(), "{") || !strings.Contains(text.String(), "capabilities") {
		t.Errorf("capabilities prints %.80q", text.String())
	}
	if status := Run([]string{"capabilities", "--report"}, &report, os.Stderr); status != 0 || !strings.HasPrefix(report.String(), "{") {
		t.Errorf("--report prints %.80q (status %d)", report.String(), status)
	}
}

func TestExportHumanNamesTheFormatThePlaceAndTheLosses(t *testing.T) {
	clean := exportHuman("epub", ojson.Object{{"status", "success"}, {"file", "document.epub"}, {"losses", []any{}},
		{"directory", "/o/export-1"}})
	for _, want := range []string{"Wrote epub to /o/export-1", "main file: document.epub", "no losses"} {
		if !strings.Contains(clean, want) {
			t.Errorf("missing %q in %q", want, clean)
		}
	}
	lossy := exportHuman("html", ojson.Object{{"status", "degraded"}, {"directory", "/o/e"},
		{"losses", []any{ojson.Object{{"code", "math.image"}, {"count", int64(2)}}}}})
	for _, want := range []string{"status: degraded", "losses: 1 entry", "code=math.image count=2"} {
		if !strings.Contains(lossy, want) {
			t.Errorf("missing %q in %q", want, lossy)
		}
	}
}

func TestOutlineOpensTwoLevelsAndCountsTheRest(t *testing.T) {
	text := outline(ojson.Object{{"a", "x"}, {"list", []any{"p", "q"}}, {"many", make([]any, 12)}, {"empty", nil},
		{"inner", ojson.Object{{"deep", ojson.Object{{"deeper", ojson.Object{{"z", "1"}}}}}}}}, "", 1)
	for _, want := range []string{"a: x\n", "list: p, q\n", "many: 12 items\n", "empty: none\n", "inner:\n  deep: 1 field\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
}
