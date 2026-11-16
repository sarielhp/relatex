package stylemodel

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Decision 104's font zoo: a condensed series or Latin Modern's upright italic stopped a conversion.
func TestWidthQualifiedSeriesAndUprightItalic(t *testing.T) {
	style := rec.Map{"source": "nfss-font-id", "font_id": int64(17), "font_name": "DejaVuSansCondensed-Bold-tlf-t1",
		"size_sp": int64(655360), "encoding": "T1", "family": "DejaVuSans-TLF", "series": "bc", "shape": "ui",
		"generic": "sans-serif"}
	properties := Properties(style)
	if properties["fontWeight"] != "700" || properties["fontStyle"] != "normal" {
		t.Fatalf("bc/ui read as %v", properties)
	}
	for series, weight := range map[string]int{"c": 400, "sbc": 600, "mx": 400, "db": 600, "bx": 700} {
		if Weights[series] != weight {
			t.Errorf("series %s weighs %d, want %d", series, Weights[series], weight)
		}
	}
}
