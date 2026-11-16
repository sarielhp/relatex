package svgoutline

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// repaintSVG is a page whose figure paints its label "C" twice, identically, each copy in its own
// clip group: the case of books/0003 page 375 (an Ipe figure) and books/0004 page 180, which MuPDF's
// structured text reads as one bold character. first and second are the two uses' extra attributes.
func repaintSVG(first, second string) []byte {
	use := `<g clip-path="url(#clip_%d)"><use data-text="C" xlink:href="#font_22_10" ` +
		`transform="matrix(10,0,0,-10,10,20)" fill="#0000ff"%s/></g>`
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` +
		`<defs><path id="font_22_10" d="M0 0C0 2 2 2 2 0Z"/>` +
		`<clipPath id="clip_1"><path d="M0 0H50V50H0Z"/></clipPath><clipPath id="clip_2"><path d="M0 0H50V50H0Z"/></clipPath></defs>` +
		fmt.Sprintf(use, 1, first) + fmt.Sprintf(use, 2, second) + `</svg>`)
}

func repaintPaint(index int64) rec.Map {
	return rec.Map{"font": "ALBEDP+NimbusRomNo9L-ReguItal", "glyph": "C", "text": "C", "operation": "fill_text",
		"matrix": []any{10.0, 0.0, 0.0, -10.0, 10.0, 20.0}, "baseline": []any{10.0, 20.0}, "advance": []any{6.67, 0.0},
		"index": index}
}

func repaintGlyph(number int) rec.Map {
	return rec.Map{"id": fmt.Sprintf("p1-g%d", number), "line": int64(1), "text": "C", "font": "NimbusRomNo9L-ReguItal",
		"baseline": []any{10.0, 20.0}, "metric_quad": []any{10.0, 13.0, 16.0, 13.0, 10.0, 20.0, 16.0, 20.0}}
}

func bindFailure(run func()) (message string) {
	defer func() {
		if failure, ok := recover().(rec.Failure); ok {
			message = failure.Message
		}
	}()
	run()
	return ""
}

func boundUses(t *testing.T, svg []byte, painted, glyphs []rec.Map) []int64 {
	t.Helper()
	var bound []rec.Map
	if message := bindFailure(func() { bound = Bind(svg, painted, glyphs) }); message != "" {
		t.Fatalf("Bind failed: %s", message)
	}
	values := make([]any, len(bound))
	uses := make([]int64, len(bound))
	for index, glyph := range bound {
		values[index] = glyph
		binding := rec.FetchMap(glyph, "outline_binding")
		uses[index] = rec.FetchInt(binding, "use")
		if paint := rec.FetchInt(rec.FetchMap(binding, "paint"), "index"); paint != int64(index) {
			t.Fatalf("glyph %d bound to paint %d", index, paint)
		}
	}
	// The bundle reader validates the bound glyphs against the page SVG alone.
	if message := bindFailure(func() { Validate(svg, values) }); message != "" {
		t.Fatalf("Validate failed: %s", message)
	}
	return uses
}

func TestIdenticalRepaintIsOnePaint(t *testing.T) {
	svg := repaintSVG("", "")
	uses := boundUses(t, svg, []rec.Map{repaintPaint(0), repaintPaint(1)}, []rec.Map{repaintGlyph(1)})
	if len(uses) != 1 || uses[0] != 0 {
		t.Fatalf("bound uses %v, want the first copy", uses)
	}
}

// A figure placed twice by two transforms lands its copies 0.0002 pt apart (papers/0182): one
// placement within the tolerance of every position (further apart, overprint covers the copy).
func TestRepaintWithinThePositionTolerance(t *testing.T) {
	placed := func(x string) []byte {
		return []byte(strings.Replace(string(repaintSVG("", "")), `matrix(10,0,0,-10,10,20)" fill="#0000ff"/></g></svg>`,
			`matrix(10,0,0,-10,`+x+`,20)" fill="#0000ff"/></g></svg>`, 1))
	}
	second := repaintPaint(1)
	second["matrix"] = []any{10.0, 0.0, 0.0, -10.0, 10.0002, 20.0}
	second["baseline"] = []any{10.0002, 20.0}
	uses := boundUses(t, placed("10.0002"), []rec.Map{repaintPaint(0), second}, []rec.Map{repaintGlyph(1)})
	if len(uses) != 1 || uses[0] != 0 {
		t.Fatalf("bound uses %v, want the first copy", uses)
	}
	// Two copies whose placements differ in their glyph name are two paints, however close.
	renamed := maps.Clone(second)
	renamed["glyph"] = "C.alt"
	message := bindFailure(func() { Bind(placed("10.0002"), []rec.Map{repaintPaint(0), renamed}, []rec.Map{repaintGlyph(1)}) })
	if message != "Missing or ambiguous native glyph paint: p1-g1 (2 matches)" {
		t.Fatalf("renamed copy: got %q", message)
	}
}

func TestTwoGlyphsTakeTheCopiesInPaintOrder(t *testing.T) {
	svg := repaintSVG("", "")
	uses := boundUses(t, svg, []rec.Map{repaintPaint(0), repaintPaint(1)}, []rec.Map{repaintGlyph(1), repaintGlyph(2)})
	if len(uses) != 2 || uses[0] != 0 || uses[1] != 1 {
		t.Fatalf("bound uses %v, want 0 and 1", uses)
	}
}

func TestRepaintNeedsIdenticalEvidence(t *testing.T) {
	differentGlyph := repaintPaint(1)
	differentGlyph["glyph"] = "C.alt"
	shifted := repaintPaint(1)
	shifted["advance"] = []any{6.0, 0.0}
	cases := []struct {
		name          string
		first, second string
		second2       rec.Map
		want          string
	}{
		{"another glyph", "", "", differentGlyph, "Missing or ambiguous native glyph paint: p1-g1 (2 matches)"},
		{"another advance", "", "", shifted, "Missing or ambiguous native glyph paint: p1-g1 (2 matches)"},
		{"another fill", "", ` stroke="#ff0000"`, repaintPaint(1), "Native glyph has no SVG outline placement: p1-g1 (2 matches)"},
		{"translucent", ` fill-opacity=".5"`, ` fill-opacity=".5"`, repaintPaint(1), "Native glyph has no SVG outline placement: p1-g1 (2 matches)"},
	}
	for _, item := range cases {
		svg := repaintSVG(item.first, item.second)
		message := bindFailure(func() { Bind(svg, []rec.Map{repaintPaint(0), item.second2}, []rec.Map{repaintGlyph(1)}) })
		if message != item.want {
			t.Errorf("%s: got %q, want %q", item.name, message, item.want)
		}
	}
}

// A copy the binding left out that differs from the bound use (here its fill) is uncovered ink.
func TestUncoveredDifferentCopyFailsValidation(t *testing.T) {
	plain := repaintSVG("", "")
	bound := Bind(plain, []rec.Map{repaintPaint(0), repaintPaint(1)}, []rec.Map{repaintGlyph(1)})
	recolored := []byte(strings.Replace(string(plain), `fill="#0000ff"/></g></svg>`, `fill="#ff0000"/></g></svg>`, 1))
	glyph := maps.Clone(bound[0])
	if message := bindFailure(func() { Validate(recolored, []any{glyph}) }); message != "SVG glyph placement coverage differs" {
		t.Fatalf("got %q", message)
	}
}
