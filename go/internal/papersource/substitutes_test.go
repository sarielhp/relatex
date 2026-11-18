package papersource

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestRecordCompatibilityReportsSubstitutesAndIpeGraphics(t *testing.T) {
	diags := diagnostics.New(rec.Map{
		"codes": rec.Map{
			"source.package_substituted": rec.Map{"level": "warning"},
			"figure.ipe_converted":       rec.Map{"level": "warning"},
			"figure.ipe_placeholder":     rec.Map{"level": "alert"},
		},
	}, nil, false)
	source := &Source{
		diagnostics: diags,
		report: rec.Map{
			"compatibility": []any{rec.Map{"shim": "psfig", "cause": "missing"}},
			"ipe_graphics":  []any{rec.Map{"path": "a.ipe", "how": "EPS"}, rec.Map{"path": "b.ipe", "how": "placeholder"}},
			"results":       rec.Map{},
		},
	}
	source.recordCompatibility()
	codes := []string{}
	for _, entry := range diags.Entries() {
		codes = append(codes, rec.ToS(rec.AsMap(entry)["code"])+"/"+rec.ToS(rec.AsMap(entry)["level"]))
	}
	want := []string{"source.package_substituted/warning", "figure.ipe_converted/warning", "figure.ipe_placeholder/alert"}
	if !slices.Equal(codes, want) {
		t.Errorf("codes = %v, want %v", codes, want)
	}
}
