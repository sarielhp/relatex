package svgoutline

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A matplotlib label in ArialMT paints its space glyph, which the paint trace and the page SVG
// call " " and MuPDF's structured text U+00A0 (papers3/0565): the no-break space binds to it.
func TestNoBreakSpaceBindsToItsSpacePaint(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` +
		`<defs><path id="font_5_3" d=""/></defs>` +
		`<use data-text=" " xlink:href="#font_5_3" transform="matrix(5,0,0,-5,30,40)"/></svg>`)
	paint := rec.Map{"font": "AAAAAA+ArialMT", "glyph": "3", "text": " ", "operation": "fill_text",
		"matrix": []any{5.0, 0.0, 0.0, -5.0, 30.0, 40.0}, "baseline": []any{30.0, 40.0}, "advance": []any{1.39, 0.0},
		"index": int64(0)}
	glyph := rec.Map{"id": "p1-g1", "line": int64(1), "text": " ", "font": "ArialMT",
		"baseline": []any{30.0, 40.0}, "metric_quad": []any{30.0, 36.0, 31.39, 36.0, 30.0, 41.0, 31.39, 41.0}}
	uses := boundUses(t, svg, []rec.Map{paint}, []rec.Map{glyph})
	if len(uses) != 1 || uses[0] != 0 {
		t.Fatalf("bound uses %v", uses)
	}
}

// A Type 3 font that names its blank glyph U+00A0 in the paint trace, the page SVG and the
// structured text alike (papers5/1506) still binds.
func TestNoBreakSpaceEverywhereStillBinds(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` +
		`<defs><path id="font_12_201" d=""/></defs>` +
		"<use data-text=\" \" xlink:href=\"#font_12_201\" transform=\"matrix(4,0,0,-4,30,40)\"/></svg>")
	paint := rec.Map{"font": "Type3 (280 0 R)", "glyph": "201", "text": " ", "operation": "fill_text",
		"matrix": []any{4.0, 0.0, 0.0, -4.0, 30.0, 40.0}, "baseline": []any{30.0, 40.0}, "advance": []any{1.0, 0.0},
		"index": int64(0)}
	glyph := rec.Map{"id": "p1-g1", "line": int64(1), "text": " ", "font": "Type3 (280 0 R)",
		"baseline": []any{30.0, 40.0}, "metric_quad": []any{30.0, 37.0, 31.0, 37.0, 30.0, 41.0, 31.0, 41.0}}
	if uses := boundUses(t, svg, []rec.Map{paint}, []rec.Map{glyph}); len(uses) != 1 {
		t.Fatalf("bound uses %v", uses)
	}
}

// A matplotlib label line ends in its space glyph under an ActualText of a newline (papers/0057,
// papers/0098): the paint trace and the page SVG record " ", MuPDF's structured text "\n". The
// newline binds to that space paint and its use.
func TestActualTextNewlineBindsToItsSpacePaint(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` +
		`<defs><path id="font_10_3" d=""/></defs>` +
		`<use data-text=" " xlink:href="#font_10_3" transform="matrix(5,0,0,-5,30,40)"/></svg>`)
	paint := rec.Map{"font": "AAAAAA+ArialMT", "glyph": "3", "text": " ", "operation": "fill_text",
		"matrix": []any{5.0, 0.0, 0.0, -5.0, 30.0, 40.0}, "baseline": []any{30.0, 40.0}, "advance": []any{1.39, 0.0},
		"index": int64(0)}
	glyph := rec.Map{"id": "p1-g1", "line": int64(1), "text": "\n", "font": "ArialMT",
		"baseline": []any{30.0, 40.0}, "metric_quad": []any{30.0, 36.0, 31.39, 36.0, 30.0, 41.0, 31.39, 41.0}}
	if uses := boundUses(t, svg, []rec.Map{paint}, []rec.Map{glyph}); len(uses) != 1 || uses[0] != 0 {
		t.Fatalf("bound uses %v", uses)
	}
}
