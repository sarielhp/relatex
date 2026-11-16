package svgoutline

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// fontNameSVG is a page with one use of "N" at (30, 40).
var fontNameSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` +
	`<defs><path id="font_3_17" d="M0 0H.6V.7H0Z"/></defs>` +
	`<use data-text="N" xlink:href="#font_3_17" transform="matrix(12,0,0,-12,30,40)"/></svg>`)

func fontNamePaint(font string) rec.Map {
	return rec.Map{"font": font, "glyph": "17", "text": "N", "operation": "fill_text",
		"matrix": []any{12.0, 0.0, 0.0, -12.0, 30.0, 40.0}, "baseline": []any{30.0, 40.0}, "advance": []any{8.16, 0.0},
		"index": int64(0)}
}

func fontNameGlyph(font string) rec.Map {
	return rec.Map{"id": "p2-g1", "line": int64(1), "text": "N", "font": font,
		"baseline": []any{30.0, 40.0}, "metric_quad": []any{30.0, 31.6, 38.16, 31.6, 30.0, 40.0, 38.16, 40.0}}
}

// MuPDF's structured text drops everything through the first "+" of a paint trace's font name, not
// only a six-letter subset tag: a figure's "CIDFont+F1" is "F1" there (papers3/0726).
func TestFontNameDropsAnyPrefixThroughThePlus(t *testing.T) {
	if paintFont("CIDFont+F1") != "F1" || paintFont("ABCDEF+CMR10") != "CMR10" || paintFont("Type3 (280 0 R)") != "Type3 (280 0 R)" {
		t.Fatalf("paintFont: %q %q", paintFont("CIDFont+F1"), paintFont("ABCDEF+CMR10"))
	}
	if uses := boundUses(t, fontNameSVG, []rec.Map{fontNamePaint("CIDFont+F1")}, []rec.Map{fontNameGlyph("F1")}); len(uses) != 1 {
		t.Fatalf("bound uses %v", uses)
	}
}

// It drops it once: a subset of that font, "BHWZGQ+CIDFont+F1" in the paint trace, is "CIDFont+F1"
// there, and the structured text's name is compared as it is (papers/0217).
func TestFontNameDropsThePrefixOnce(t *testing.T) {
	if paintFont("BHWZGQ+CIDFont+F1") != "CIDFont+F1" {
		t.Fatalf("paintFont: %q", paintFont("BHWZGQ+CIDFont+F1"))
	}
	paint := []rec.Map{fontNamePaint("BHWZGQ+CIDFont+F1")}
	if uses := boundUses(t, fontNameSVG, paint, []rec.Map{fontNameGlyph("CIDFont+F1")}); len(uses) != 1 {
		t.Fatalf("bound uses %v", uses)
	}
	message := bindFailure(func() { Bind(fontNameSVG, paint, []rec.Map{fontNameGlyph("F1")}) })
	if message != "Missing or ambiguous native glyph paint: p2-g1 (0 matches)" {
		t.Fatalf("another font: got %q", message)
	}
}
