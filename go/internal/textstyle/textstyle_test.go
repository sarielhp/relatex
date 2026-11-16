package textstyle

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stylemodel"
)

func style(size int64) rec.Map {
	return rec.Map{"source": "nfss-font-id", "font_id": int64(17), "font_name": "LMRoman10-Regular", "size_sp": size,
		"encoding": "TU", "family": "lmr", "series": "m", "shape": "n", "generic": "serif"}
}

func document(spans ...rec.Map) rec.Map {
	content := []any{}
	for _, span := range spans {
		content = append(content, span)
	}
	return rec.Map{"nodes": rec.Map{"p1": rec.Map{"content": content}}}
}

func TestBaseSizeIsTheSizeOfMostCharacters(t *testing.T) {
	body := rec.Map{"type": "text", "text": "the body of the paragraph", "style": style(720_896)}
	title := rec.Map{"type": "text", "text": "Title", "style": style(943_718)}
	math := rec.Map{"type": "math", "style": style(1_000_000)}
	if base := stylemodel.BaseSize(document(title, body, math)); base != 720_896 {
		t.Fatalf("base %d", base)
	}
	if base := stylemodel.BaseSize(document()); base != stylemodel.DefaultBase {
		t.Fatalf("default base %d", base)
	}
	// A tie goes to the smaller size.
	tie := document(rec.Map{"type": "text", "text": "abc", "style": style(786_432)}, rec.Map{"type": "text", "text": "xyz", "style": style(655_360)})
	if base := stylemodel.BaseSize(tie); base != 655_360 {
		t.Fatalf("tie base %d", base)
	}
}

func TestCSSSizesRelativeToTheBaseOrInPixels(t *testing.T) {
	if css := stylemodel.CSS(style(786_432), 655_360); css != "font-weight:400;font-style:normal;font-variant-caps:normal;font-size:1.200000rem;font-family:serif" {
		t.Fatalf("relative css %q", css)
	}
	if css := stylemodel.CSS(style(655_360), 0); css != "font-weight:400;font-style:normal;font-variant-caps:normal;font-size:13.283520px;font-family:serif" {
		t.Fatalf("pixel css %q", css)
	}
}

// fontawesome5 selects its faces with its own series names (lipics-v2021's \faIcon[regular]).
func TestFontAwesomeSeriesNamesHaveWeights(t *testing.T) {
	icon := style(655_360)
	icon["family"], icon["series"] = "fontawesomefree", "regular"
	if weight := stylemodel.Properties(icon)["fontWeight"]; weight != "400" {
		t.Fatalf("regular weight %v", weight)
	}
	icon["series"] = "solid"
	if weight := stylemodel.Properties(icon)["fontWeight"]; weight != "900" {
		t.Fatalf("solid weight %v", weight)
	}
}

// mathdesign without its expert fonts sets small caps as its faked fsc shape (cohort 5, 1346);
// an unknown shape still fails.
func TestMathdesignFakedSmallCapsAreSmallCaps(t *testing.T) {
	caps := style(655_360)
	caps["family"], caps["shape"] = "mdbch", "fsc"
	if variant := stylemodel.Properties(stylemodel.Validate(caps))["fontVariantCaps"]; variant != "small-caps" {
		t.Fatalf("fsc variant %v", variant)
	}
	caps["shape"] = "fic"
	if values := stylemodel.Properties(stylemodel.Validate(caps)); values["fontStyle"] != "italic" || values["fontVariantCaps"] != "small-caps" {
		t.Fatalf("fic %v", values)
	}
	caps["shape"] = "xsc"
	defer func() {
		if recover() == nil {
			t.Fatal("unknown shape xsc was accepted")
		}
	}()
	stylemodel.Validate(caps)
}
