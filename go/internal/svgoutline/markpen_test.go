package svgoutline

import (
	"fmt"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// arrowSVG is an extensible arrow accent (U+20D7) assembled from two pieces, its left piece
// starting exactly at the pen after the comma before it: books/0003 page 500.
const arrowSVG = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` +
	`<defs><path id="font_5_6094" d="M0 0C0 2 2 2 2 0Z"/><path id="font_5_6095" d="M0 0L1 0L1 1Z"/></defs>` +
	`<use data-text="&#x20d7;" xlink:href="#font_5_6094" transform="matrix(10,0,0,-10,10,20)"/>` +
	`<use data-text="&#x20d7;" xlink:href="#font_5_6095" transform="matrix(10,0,0,-10,12,20)"/></svg>`

func arrowPiece(index int64, glyph string, x float64) rec.Map {
	return rec.Map{"font": "ABCDEF+Math", "glyph": glyph, "text": "⃗", "operation": "fill_text",
		"matrix": []any{10.0, 0.0, 0.0, -10.0, x, 20.0}, "baseline": []any{x, 20.0}, "advance": []any{2.0, 0.0}, "index": index}
}

// arrowMark is a piece as MuPDF's structured text places it: every piece at the pen before the first.
func arrowMark(number int) rec.Map {
	return rec.Map{"id": fmt.Sprintf("p1-g%d", number), "line": int64(1), "text": "⃗", "font": "Math",
		"baseline": []any{10.0, 20.0}, "metric_quad": []any{10.0, 15.0, 10.0, 15.0, 10.0, 20.0, 10.0, 20.0}}
}

func TestArrowPiecesAfterAPieceAtItsOwnOrigin(t *testing.T) {
	svg := []byte(arrowSVG)
	painted := []rec.Map{arrowPiece(0, "6094", 10), arrowPiece(1, "6095", 12)}
	var bound []rec.Map
	if message := bindFailure(func() { bound = Bind(svg, painted, []rec.Map{arrowMark(1), arrowMark(2)}) }); message != "" {
		t.Fatalf("Bind failed: %s", message)
	}
	origins := []any{}
	values := []any{}
	for _, glyph := range bound {
		binding := rec.FetchMap(glyph, "outline_binding")
		origins = append(origins, binding["origin_binding"], rec.FetchInt(binding, "use"))
		values = append(values, glyph)
	}
	if fmt.Sprint(origins) != "[paint_origin 0 previous_paint_pen 1]" {
		t.Fatalf("bindings %v", origins)
	}
	if message := bindFailure(func() { Validate(svg, values) }); message != "" {
		t.Fatalf("Validate failed: %s", message)
	}
}

// A mark after a mark elsewhere than the shared pen still needs a paint of its own there.
func TestArrowPieceAwayFromThePenIsUnbound(t *testing.T) {
	svg := []byte(arrowSVG)
	painted := []rec.Map{arrowPiece(0, "6094", 10), arrowPiece(1, "6095", 12)}
	moved := arrowMark(2)
	moved["baseline"] = []any{11.0, 20.0}
	moved["metric_quad"] = []any{11.0, 15.0, 11.0, 15.0, 11.0, 20.0, 11.0, 20.0}
	message := bindFailure(func() { Bind(svg, painted, []rec.Map{arrowMark(1), moved}) })
	if message != "Missing or ambiguous native glyph paint: p1-g2 (0 matches)" {
		t.Fatalf("got %q", message)
	}
}
