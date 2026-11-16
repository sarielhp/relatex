package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/rec"
)

// Text after an author's fill takes the fill's inks (decision 099): a coloured span its ink on
// that fill, an uncoloured one is marked with the fill (no inline colour, which the browser
// inventory would read as its own) and a rule gives it the fill's black, and white on the fill is not hidden. Text before
// the fill and in another paragraph keeps the page's inks.
func TestSpansOnAFillTakeTheFillsInks(t *testing.T) {
	fill := rec.Map{"type": "rule", "color": "#f58a4d", "width_sp": int64(5144052), "height_sp": int64(794296),
		"depth_sp": int64(364118), "margin_left_sp": int64(0), "margin_right_sp": int64(-5144052)}
	white := rec.Map{"type": "text", "text": "Try", "color": "#ffffff"}
	plain := rec.Map{"type": "text", "text": "it"}
	document := rec.Map{"nodes": rec.Map{
		"box":  rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": "a"}, fill, white, plain}},
		"page": rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": "y", "color": "#ffffff"}}},
	}}
	document[contrast.Field] = contrast.Adjustments(document)
	paper := &Paper{document: document, nodes: rec.FetchMap(document, "nodes")}
	box := paper.spans(rec.FetchList(rec.FetchMap(paper.nodes, "box"), "content"))
	for _, want := range []string{`style="color:var(--ink-ffffff-on-f58a4d)" data-kind="text">Try<`, `<span data-fill="f58a4d" data-kind="text">it<`, `<span data-kind="text">a<`, `data-kind="rule" data-fill-box="f58a4d"`} {
		if !strings.Contains(box, want) {
			t.Errorf("missing %s in %s", want, box)
		}
	}
	if strings.Contains(box, "aria-hidden") || paper.fill != "" {
		t.Errorf("white on the fill is hidden, or the fill outlives its list: %s", box)
	}
	page := paper.spans(rec.FetchList(rec.FetchMap(paper.nodes, "page"), "content"))
	if !strings.Contains(page, `aria-hidden="true"`) || !strings.Contains(page, "var(--ink-ffffff)") {
		t.Errorf("white on the page = %s", page)
	}
	if !strings.Contains(darkTheme, "--ink: "+darkInk+";") {
		t.Errorf("darkInk %s is not the dark theme's ink", darkInk)
	}
	css := inksCSS(document)
	for _, want := range []string{"--ink-ffffff-on-f58a4d: #3d3c3c;", "--ink-ffffff-on-f58a4d: #ffffff;", "--fill-f58a4d: #733100;", "--fill-f58a4d: #f58a4d;", "--ink-on-f58a4d: #e9e6df;",
		`[data-fill-box="f58a4d"] { background: var(--fill-f58a4d) !important; }`, "--ink-on-f58a4d: var(--ink);", "--ink-ffffff: transparent;", `[data-fill="f58a4d"] { color: var(--ink-on-f58a4d); }`} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %s in %s", want, css)
		}
	}
}
