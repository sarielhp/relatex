package svgoutline

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// markerSVG is a page with a 100 by 100 viewBox whose uses of "G" (font_5_77) are given in paint order.
func markerSVG(target string, uses ...string) []byte {
	if target == "" {
		target = `<path id="font_5_77" d="M0 0H1V1H0Z"/>`
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" ` +
		`width="100" height="100" viewBox="0 0 100 100"><defs>` + target + `</defs>` + strings.Join(uses, "") + `</svg>`)
}

// markerUse is a use of the marker at (10, 20), scaled by 4, with extra attributes.
func markerUse(extra string) string {
	return `<use data-text="G" xlink:href="#font_5_77" transform="matrix(4,0,0,-4,10,20)"` + extra + `/>`
}

const strokeAttributes = ` stroke-width=".2" fill="none" stroke="#000000"`

func markerPaint(index int64, operation string, y float64) rec.Map {
	return rec.Map{"font": "ZapfDingbats", "glyph": "a71", "text": "G", "operation": operation,
		"matrix": []any{4.0, 0.0, 0.0, -4.0, 10.0, y}, "baseline": []any{10.0, y}, "advance": []any{3.0, 0.0}, "index": index}
}

func markerGlyph(y float64) rec.Map {
	return rec.Map{"id": "p1-g1", "line": int64(1), "text": "G", "font": "ZapfDingbats", "baseline": []any{10.0, y},
		"metric_quad": []any{10.0, y - 3, 13.0, y - 3, 10.0, y + 1, 13.0, y + 1}}
}

// bindOne binds one glyph and validates the result as the bundle reader does; it returns the binding.
func bindOne(t *testing.T, svg []byte, painted []rec.Map, glyph rec.Map) rec.Map {
	t.Helper()
	var bound []rec.Map
	if message := bindFailure(func() { bound = Bind(svg, painted, []rec.Map{glyph}) }); message != "" {
		t.Fatalf("Bind failed: %s", message)
	}
	if message := bindFailure(func() { Validate(svg, []any{bound[0]}) }); message != "" {
		t.Fatalf("Validate failed: %s", message)
	}
	return rec.FetchMap(bound[0], "outline_binding")
}

// A Type 3 glyph (slides/0016) is a group of paths in glyph space, each carrying the font matrix.
func TestTypeThreeGlyphGroupIsMeasured(t *testing.T) {
	group := `<g id="font_5_77"><path transform="matrix(.001,0,0,.001,0,0)" d="M0 0H1000V500H0Z"/>` +
		`<path transform="matrix(.001,0,0,.001,0,0)" fill-rule="evenodd" d="M0 0H500V1000H0Z"/></g>`
	binding := bindOne(t, markerSVG(group, markerUse("")), []rec.Map{markerPaint(0, "fill_text", 20)}, markerGlyph(20))
	if want := []any{10.0, 16.0, 14.0, 20.0}; !rec.Equal(binding["bounds"], want) {
		t.Fatalf("bounds %v, want %v", binding["bounds"], want)
	}
	empty := bindOne(t, markerSVG(`<g id="font_5_77"></g>`, markerUse("")), []rec.Map{markerPaint(0, "fill_text", 20)}, markerGlyph(20))
	if empty["status"] != "empty_outline" {
		t.Fatalf("empty Type 3 glyph: %v", empty)
	}
	for _, unsupported := range []string{`<g id="font_5_77"><path d="M0 0H1V1H0Z" stroke="#000"/></g>`,
		`<g id="font_5_77"><g><path d="M0 0H1V1H0Z"/></g></g>`, `<g id="font_5_77" transform="scale(2)"><path d="M0 0H1V1H0Z"/></g>`} {
		message := bindFailure(func() { For(markerSVG(unsupported, markerUse(""))) })
		if message != "Unsupported SVG glyph outline target" {
			t.Errorf("%s: got %q", unsupported, message)
		}
	}
}

// A glyph in text render mode 2 is a fill and then a stroke of one outline: it binds to the fill.
func TestFilledAndStrokedGlyphBindsTheFill(t *testing.T) {
	svg := markerSVG("", markerUse(""), markerUse(strokeAttributes))
	painted := []rec.Map{markerPaint(0, "fill_text", 20), markerPaint(1, "stroke_text", 20)}
	if binding := bindOne(t, svg, painted, markerGlyph(20)); binding["use"] != int64(0) {
		t.Fatalf("bound %v, want the fill", binding)
	}
	// A stroke wider than twice the overprint tolerance is ink the outline does not measure.
	wide := markerSVG("", markerUse(""), markerUse(` stroke-width=".3" fill="none" stroke="#000000"`))
	if message := bindFailure(func() { Bind(wide, painted, []rec.Map{markerGlyph(20)}) }); message != "Native glyph has no SVG outline placement: p1-g1 (2 matches)" {
		t.Fatalf("wide stroke: got %q", message)
	}
	// A stroke before its fill is no render mode 2 paint.
	reversed := []rec.Map{markerPaint(0, "stroke_text", 20), markerPaint(1, "fill_text", 20)}
	if message := bindFailure(func() { Bind(svg, reversed, []rec.Map{markerGlyph(20)}) }); message != "Missing or ambiguous native glyph paint: p1-g1 (2 matches)" {
		t.Fatalf("reversed: got %q", message)
	}
}

// MuPDF 1.28.2's page SVG draws a stroked glyph's line width untransformed by the page CTM
// (papers/0073, papers4/0976): the SVG's 1.2 pt here is the trace's line width 1.2 in a figure
// scaled by 0.1, 0.12 pt in the PDF. The stroke's ink is the trace's width through the CTM.
func TestStrokeInkIsTheTraceWidthThroughTheCTM(t *testing.T) {
	wide := markerSVG("", markerUse(""), markerUse(` stroke-width=".3" fill="none" stroke="#000000"`))
	stroked := func(line, device float64) []rec.Map {
		stroke := markerPaint(1, "stroke_text", 20)
		stroke["line_width"], stroke["device_line_width"] = line, device
		return []rec.Map{markerPaint(0, "fill_text", 20), stroke}
	}
	if binding := bindOne(t, wide, stroked(1.2, 0.12), markerGlyph(20)); binding["use"] != int64(0) {
		t.Fatalf("bound %v, want the fill", binding)
	}
	// The tolerance holds for the true width, and the trace's width must be the SVG's to stand for it.
	for _, widths := range [][2]float64{{1.2, 1.2}, {0.8, 0.12}} {
		message := bindFailure(func() { Bind(wide, stroked(widths[0], widths[1]), []rec.Map{markerGlyph(20)}) })
		if message != "Native glyph has no SVG outline placement: p1-g1 (2 matches)" {
			t.Errorf("widths %v: got %q", widths, message)
		}
	}
}

// Two plot series mark one point in two colors (slides/0011): the glyph binds to the copy on top.
func TestRecoloredCopyBindsTheTopmost(t *testing.T) {
	svg := markerSVG("", markerUse(""), markerUse(strokeAttributes), markerUse(` fill="#ff0000"`),
		markerUse(` stroke-width=".2" fill="none" stroke="#ff0000"`))
	painted := []rec.Map{markerPaint(0, "fill_text", 20), markerPaint(1, "stroke_text", 20), markerPaint(2, "fill_text", 20),
		markerPaint(3, "stroke_text", 20)}
	if binding := bindOne(t, svg, painted, markerGlyph(20)); binding["use"] != int64(2) {
		t.Fatalf("bound %v, want the red fill", binding)
	}
	// A translucent top copy hides nothing.
	translucent := markerSVG("", markerUse(""), markerUse(` fill="#ff0000" fill-opacity=".5"`))
	twice := []rec.Map{markerPaint(0, "fill_text", 20), markerPaint(1, "fill_text", 20)}
	if message := bindFailure(func() { Bind(translucent, twice, []rec.Map{markerGlyph(20)}) }); message != "Native glyph has no SVG outline placement: p1-g1 (2 matches)" {
		t.Fatalf("translucent: got %q", message)
	}
}

// A glyph below the page (an overfull frame) is painted but culled from the page SVG.
func TestGlyphOutsideThePageHasNoOutline(t *testing.T) {
	svg := markerSVG("", markerUse(""))
	painted := []rec.Map{markerPaint(0, "fill_text", 20), markerPaint(1, "fill_text", 110)}
	glyphs := []rec.Map{markerGlyph(20), markerGlyph(110)}
	glyphs[1]["id"] = "p1-g2"
	var bound []rec.Map
	if message := bindFailure(func() { bound = Bind(svg, painted, glyphs) }); message != "" {
		t.Fatalf("Bind failed: %s", message)
	}
	binding := rec.FetchMap(bound[1], "outline_binding")
	if binding["status"] != "outside_page" || binding["bounds"] != nil || !rec.Equal(bound[1]["ink_quad"], Quad(nil, []any{10.0, 110.0})) {
		t.Fatalf("outside glyph: %v", bound[1])
	}
	if message := bindFailure(func() { Validate(svg, []any{bound[0], bound[1]}) }); message != "" {
		t.Fatalf("Validate failed: %s", message)
	}
	// A glyph that reaches into the page needs its outline.
	glyphs[1]["metric_quad"] = []any{10.0, 99.0, 13.0, 99.0, 10.0, 111.0, 13.0, 111.0}
	if message := bindFailure(func() { Bind(svg, painted, glyphs) }); message != "Native glyph has no SVG outline placement: p1-g2 (0 matches)" {
		t.Fatalf("reaching glyph: got %q", message)
	}
	// An outside binding of a glyph inside the page is refused.
	forged := rec.FetchMap(bound[1], "outline_binding")
	inside := markerGlyph(20)
	inside["outline_binding"] = forged
	inside["ink_quad"] = Quad(nil, []any{10.0, 20.0})
	forged["paint"] = markerPaint(0, "fill_text", 20)
	if message := bindFailure(func() { Validate(svg, []any{inside}) }); message != "Invalid outside-page glyph binding" {
		t.Fatalf("forged: got %q", message)
	}
}
