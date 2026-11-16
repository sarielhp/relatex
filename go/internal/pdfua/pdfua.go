// Package pdfua is what the tagged PDF's report shares (decisions 097 and 105): veraPDF's verdict
// on a PDF against PDF/UA-2 (verapdf.go) and the report's diagnostics, which the conversion and the
// accessibility report read back. Route 2 (internal/pdftag) makes the file; route 1, the document
// rebuilt with LaTeX's tagging, was removed on 2026-11-03 (git tag v0.1.55 holds its last version).
package pdfua

import (
	"fmt"
	"strings"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Diagnostic is one finding of the tagged PDF for the conversion's diagnostics.
type Diagnostic struct {
	Code, Message string
	Magnitude     int64
}

// Diagnostics are the verdict's findings: pdfua.invalid when veraPDF rejects the file,
// pdfua.unverified when there is no verdict.
func (v Validation) Diagnostics() []Diagnostic {
	switch v.Status {
	case "failed":
		return []Diagnostic{{"pdfua.invalid", "veraPDF rejects the tagged PDF as PDF/UA-2: " + failedRules(v), int64(len(v.Failures))}}
	case "unavailable":
		return []Diagnostic{{"pdfua.unverified", "the tagged PDF was not validated: " + v.Reason, 0}}
	}
	return nil
}

func failedRules(validation Validation) string {
	parts := []string{}
	for _, failure := range validation.Failures[:min(4, len(validation.Failures))] {
		parts = append(parts, fmt.Sprintf("%v-%v %v (%v)", failure.Get("clause"), failure.Get("test"), failure.Get("description"),
			failure.Get("failed_checks")))
	}
	return strings.Join(parts, "; ")
}

// DiagnosticList is the findings as the export report lists them.
func DiagnosticList(found []Diagnostic) []any {
	list := []any{}
	for _, diagnostic := range found {
		list = append(list, ojson.Object{{"code", diagnostic.Code}, {"message", diagnostic.Message}, {"magnitude", diagnostic.Magnitude}})
	}
	return list
}

// Diagnostics reads the diagnostics back from a report (written now or restored from the cache).
func Diagnostics(report ojson.Object) []Diagnostic {
	found := []Diagnostic{}
	list, _ := report.Get("diagnostics").([]any)
	for _, item := range list {
		entry := asObject(item)
		magnitude, _ := entry["magnitude"].(int64)
		if number, ok := entry["magnitude"].(float64); ok {
			magnitude = int64(number)
		}
		found = append(found, Diagnostic{Code: rec.ToS(entry["code"]), Message: rec.ToS(entry["message"]), Magnitude: magnitude})
	}
	return found
}

func asObject(value any) map[string]any {
	switch typed := value.(type) {
	case ojson.Object:
		result := map[string]any{}
		for _, pair := range typed {
			result[pair.Key()] = pair.Value()
		}
		return result
	case map[string]any:
		return typed
	}
	return map[string]any{}
}
