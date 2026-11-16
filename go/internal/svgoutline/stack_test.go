package svgoutline

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// stackSVG is a tall \left| built of extension pieces of one outline, one use per baseline y.
func stackSVG(ys ...float64) []byte {
	var uses strings.Builder
	for _, y := range ys {
		fmt.Fprintf(&uses, `<use data-text="⃓" xlink:href="#font_6_3" transform="matrix(10,0,0,-10,50,%g)"/>`, y)
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` +
		`<defs><path id="font_6_3" d="M0 -0.1L0.05 -0.1L0.05 0.65L0 0.65Z"/></defs>` + uses.String() + `</svg>`)
}

func stackPaint(index int64, y float64) rec.Map {
	return rec.Map{"font": "ABCDEF+CMEX10", "glyph": "vextendsingle", "text": "⃓", "operation": "fill_text",
		"matrix": []any{10.0, 0.0, 0.0, -10.0, 50.0, y}, "baseline": []any{50.0, y}, "advance": []any{0.0, 0.0},
		"index": index}
}

func stackGlyph(number int, y float64) rec.Map {
	return rec.Map{"id": fmt.Sprintf("p1-g%d", number), "line": int64(1), "text": "⃓", "font": "CMEX10",
		"baseline": []any{50.0, y}, "metric_quad": []any{50.0, y - 6.5, 50.0, y - 6.5, 50.0, y + 1, 50.0, y + 1}}
}

// MuPDF's structured text merges overlapping pieces of one delimiter (papers/0124: three pieces, two
// characters): the piece left without a character continues the bound column and adds no glyph.
func TestStackedDelimiterPieceContinuesItsColumn(t *testing.T) {
	svg := stackSVG(20, 26, 32)
	paints := []rec.Map{stackPaint(0, 20), stackPaint(1, 26), stackPaint(2, 32)}
	uses := boundUses(t, svg, paints, []rec.Map{stackGlyph(1, 20), stackGlyph(2, 26)})
	if len(uses) != 2 {
		t.Fatalf("bound uses %v", uses)
	}
}

// A copy apart from every bound copy of its outline is ink no text glyph accounts for.
func TestDetachedCopyIsStillUncovered(t *testing.T) {
	svg := stackSVG(20, 40)
	paints := []rec.Map{stackPaint(0, 20), stackPaint(1, 40)}
	if message := bindFailure(func() { Bind(svg, paints, []rec.Map{stackGlyph(1, 20)}) }); message != "SVG glyph placement coverage differs" {
		t.Fatalf("Bind: %q", message)
	}
}
