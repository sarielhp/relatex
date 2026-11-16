package layout

import (
	"testing"

	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// A ligature glyph without Unicode under ActualText "fi" (papers5/1289): the structured text writes
// "f" with the glyph's advance and "i" at its end, so the paint's text is the replacement and the
// glyph's own Unicode, which the page SVG keeps, is its drawn text. The two records bind as one glyph.
func TestActualTextOfOneGlyphIsItsText(t *testing.T) {
	trace := nativetrace.Parse([]byte(`<document><page>` +
		`<metatext type="actualtext" txt="fi"><fill_text transform="1 0 0 1 0 0">` +
		`<span font="Type3 (209 0 R)" wmode="0" trm="10 0 0 10"><g unicode="&#xFFFD;" glyph="240" x="20" y="30" adv=".5"/></span>` +
		`</fill_text></metatext>` +
		`<metatext type="actualtext" txt="ab"><fill_text transform="1 0 0 1 0 0">` +
		`<span font="F" wmode="0" trm="10 0 0 10"><g unicode="x" glyph="1" x="40" y="30" adv=".5"/><g unicode="y" glyph="2" x="45" y="30" adv=".5"/></span>` +
		`</fill_text></metatext></page></document>`)).Descendants("page")[0]
	paint := newUnicodeGlyphs(trace)
	if first := paint.painted[0]; first["text"] != "fi" || first["drawn_text"] != "�" {
		t.Fatalf("replaced paint %v", first)
	}
	// Replacement text over several glyphs is not split among them: they keep their own.
	if second := paint.painted[1]; second["text"] != "x" || second["drawn_text"] != nil {
		t.Fatalf("several glyphs %v", second)
	}
	records := []rec.Map{
		{"id": "p1-g1", "text": "f", "font": "Type3 (209 0 R)", "line": int64(1), "size": 10.0, "baseline": []any{20.0, 30.0},
			"metric_quad": []any{20.0, 22.0, 25.0, 22.0, 20.0, 31.0, 25.0, 31.0}},
		{"id": "p1-g2", "text": "i", "font": "Type3 (209 0 R)", "line": int64(1), "size": 10.0, "baseline": []any{25.0, 30.0},
			"metric_quad": []any{25.0, 22.0, 25.0, 22.0, 25.0, 31.0, 25.0, 31.0}},
	}
	bound := paint.bind(records)
	if len(bound) != 1 || bound[0]["text"] != "fi" {
		t.Fatalf("bound %v", bound)
	}
}

// A stroked glyph's paint keeps the trace's line width and that width through the CTM.
func TestStrokedPaintKeepsItsLineWidths(t *testing.T) {
	trace := nativetrace.Parse([]byte(`<document><page>` +
		`<stroke_text linewidth="2" transform=".1 0 0 -.1 5 50">` +
		`<span font="F" wmode="0" trm="75 0 0 75"><g unicode="x" glyph="1" x="0" y="30" adv=".5"/></span>` +
		`</stroke_text></page></document>`)).Descendants("page")[0]
	paint := newUnicodeGlyphs(trace).painted[0]
	device, _ := rec.Number(paint["device_line_width"])
	if paint["line_width"] != 2.0 || device < 0.2-1e-9 || device > 0.2+1e-9 {
		t.Fatalf("line widths %v %v", paint["line_width"], paint["device_line_width"])
	}
}
