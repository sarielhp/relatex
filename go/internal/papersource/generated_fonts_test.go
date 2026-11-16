package papersource

import (
	"testing"

	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestRecordCompatibilityReportsGeneratedFontsDiagnostic(t *testing.T) {
	diags := diagnostics.New(rec.Map{
		"codes": rec.Map{
			"fonts.generated_metrics": rec.Map{"level": "whatever"},
		},
	}, nil, false)
	source := &Source{
		diagnostics: diags,
		report: rec.Map{
			"generated_fonts": []any{"larm1000", "larm1200"},
			"results":         rec.Map{},
		},
	}
	source.recordCompatibility()
	entries := diags.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 diagnostic entry, got %v", entries)
	}
	entry := rec.AsMap(entries[0])
	if rec.ToS(entry["code"]) != "fonts.generated_metrics" || rec.ToS(entry["level"]) != "whatever" {
		t.Fatalf("unexpected entry: %v", entry)
	}
}
