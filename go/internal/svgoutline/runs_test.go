package svgoutline

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// runSVG is a page whose uses are given as (path number, text, x, extra attributes), each at
// scale 4 on the baseline y = 20.
func runSVG(uses ...string) []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" ` +
		`width="100" height="100" viewBox="0 0 100 100"><defs><path id="font_5_1" d="M0 0H1V1H0Z"/>` +
		`<path id="font_5_2" d="M0 0H.5V1H0Z"/><path id="font_5_21" d="M0 0H1V.5H0Z"/></defs>` + strings.Join(uses, "") + `</svg>`)
}

func runUse(path int, text string, x float64, extra string) string {
	return fmt.Sprintf(`<use data-text="%s" xlink:href="#font_5_%d" transform="matrix(4,0,0,-4,%g,20)"%s/>`, text, path, x, extra)
}

func runPaint(index int64, operation, glyph, text string, x float64) rec.Map {
	return rec.Map{"font": "ABCDEF+Pieces", "glyph": glyph, "text": text, "operation": operation,
		"matrix": []any{4.0, 0.0, 0.0, -4.0, x, 20.0}, "baseline": []any{x, 20.0}, "advance": []any{4.0, 0.0}, "index": index}
}

func runGlyph(number int, text string, x float64) rec.Map {
	return rec.Map{"id": fmt.Sprintf("p1-g%d", number), "line": int64(1), "text": text, "font": "Pieces",
		"baseline": []any{x, 20.0}, "metric_quad": []any{x, 16.0, x + 4, 16.0, x, 20.0, x + 4, 20.0}}
}

// A run of glyphs shown in text render mode 2 is filled and then stroked as a whole (papers/0073,
// papers5/1289, papers5/1416): the paint trace records the run's fill_text and then its stroke_text,
// the page SVG its filled uses and then its stroked ones. Each glyph binds to its fill; its stroke is
// the run's length later.
func TestStrokedRunBindsEachFill(t *testing.T) {
	svg := runSVG(runUse(1, "A", 10, ""), runUse(2, "B", 20, ""), runUse(1, "A", 10, strokeAttributes), runUse(2, "B", 20, strokeAttributes))
	painted := []rec.Map{runPaint(0, "fill_text", "1", "A", 10), runPaint(1, "fill_text", "2", "B", 20),
		runPaint(2, "stroke_text", "1", "A", 10), runPaint(3, "stroke_text", "2", "B", 20)}
	glyphs := []rec.Map{runGlyph(1, "A", 10), runGlyph(2, "B", 20)}
	if uses := boundUses(t, svg, painted, glyphs); uses[0] != 0 || uses[1] != 1 {
		t.Fatalf("bound uses %v, want the fills", uses)
	}
	// A stroke that is no copy of its run (the run's order differs) is no render mode 2 companion.
	swapped := []rec.Map{painted[0], painted[1], runPaint(2, "stroke_text", "2", "B", 20), runPaint(3, "stroke_text", "1", "A", 10)}
	if message := bindFailure(func() { Bind(svg, swapped, glyphs) }); message != "Missing or ambiguous native glyph paint: p1-g1 (2 matches)" {
		t.Fatalf("swapped paint run: got %q", message)
	}
	swappedSVG := runSVG(runUse(1, "A", 10, ""), runUse(2, "B", 20, ""), runUse(2, "B", 20, strokeAttributes), runUse(1, "A", 10, strokeAttributes))
	if message := bindFailure(func() { Bind(swappedSVG, painted, glyphs) }); message != "Native glyph has no SVG outline placement: p1-g1 (2 matches)" {
		t.Fatalf("swapped SVG run: got %q", message)
	}
}

// A glyph whose ToUnicode gives "𝑝𝑝" has a second, zero-width character at its pen; MuPDF drops the
// next glyph of that font and character whose origin lies within a tenth of its size of it, as a
// fake-bold copy (CambriaMath in papers5/1289). Its use is admitted as merged, only that near.
func TestCopyOfAContinuationCharacterIsMerged(t *testing.T) {
	glyph := func(continued float64) rec.Map {
		bound := runGlyph(1, "𝑝𝑝", 10)
		bound["unicode_components"] = []any{rec.Map{"text": "𝑝", "baseline": []any{10.0, 20.0}},
			rec.Map{"text": "𝑝", "baseline": []any{continued, 20.0}}}
		return bound
	}
	svg := runSVG(runUse(1, "𝑝", 10, ""), runUse(2, "𝑝", 13.01, ""))
	painted := []rec.Map{runPaint(0, "fill_text", "1", "𝑝𝑝", 10), runPaint(1, "fill_text", "2", "𝑝𝑝", 13.01)}
	bindWith := func(continued float64) string {
		bound := glyph(continued)
		bound["unicode_binding"] = painted[0]
		return bindFailure(func() { Validate(svg, []any{Bind(svg, painted, []rec.Map{bound})[0]}) })
	}
	if message := bindWith(13); message != "" {
		t.Fatalf("merged copy: %q", message)
	}
	if message := bindWith(12); message != "SVG glyph placement coverage differs" {
		t.Fatalf("distant copy: got %q", message)
	}
}

// A paint whose text is an ActualText replacement ("fi" for a ligature glyph the trace reads as
// U+FFFD, papers5/1289) binds to the SVG use the page spells with its drawn text.
func TestReplacedTextBindsByItsDrawnText(t *testing.T) {
	svg := runSVG(runUse(1, "�", 10, ""))
	paint := runPaint(0, "fill_text", "240", "fi", 10)
	paint["drawn_text"] = "�"
	if uses := boundUses(t, svg, []rec.Map{paint}, []rec.Map{runGlyph(1, "fi", 10)}); uses[0] != 0 {
		t.Fatalf("bound uses %v", uses)
	}
	// Without its drawn text the paint names another character than the page's use.
	delete(paint, "drawn_text")
	if message := bindFailure(func() { Bind(svg, []rec.Map{paint}, []rec.Map{runGlyph(1, "fi", 10)}) }); message != "Native glyph has no SVG outline placement: p1-g1 (0 matches)" {
		t.Fatalf("no drawn text: got %q", message)
	}
}

// Two glyphs of one font that both read as U+FFFD at one origin (a LaTeX line font piece and a
// control character, papers5/1457) take the paints in paint order; their outlines are told apart by
// the glyph mapping an earlier glyph made.
func TestGlyphsAtOneOriginBindInPaintOrder(t *testing.T) {
	svg := runSVG(runUse(1, "�", 0, ""), runUse(1, "�", 10, ""), runUse(21, "�", 10, ""))
	painted := []rec.Map{runPaint(0, "fill_text", "a1", "�", 0), runPaint(1, "fill_text", "a1", "�", 10),
		runPaint(2, "fill_text", "a21", "�", 10)}
	glyphs := []rec.Map{runGlyph(1, "�", 0), runGlyph(2, "�", 10), runGlyph(3, "�", 10)}
	if uses := boundUses(t, svg, painted, glyphs); uses[1] != 1 || uses[2] != 2 {
		t.Fatalf("bound uses %v", uses)
	}
	// Without the earlier glyph nothing tells the two outlines apart.
	unmapped := runSVG(runUse(1, "�", 10, ""), runUse(21, "�", 10, ""))
	message := bindFailure(func() { Bind(unmapped, painted[1:], glyphs[1:]) })
	if message != "Native glyph has no SVG outline placement: p1-g2 (2 matches)" {
		t.Fatalf("unmapped outlines: got %q", message)
	}
	// One structured glyph for two paints of different outlines is MuPDF keeping one of stacked pieces:
	// it takes the first paint and the other outline is the stacked companion.
	if uses := boundUses(t, svg, painted, glyphs[:2]); uses[1] != 1 {
		t.Fatalf("stacked pieces: bound uses %v", uses)
	}
	// A readable character painted by two glyph names under one structured glyph stays ambiguous.
	letters := []rec.Map{runPaint(0, "fill_text", "a1", "A", 10), runPaint(1, "fill_text", "a21", "A", 10)}
	lettersSVG := runSVG(runUse(1, "A", 10, ""), runUse(21, "A", 10, ""))
	if message := bindFailure(func() { Bind(lettersSVG, letters, []rec.Map{runGlyph(1, "A", 10)}) }); message == "" {
		t.Fatalf("readable stacked glyphs accepted")
	}
}

// A paint whose text is a tab with the characters MuPDF adds after it (CR, space, no-break space)
// binds to the use of its first character, which the page SVG spells as a blank (papers2/0349).
func TestMultiCharacterBlankPaintBindsByItsFirstCharacter(t *testing.T) {
	svg := runSVG(runUse(1, "\t", 10, ""))
	paint := runPaint(0, "fill_text", "1", "\t\r  ", 10)
	if uses := boundUses(t, svg, []rec.Map{paint}, []rec.Map{runGlyph(1, "\t\r \u00a0", 10)}); uses[0] != 0 {
		t.Fatalf("bound uses %v", uses)
	}
}
