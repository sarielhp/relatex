package textstyle

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A family observed through NFSS (not a PostScript or Computer Modern face) supplies the face of
// a direct font named by its suffix; without the observation the font stays unstyled.
func TestUnobservedTeXFontUsesKnownFamilyAndFaceSuffix(t *testing.T) {
	base := rec.Map{"source": "nfss-font-id", "font_id": int64(21), "font_name": "LMRoman12-Regular", "size_sp": int64(786_432),
		"encoding": "TU", "family": "abc", "series": "m", "shape": "n", "generic": "serif"}
	row := rec.Map{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(18), "font_name": "abcb",
		"size_sp": int64(720_896)}
	want := rec.Map{"source": "unobserved-tex-font-id", "font_id": int64(18), "font_name": "abcb", "size_sp": int64(720_896),
		"encoding": "unknown", "family": "abc", "series": "b", "shape": "n", "generic": "serif"}
	if got := UnobservedStyle(row, Catalog{21: base}); !rec.Equal(got, want) {
		t.Fatalf("unobserved style = %v", got)
	}
	if got := UnobservedStyle(row, Catalog{}); got != nil {
		t.Fatalf("style without a known family = %v", got)
	}
}

func TestUnobservedPostScriptFontReadsItsFaceName(t *testing.T) {
	faces := map[string][2]string{"ptmr": {"m", "n"}, "ptmb": {"b", "n"}, "ptmrc": {"m", "sc"},
		"ptmbc": {"b", "sc"}, "phvbc": {"b", "sc"}, "pcrro": {"m", "sl"}}
	for name, want := range faces {
		row := rec.Map{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(54), "font_name": name,
			"size_sp": int64(786_432)}
		got := UnobservedStyle(row, Catalog{})
		if rec.FetchString(got, "font_name") != name || rec.FetchString(got, "series") != want[0] ||
			rec.FetchString(got, "shape") != want[1] || rec.FetchString(got, "family") != name[:3] {
			t.Fatalf("%s style = %v", name, got)
		}
	}
	row := rec.Map{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(54), "font_name": "unkb",
		"size_sp": int64(786_432)}
	if got := UnobservedStyle(row, Catalog{}); got != nil {
		t.Fatalf("unknown PostScript family = %v", got)
	}
}

func TestUnobservedComputerModernFontReadsItsFaceName(t *testing.T) {
	row := rec.Map{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(20), "font_name": "cmti10",
		"size_sp": int64(786_432)}
	want := rec.Map{"source": "unobserved-tex-font-id", "font_id": int64(20), "font_name": "cmti10", "size_sp": int64(786_432),
		"encoding": "unknown", "family": "cmr", "series": "m", "shape": "it", "generic": "serif"}
	if got := UnobservedStyle(row, Catalog{}); !rec.Equal(got, want) {
		t.Fatalf("cmti10 style = %v", got)
	}
	row["font_name"] = "cmti"
	if got := UnobservedStyle(row, Catalog{}); got != nil {
		t.Fatalf("a face without its design size = %v", got)
	}
}
