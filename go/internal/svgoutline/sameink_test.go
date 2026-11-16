package svgoutline

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// sameInkSVG is a page whose cell fill draws a Type 3 box glyph twice at one origin, once from each of two
// fonts of one figure (papers6/0899): the glyph groups differ in id alone.
func sameInkSVG(second string) []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><defs>` +
		`<g id="font_56_0"><path transform="matrix(.01,0,0,.01,0,0)" d="M0 0H100V100H0Z"/></g>` +
		`<g id="font_60_0"><path transform="matrix(.01,0,0,.01,0,0)" d="` + second + `"/></g></defs>` +
		`<use data-text="C" xlink:href="#font_56_0" transform="matrix(10,0,0,-10,10,20)" fill="#ffffff"/>` +
		`<use data-text="C" xlink:href="#font_60_0" transform="matrix(10,0,0,-10,10,20)" fill="#ffffff"/></svg>`)
}

func TestGlyphGroupsOfOneGeometryAreOneInk(t *testing.T) {
	uses := boundUses(t, sameInkSVG("M0 0H100V100H0Z"), []rec.Map{repaintPaint(0), repaintPaint(1)}, []rec.Map{repaintGlyph(1)})
	if len(uses) != 1 || uses[0] != 0 {
		t.Fatalf("bound uses %v, want the first copy", uses)
	}
}

func TestGlyphGroupsOfOtherGeometryStayAmbiguous(t *testing.T) {
	painted, glyphs := []rec.Map{repaintPaint(0), repaintPaint(1)}, []rec.Map{repaintGlyph(1)}
	message := bindFailure(func() { Bind(sameInkSVG("M0 0H100V50H0Z"), painted, glyphs) })
	if message == "" {
		t.Fatal("two different outlines at one origin must not bind as one")
	}
}

// A paint's own color tells two stacked uses apart (a white box under which a black pattern of the same
// Type 3 font is drawn): the paint binds the use filled in its color.
func TestPaintColorPicksTheUseFilledInIt(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><defs>` +
		`<path id="font_1_0" d="M0 0H1V1H0Z"/><path id="font_1_1" d="M0 0H1V2H0Z"/></defs>` +
		`<use data-text="C" xlink:href="#font_1_0" transform="matrix(10,0,0,-10,10,20)" fill="#ffffff"/>` +
		`<use data-text="C" xlink:href="#font_1_1" transform="matrix(10,0,0,-10,10,20)"/></svg>`)
	paint := repaintPaint(0)
	paint["fill"] = "#000000"
	binding := rec.FetchMap(Bind(svg, []rec.Map{paint}, []rec.Map{repaintGlyph(1)})[0], "outline_binding")
	if use := rec.FetchInt(binding, "use"); use != 1 {
		t.Fatalf("bound use %d, want the black one", use)
	}
}
