package svgoutline

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A plot drawn again embeds each copy's marker in a subset font of its own (papers/0209): the
// copies are one glyph painted again, under two path ids of one geometry.
func TestRepaintInAnotherSubsetIsOnePaint(t *testing.T) {
	subsetSVG := func(second string) []byte {
		page := strings.Replace(string(repaintSVG("", "")), `<defs>`, `<defs><path id="font_23_10" d="`+second+`"/>`, 1)
		return []byte(strings.Replace(page, `clip_2)"><use data-text="C" xlink:href="#font_22_10"`,
			`clip_2)"><use data-text="C" xlink:href="#font_23_10"`, 1))
	}
	second := repaintPaint(1)
	second["font"] = "ALBEDQ+NimbusRomNo9L-ReguItal"
	painted := []rec.Map{repaintPaint(0), second}
	if uses := boundUses(t, subsetSVG("M0 0C0 2 2 2 2 0Z"), painted, []rec.Map{repaintGlyph(1)}); len(uses) != 1 || uses[0] != 0 {
		t.Fatalf("bound uses %v, want the first copy", uses)
	}
	// Another outline under the second id is no copy of the first.
	if message := bindFailure(func() { Bind(subsetSVG("M0 0H2V2H0Z"), painted, []rec.Map{repaintGlyph(1)}) }); message != "Native glyph has no SVG outline placement: p1-g1 (2 matches)" {
		t.Fatalf("other outline: got %q", message)
	}
	// Paints of another font are not one glyph: neither binds when the structured text keeps both.
	other := repaintPaint(1)
	other["font"] = "ALBEDQ+NimbusRomNo9L-Regu"
	twice := []rec.Map{repaintGlyph(1), repaintGlyph(2)}
	if message := bindFailure(func() { Bind(subsetSVG("M0 0C0 2 2 2 2 0Z"), []rec.Map{repaintPaint(0), other}, twice) }); message != "Missing or ambiguous native glyph paint: p1-g2 (0 matches)" {
		t.Fatalf("another font: got %q", message)
	}
}
