package svgoutline

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// mergeUse is one SVG use of an outline at scale and origin x, y.
type mergeUse struct {
	path, text, fill string
	scale, x, y      float64
}

func mergeSVG(uses ...mergeUse) []byte {
	var body strings.Builder
	for _, use := range uses {
		fmt.Fprintf(&body, `<use data-text=%q xlink:href="#%s" transform="matrix(%g,0,0,%g,%g,%g)"%s/>`,
			use.text, use.path, use.scale, -use.scale, use.x, use.y, use.fill)
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><defs>` +
		`<path id="font_4_29" d="M0 0L0.5 0L0.5 0.7L0 0.7Z"/><path id="font_7_24" d="M0 0L0.55 0L0.55 0.1L0 0.1Z"/>` +
		`<path id="font_2_48" d="M0 0L0.45 0L0.45 0.43L0 0.43Z"/></defs>` + body.String() + `</svg>`)
}

func mergePaint(index int64, font, glyph, text string, scale, x, y float64) rec.Map {
	return rec.Map{"font": font, "glyph": glyph, "text": text, "operation": "fill_text",
		"matrix": []any{scale, 0.0, 0.0, -scale, x, y}, "baseline": []any{x, y}, "advance": []any{scale / 2, 0.0},
		"index": index}
}

func mergeGlyph(number int, font, text string, x, y float64) rec.Map {
	return rec.Map{"id": fmt.Sprintf("p1-g%d", number), "line": int64(1), "text": text, "font": font,
		"baseline": []any{x, y}, "metric_quad": []any{x, y - 9, x + 6, y - 9, x, y + 1, x + 6, y + 1}}
}

// A grey enlarged "2" painted over a black one a third of a point off (papers2/0352), or a figure
// label drawn again a point away under another clip (papers2/0254): MuPDF's structured text keeps
// the first as one bold character (fake bold: same font and character, within a tenth of the
// copy's size on each axis). The copies are merged, not uncovered.
func TestFakeBoldCopiesAreMerged(t *testing.T) {
	svg := mergeSVG(mergeUse{"font_4_29", "2", "", 12.1941, 198.87416, 246.84793},
		mergeUse{"font_4_29", "2", ` fill="#ababab"`, 13.549, 199.22632, 247.1663},
		mergeUse{"font_4_29", "2", ` fill="#ababab"`, 13.549, 199.22632, 247.1663})
	paints := []rec.Map{mergePaint(0, "ABCDEF+LMRoman10-Regular", "50", "2", 12.1941, 198.87416, 246.84793),
		mergePaint(1, "ABCDEF+LMRoman10-Regular", "50", "2", 13.549, 199.22632, 247.1663),
		mergePaint(2, "ABCDEF+LMRoman10-Regular", "50", "2", 13.549, 199.22632, 247.1663)}
	glyphs := []rec.Map{mergeGlyph(1, "LMRoman10-Regular", "2", 198.87416, 246.84793)}
	if uses := boundUses(t, svg, paints, glyphs); len(uses) != 1 || uses[0] != 0 {
		t.Fatalf("bound uses %v", uses)
	}
}

// A copy a tenth of its size or more away is a character of its own MuPDF would have kept.
func TestDetachedFakeBoldCopyIsUncovered(t *testing.T) {
	svg := mergeSVG(mergeUse{"font_4_29", "2", "", 12, 100, 200}, mergeUse{"font_4_29", "2", "", 12, 101.3, 200})
	paints := []rec.Map{mergePaint(0, "ABCDEF+LMRoman10-Regular", "50", "2", 12, 100, 200),
		mergePaint(1, "ABCDEF+LMRoman10-Regular", "50", "2", 12, 101.3, 200)}
	glyphs := []rec.Map{mergeGlyph(1, "LMRoman10-Regular", "2", 100, 200)}
	if message := bindFailure(func() { Bind(svg, paints, glyphs) }); message != "SVG glyph placement coverage differs" {
		t.Fatalf("Bind: %q", message)
	}
}

// A TeX-mathx "r" followed by a CMMI10 "r" a fifth of a point away (papers2/0387): MuPDF drops the
// second as a repeat of the character it just added, whatever its font.
func TestRepeatedCharacterIsMerged(t *testing.T) {
	svg := mergeSVG(mergeUse{"font_7_24", "r", "", 10.90909, 136.553, 131.23401},
		mergeUse{"font_2_48", "r", "", 10.90909, 136.364, 131.23499})
	paints := []rec.Map{mergePaint(0, "ESUREE+TeX-mathx10", "r", "r", 10.90909, 136.553, 131.23401),
		mergePaint(1, "MWDTMO+CMMI10", "r", "r", 10.90909, 136.364, 131.23499)}
	glyphs := []rec.Map{mergeGlyph(1, "TeX-mathx10", "r", 136.553, 131.23401)}
	if uses := boundUses(t, svg, paints, glyphs); len(uses) != 1 || uses[0] != 0 {
		t.Fatalf("bound uses %v", uses)
	}
}

// The repeat is of the last character added only: another character between keeps the copy apart.
func TestRepeatAfterAnotherCharacterIsUncovered(t *testing.T) {
	svg := mergeSVG(mergeUse{"font_7_24", "r", "", 10.90909, 136.553, 131.23401},
		mergeUse{"font_4_29", "2", "", 10.90909, 150, 131.23401},
		mergeUse{"font_2_48", "r", "", 10.90909, 136.364, 131.23499})
	paints := []rec.Map{mergePaint(0, "ESUREE+TeX-mathx10", "r", "r", 10.90909, 136.553, 131.23401),
		mergePaint(1, "ABCDEF+LMRoman10-Regular", "50", "2", 10.90909, 150, 131.23401),
		mergePaint(2, "MWDTMO+CMMI10", "r", "r", 10.90909, 136.364, 131.23499)}
	glyphs := []rec.Map{mergeGlyph(1, "TeX-mathx10", "r", 136.553, 131.23401),
		mergeGlyph(2, "LMRoman10-Regular", "2", 150, 131.23401)}
	if message := bindFailure(func() { Bind(svg, paints, glyphs) }); message != "SVG glyph placement coverage differs" {
		t.Fatalf("Bind: %q", message)
	}
}
