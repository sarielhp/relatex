package svgoutline

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// stencilPNG is a 4 by 6 gray bitmap whose set pixels are columns 1-2 of rows 1-4, as a data URI
// broken into lines the way MuPDF writes it.
func stencilPNG(t *testing.T, inked bool) string {
	t.Helper()
	bitmap := image.NewGray(image.Rect(0, 0, 4, 6))
	for y := 1; inked && y <= 4; y++ {
		for x := 1; x <= 2; x++ {
			bitmap.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, bitmap); err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(data.Bytes())
	return "data:image/png;base64,\n" + encoded[:10] + "\n" + encoded[10:]
}

// stencilGlyph is a bitmap Type 3 glyph as MuPDF writes it (papers/0101): a mask holding the
// bitmap and a rect it stencils, the rect's transform mapping pixels to glyph space.
func stencilGlyph(uri, rect, mask string) string {
	if rect == "" {
		rect = `x="0" y="0" width="4" height="6"`
	}
	return `<g id="font_5_77"><mask id="mask_6"` + mask + `><image id="image_7" width="4" height="6" xlink:href="` + uri +
		`"/></mask><rect ` + rect + ` transform="matrix(.25,0,0,-.25,0,1)" mask="url(#mask_6)"/></g>`
}

// A Type 3 glyph drawn through a clip (papers6/0798) is measured as the clip's cut of its paths.
func TestClippedGroupGlyphIsCutByItsClip(t *testing.T) {
	glyph := `<g id="font_5_77"><g><clipPath id="clip_5"><path d="M0 0H1V1H0Z"/></clipPath>` +
		`<g clip-path="url(#clip_5)"><path d="M0 0H2V2H0Z"/></g></g></g>`
	binding := bindOne(t, markerSVG(glyph, markerUse("")), []rec.Map{markerPaint(0, "fill_text", 20)}, markerGlyph(20))
	if want := []any{10.0, 16.0, 14.0, 20.0}; !rec.Equal(binding["bounds"], want) {
		t.Fatalf("bounds %v, want %v", binding["bounds"], want)
	}
}

// A bitmap glyph's bounds are its set pixels' box, not the stencilled rect's.
func TestStencilGlyphIsMeasuredByItsInk(t *testing.T) {
	svg := markerSVG(stencilGlyph(stencilPNG(t, true), "", ""), markerUse(""))
	binding := bindOne(t, svg, []rec.Map{markerPaint(0, "fill_text", 20)}, markerGlyph(20))
	if want := []any{11.0, 17.0, 13.0, 21.0}; !rec.Equal(binding["bounds"], want) {
		t.Fatalf("bounds %v, want %v", binding["bounds"], want)
	}
	blank := markerSVG(stencilGlyph(stencilPNG(t, false), "", ""), markerUse(""))
	if empty := bindOne(t, blank, []rec.Map{markerPaint(0, "fill_text", 20)}, markerGlyph(20)); empty["status"] != "empty_outline" {
		t.Fatalf("blank bitmap: %v", empty)
	}
}

// Any other mask, image or rect is no glyph outline.
func TestUnsupportedStencilsFail(t *testing.T) {
	uri := stencilPNG(t, true)
	for _, target := range []string{
		stencilGlyph(uri, `x="1" y="0" width="4" height="6"`, ""),
		stencilGlyph(uri, `x="0" y="0" width="4" height="5"`, ""),
		stencilGlyph(uri, `x="0" y="0" width="4" height="6" fill="red"`, ""),
		stencilGlyph(uri, "", ` maskUnits="userSpaceOnUse"`),
		strings.Replace(stencilGlyph(uri, "", ""), `mask="url(#mask_6)"`, `mask="url(#mask_9)"`, 1),
		strings.Replace(stencilGlyph(uri, "", ""), `</mask>`, `<rect width="1" height="1"/></mask>`, 1),
		stencilGlyph("data:image/jpeg;base64,AAAA", "", ""),
	} {
		if message := bindFailure(func() { For(markerSVG(target, markerUse(""))) }); message != "Unsupported SVG glyph outline target" {
			t.Errorf("%s: got %q", target, message)
		}
	}
	broken := stencilGlyph("data:image/png;base64,AAAA", "", "")
	if message := bindFailure(func() { For(markerSVG(broken, markerUse(""))) }); !strings.HasPrefix(message, "Invalid SVG glyph stencil") {
		t.Errorf("broken PNG: got %q", message)
	}
}

// stackedTargets are two fonts' glyphs of one text: font_5 (the marker) and font_6.
const stackedTargets = `<path id="font_5_77" d="M0 0H1V1H0Z"/><path id="font_6_77" d="M0 0H1V2H0Z"/>`

func stackedUse(font, text string) string {
	return `<use data-text="` + text + `" xlink:href="#font_` + font + `_77" transform="matrix(4,0,0,-4,10,20)"/>`
}

func otherFont(entry rec.Map) rec.Map {
	copied := rec.Map{}
	for key, value := range entry {
		copied[key] = value
	}
	copied["font"] = "XYBTIP"
	return copied
}

// Glyphs of two fonts stacked at one placement (xy-pic arrow tips, papers/0102) bind in paint order,
// and the one structured text drops is the companion of the one it keeps.
func TestStackedFontsBindInPaintOrder(t *testing.T) {
	svg := markerSVG(stackedTargets, stackedUse("5", "G"), stackedUse("6", "G"))
	first, second := markerPaint(0, "fill_text", 20), otherFont(markerPaint(1, "fill_text", 20))
	if binding := bindOne(t, svg, []rec.Map{first, second}, markerGlyph(20)); binding["use"] != int64(0) {
		t.Fatalf("first paint bound %v, want use 0", binding)
	}
	swapped := []rec.Map{otherFont(markerPaint(0, "fill_text", 20)), markerPaint(1, "fill_text", 20)}
	if binding := bindOne(t, svg, swapped, markerGlyph(20)); binding["use"] != int64(1) {
		t.Fatalf("second paint bound %v, want use 1", binding)
	}
	other := otherFont(markerGlyph(20))
	other["id"] = "p1-g2"
	if uses := boundUses(t, svg, []rec.Map{first, second}, []rec.Map{markerGlyph(20), other}); !slices.Equal(uses, []int64{0, 1}) {
		t.Fatalf("both glyphs bound %v", uses)
	}
	// A use of another text at the placement is ink no glyph binds.
	mixed := markerSVG(stackedTargets, stackedUse("5", "G"), stackedUse("6", "H"))
	if message := bindFailure(func() { Bind(mixed, []rec.Map{first}, []rec.Map{markerGlyph(20)}) }); message != "SVG glyph placement coverage differs" {
		t.Fatalf("other text: got %q", message)
	}
	// Two paints of one font are no stacked fonts: paint order alone does not tell them apart.
	same := []rec.Map{markerPaint(0, "fill_text", 20), markerPaint(1, "fill_text", 20)}
	if message := bindFailure(func() { Bind(svg, same, []rec.Map{markerGlyph(20)}) }); !strings.HasPrefix(message, "Missing or ambiguous") &&
		!strings.HasPrefix(message, "Native glyph has no SVG outline placement") {
		t.Fatalf("one font twice: got %q", message)
	}
}
