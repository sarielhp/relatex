package pdfua

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/ojson"
)

// veraPDF, the PDF/UA validator (Java), is an external program the product uses when it is there:
// RELATEX_VERAPDF names it, else the configuration's tools.verapdf, else verapdf on PATH (decision 129,
// amendment 4). Without it the PDF is reported unvalidated.

// Validation is veraPDF's verdict on a PDF against the PDF/UA-2 profile.
type Validation struct {
	Status   string // passed, failed or unavailable
	Profile  string
	Program  string
	Failures []ojson.Object // each failed rule: clause, test, failed checks, description
	Checks   []Check        // the failed checks behind them, with their contexts
	Reason   string         // why there is no verdict
}

// Check is one failed check of a rule: what it saw and where.
type Check struct {
	Clause  string
	Test    int64
	Status  string
	Context string
	Message string
}

// validator is the veraPDF program, or "": the lookup every configured tool uses, so a variable or a
// configured path that names no program finds nothing.
func validator() string {
	path, _ := command.Locate("verapdf")
	return path
}

// Validate runs veraPDF's PDF/UA-2 profile on the PDF.
func Validate(pdf string) Validation {
	program := validator()
	if program == "" {
		return Validation{Status: "unavailable", Reason: "veraPDF not found (set tools.verapdf in the configuration, RELATEX_VERAPDF, or put verapdf on PATH)"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	run := exec.CommandContext(ctx, program, "--flavour", "ua2", "--format", "json", pdf)
	run.Env = command.Environment(nil)
	output, err := run.Output()
	validation := readValidation(output)
	validation.Program = program
	if validation.Status == "" {
		validation.Status, validation.Reason = "unavailable", fmt.Sprintf("veraPDF gave no report: %v", err)
	}
	return validation
}

// readValidation reads veraPDF's JSON report.
func readValidation(output []byte) Validation {
	var report struct {
		Report struct {
			Jobs []struct {
				ValidationResult json.RawMessage `json:"validationResult"`
			} `json:"jobs"`
		} `json:"report"`
	}
	if json.Unmarshal(output, &report) != nil || len(report.Report.Jobs) == 0 {
		return Validation{}
	}
	type check struct {
		Status  string `json:"status"`
		Context string `json:"context"`
		Message string `json:"errorMessage"`
	}
	type rule struct {
		Clause       string  `json:"clause"`
		TestNumber   int64   `json:"testNumber"`
		Status       string  `json:"status"`
		FailedChecks int64   `json:"failedChecks"`
		Description  string  `json:"description"`
		Checks       []check `json:"checks"`
	}
	type result struct {
		Compliant   *bool  `json:"compliant"`
		ProfileName string `json:"profileName"`
		Details     struct {
			RuleSummaries []rule `json:"ruleSummaries"`
		} `json:"details"`
	}
	results := []result{}
	raw := report.Report.Jobs[0].ValidationResult
	if json.Unmarshal(raw, &results) != nil {
		single := result{}
		if json.Unmarshal(raw, &single) != nil {
			return Validation{}
		}
		results = []result{single}
	}
	if len(results) == 0 || results[0].Compliant == nil {
		return Validation{}
	}
	validation := Validation{Status: "failed", Profile: results[0].ProfileName, Failures: []ojson.Object{}}
	if *results[0].Compliant {
		validation.Status = "passed"
	}
	for _, summary := range results[0].Details.RuleSummaries {
		if summary.Status == "passed" {
			continue
		}
		validation.Failures = append(validation.Failures, ojson.Object{{"clause", summary.Clause}, {"test", summary.TestNumber},
			{"failed_checks", summary.FailedChecks}, {"description", strings.TrimSpace(summary.Description)}})
		kept := 0
		for _, detail := range summary.Checks {
			if detail.Status == "passed" || kept >= maxChecks {
				continue
			}
			kept++
			validation.Checks = append(validation.Checks, Check{Clause: summary.Clause, Test: summary.TestNumber,
				Status: detail.Status, Context: detail.Context, Message: strings.TrimSpace(detail.Message)})
		}
	}
	return validation
}

// maxChecks bounds the failed checks kept per rule: a diagnostic names the figure and the font,
// so its few checks are enough.
const maxChecks = 20

// Report is the verdict as the export report writes it.
func (v Validation) Report() ojson.Object {
	report := ojson.Object{{"status", v.Status}}
	if v.Profile != "" {
		report = append(report, ojson.Pair{"profile", v.Profile})
	}
	if v.Reason != "" {
		report = append(report, ojson.Pair{"reason", v.Reason})
	}
	if v.Status == "failed" {
		failures := []any{}
		for _, failure := range v.Failures {
			failures = append(failures, failure)
		}
		report = append(report, ojson.Pair{"failed_rules", failures})
	}
	return report
}
