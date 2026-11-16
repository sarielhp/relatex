package svgoutline

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A glyph in text render mode 3 (LaTeXiT's hidden source, papers/0209; OCR text, papers2/0466)
// is painted as an ignore_text but has no outline in the page SVG.
func TestInvisibleGlyphHasNoOutline(t *testing.T) {
	svg := markerSVG("", markerUse(""))
	painted := []rec.Map{markerPaint(0, "fill_text", 20), markerPaint(1, "ignore_text", 50)}
	glyphs := []rec.Map{markerGlyph(20), markerGlyph(50)}
	glyphs[1]["id"] = "p1-g2"
	var bound []rec.Map
	if message := bindFailure(func() { bound = Bind(svg, painted, glyphs) }); message != "" {
		t.Fatalf("Bind failed: %s", message)
	}
	binding := rec.FetchMap(bound[1], "outline_binding")
	if binding["status"] != "invisible_text" || binding["bounds"] != nil || !rec.Equal(bound[1]["ink_quad"], Quad(nil, []any{10.0, 50.0})) {
		t.Fatalf("invisible glyph: %v", bound[1])
	}
	if message := bindFailure(func() { Validate(svg, []any{bound[0], bound[1]}) }); message != "" {
		t.Fatalf("Validate failed: %s", message)
	}
	// An invisible paint the SVG draws an outline for is not invisible.
	drawn := markerSVG("", markerUse(""), `<use data-text="G" xlink:href="#font_5_77" transform="matrix(4,0,0,-4,10,50)"/>`)
	if message := bindFailure(func() { Bind(drawn, painted, glyphs) }); message != "Unsupported stroked or invisible glyph outline" {
		t.Fatalf("drawn invisible glyph: got %q", message)
	}
	// A glyph painted in another mode keeps needing its outline.
	stroked := []rec.Map{markerPaint(0, "fill_text", 20), markerPaint(1, "stroke_text", 50)}
	if message := bindFailure(func() { Bind(svg, stroked, glyphs) }); message != "Unsupported stroked or invisible glyph outline" {
		t.Fatalf("stroked glyph: got %q", message)
	}
	// An invisible binding of a filled glyph is refused.
	forged := markerGlyph(20)
	forged["outline_binding"] = rec.Map{"status": "invisible_text", "paint": markerPaint(0, "fill_text", 20), "bounds": nil,
		"origin_binding": "paint_origin"}
	forged["ink_quad"] = Quad(nil, []any{10.0, 20.0})
	unbound := markerSVG("")
	if message := bindFailure(func() { Validate(unbound, []any{forged}) }); message != "SVG outline and native glyph identities differ" {
		t.Fatalf("forged: got %q", message)
	}
}

// LaTeXiT's hidden source is set a millionth of a point wide (papers/0209): a letter repeated in a
// line matches several invisible paints at one point, which bind in paint order.
func TestRepeatedInvisibleGlyphsBindInPaintOrder(t *testing.T) {
	svg := markerSVG("")
	painted := []rec.Map{markerPaint(0, "ignore_text", 50), markerPaint(1, "ignore_text", 50)}
	glyphs := []rec.Map{markerGlyph(50), markerGlyph(50)}
	glyphs[1]["id"] = "p1-g2"
	var bound []rec.Map
	if message := bindFailure(func() { bound = Bind(svg, painted, glyphs) }); message != "" {
		t.Fatalf("Bind failed: %s", message)
	}
	for index, glyph := range bound {
		paint := rec.FetchMap(rec.FetchMap(glyph, "outline_binding"), "paint")
		if paint["index"] != int64(index) {
			t.Fatalf("glyph %d bound to paint %v", index, paint["index"])
		}
	}
	// Coincident paints that are not all invisible bind in paint order (inPaintOrder), and the
	// filled one still needs its outline.
	mixed := []rec.Map{markerPaint(0, "ignore_text", 50), markerPaint(1, "fill_text", 50)}
	if message := bindFailure(func() { Bind(svg, mixed, glyphs) }); message != "Native glyph has no SVG outline placement: p1-g2 (0 matches)" {
		t.Fatalf("mixed: got %q", message)
	}
}

// A colored Type 3 glyph (twemoji, papers4/1123) fills its paths in colors of its own.
func TestColoredTypeThreeGlyphIsMeasured(t *testing.T) {
	group := `<g id="font_5_77"><path transform="matrix(.001,0,0,.001,0,0)" d="M0 0H1000V500H0Z" fill="#ff9d33"/>` +
		`<path transform="matrix(.001,0,0,.001,0,0)" d="M0 0H500V1000H0Z" fill="#ffffff"/></g>`
	binding := bindOne(t, markerSVG(group, markerUse("")), []rec.Map{markerPaint(0, "fill_text", 20)}, markerGlyph(20))
	if want := []any{10.0, 16.0, 14.0, 20.0}; !rec.Equal(binding["bounds"], want) {
		t.Fatalf("bounds %v, want %v", binding["bounds"], want)
	}
	for _, fill := range []string{"none", "url(#pattern_1)", "#ff9d3380"} {
		unsupported := `<g id="font_5_77"><path d="M0 0H1V1H0Z" fill="` + fill + `"/></g>`
		if message := bindFailure(func() { For(markerSVG(unsupported, markerUse(""))) }); message != "Unsupported SVG glyph outline target" {
			t.Errorf("fill %s: got %q", fill, message)
		}
	}
}
