package figdesc

import (
	"strings"
	"testing"
)

// MeasureSVG counts glyph uses and text characters as glyphs, shapes and other uses as marks, and
// nothing inside definitions (decision 110).
func TestMeasureSVGCountsGlyphsAndMarksOutsideDefinitions(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><defs><path id="glyph-0-1" d="M0 0"/><symbol id="s"><path d="M1 1"/></symbol></defs>` +
		`<g><use xlink:href="#glyph-0-1" x="1"/><use href="#glyph-0-1"/><use xlink:href="#s"/>` +
		`<path d="M0 0L1 1"/><rect width="1" height="1"/><circle r="1"/><text x="0">ab<tspan>c</tspan></text></g></svg>`
	got := MeasureSVG(svg)
	if got.Glyphs != 5 || got.Marks != 4 || !got.Measured {
		t.Fatalf("measured %+v", got)
	}
	if got.Complex() {
		t.Fatal("a small drawing is complex")
	}
	plot := "<svg>" + strings.Repeat(`<use xlink:href="#glyph-1-2"/>`, 45) + strings.Repeat(`<path d="M0 0"/>`, 120) + "</svg>"
	if measured := MeasureSVG(plot); !measured.Complex() || measured.String() != "45 text glyphs, 120 drawn marks" {
		t.Fatalf("plot %+v", measured)
	}
	if (Complexity{Glyphs: 100, Marks: 1000}).Complex() {
		t.Fatal("an unmeasured figure is complex")
	}
}
