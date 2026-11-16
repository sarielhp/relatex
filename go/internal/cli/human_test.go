package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/engine"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

func sampleResult(t *testing.T, status string, errors, warnings int) ojson.Object {
	t.Helper()
	attempt := t.TempDir()
	if err := os.WriteFile(filepath.Join(attempt, "run.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	if err := os.Mkdir(filepath.Join(output, "typst-source"), 0o755); err != nil {
		t.Fatal(err)
	}
	var lines []any
	for index := 0; index < errors; index++ {
		lines = append(lines, fmt.Sprintf("paper: error[code.%d]: problem %d", index, index))
	}
	lines = append(lines, "paper: warning[language.undeclared]: no language")
	return ojson.Object{{"status", status}, {"directory", attempt}, {"output", output}, {"outputs", []any{filepath.Join(output, "typst"), filepath.Join(output, "html")}},
		{"product", filepath.Join(output, "product")},
		{"configuration", ojson.Object{{"formats", []any{"markdown", "html"}}}},
		{"diagnostics", ojson.Object{{"fatal", int64(0)}, {"error", int64(errors)}, {"alert", int64(0)}, {"warning", int64(warnings)}}},
		{"diagnostic_lines", lines}}
}

// A successful conversion ends with one line: where the requested formats are, the counts and the report.
func TestHumanSummaryNamesTheRequestedPlaceTheCountsAndTheReport(t *testing.T) {
	result := sampleResult(t, "diagnostic", 0, 1)
	text := humanSummary(result, false)
	for _, want := range []string{"Conversion succeeded: ", rec.ToS(result.Get("output")), "0 errors, 1 warning", "report ", "run.json"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary %q lacks %q", text, want)
		}
	}
	if strings.Contains(text, "\n") {
		t.Errorf("the summary is one line: %q", text)
	}
	if strings.Contains(text, "product") {
		t.Errorf("the summary names the deliverables, not the product: %q", text)
	}
}

// One deliverable is named itself; nothing delivered names the product.
func TestHumanSummaryNamesTheOneDeliverable(t *testing.T) {
	result := sampleResult(t, "diagnostic", 0, 1).Set("outputs", []any{"/x/output/typst"})
	if text := humanSummary(result, false); !strings.Contains(text, ": /x/output/typst (") {
		t.Errorf("summary %q", text)
	}
	none := sampleResult(t, "diagnostic", 0, 1).Set("outputs", []any{}).Set("output", nil)
	if text := humanSummary(none, false); !strings.Contains(text, "product (") {
		t.Errorf("summary %q", text)
	}
}

func TestHumanSummaryOfAFailedConversion(t *testing.T) {
	text := humanSummary(sampleResult(t, "failed", 2, 1), false)
	if !strings.HasPrefix(text, "Conversion failed: paper: error[code.0]: problem 0") || !strings.Contains(text, "2 errors, 1 warning") {
		t.Errorf("summary %q", text)
	}
}

// At most ten error-level diagnostics are listed, then how many more the report holds; warnings are only counted.
func TestHumanErrorsAreShortListedWithTheRest(t *testing.T) {
	result := sampleResult(t, "failed", 14, 3)
	text := humanErrors(result, "REPORT", false)
	if lines := strings.Split(strings.TrimSpace(text), "\n"); len(lines) != maxErrorLines+1 || lines[maxErrorLines] != "and 4 more in REPORT" {
		t.Errorf("errors:\n%s", text)
	}
	if strings.Contains(text, "warning[") {
		t.Errorf("a warning is listed:\n%s", text)
	}
	if humanErrors(sampleResult(t, "diagnostic", 0, 1), "REPORT", false) != "" {
		t.Error("a run without errors lists none")
	}
}

// --report keeps the JSON report on stdout; without it a conversion prints for a person.
func TestReportIsAnOptionOfEveryConversionCommand(t *testing.T) {
	for _, arguments := range [][]string{{"--report"}, {"convert", "--report"}, {"replay", "attempt", "--report"}} {
		if _, err := parseLine(arguments...); err != nil {
			t.Errorf("relatex %v: %v", arguments, err)
		}
	}
	if options, _ := parsedConvert(t, "--report", "./paper"); options["report"] != true {
		t.Errorf("--report not recorded: %v", options)
	}
	if options, _ := parsedConvert(t, "./paper"); options["report"] != nil {
		t.Errorf("a report without --report: %v", options)
	}
}

// The default output of a conversion leaves out the engine stage's per-build lines, failed ones too:
// they stay in the run report and the logs.
func TestHumanOutputSilencesTheEngineBuildLines(t *testing.T) {
	quiet, failures := engine.Quiet, engine.QuietFailures
	defer func() { engine.Quiet, engine.QuietFailures, conversionHuman = quiet, failures, nil }()
	startHuman("paper").finish()
	if !engine.Quiet || !engine.QuietFailures {
		t.Errorf("Quiet %v, QuietFailures %v", engine.Quiet, engine.QuietFailures)
	}
}
