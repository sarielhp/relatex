package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/rec"
)

// A deck the PDF fills black before its text (beamer's background canvas, slides 0007; decision
// 105) shows its white text on that canvas: the text is painted, never hidden, its inks are read
// against the canvas (a dark brown is lightened for it, white is not adjusted) and the page paints
// the canvas behind the main text and its structures. Without the canvas the same white is ink the
// author hid.
func TestTextOnTheCanvasReadsAgainstIt(t *testing.T) {
	nodes := func() rec.Map {
		return rec.Map{
			"title": rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": "Lee Cho", "color": "#ffffff"}}},
			"brown": rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": "x", "color": "#332d14"}, rec.Map{"type": "text", "text": " y"}}},
		}
	}
	paint := rec.Map{"inks": []any{"#332d14", "#ffffff"}, "canvas": "#000000"}
	document := rec.Map{"nodes": nodes(), "text_paint": paint}
	document[contrast.Field] = contrast.Adjustments(document)
	if hidden := contrast.Hidden(document); len(hidden) != 0 {
		t.Errorf("hidden inks on the canvas: %v", hidden)
	}
	items := rec.FetchList(rec.AsMap(document[contrast.Field]), "items")
	if len(items) != 1 || rec.AsMap(items[0])["ink"] != "#332d14" || rec.AsMap(items[0])["fill"] != "#000000" {
		t.Errorf("canvas adjustments = %v", items)
	}
	paper := &Paper{document: document, nodes: rec.FetchMap(document, "nodes")}
	title := paper.spans(rec.FetchList(rec.FetchMap(paper.nodes, "title"), "content"))
	if strings.Contains(title, "aria-hidden") || !strings.Contains(title, "var(--ink-ffffff-on-000000)") {
		t.Errorf("white on the canvas = %s", title)
	}
	brown := paper.spans(rec.FetchList(rec.FetchMap(paper.nodes, "brown"), "content"))
	if !strings.Contains(brown, `data-fill="000000"`) {
		t.Errorf("uncoloured text on the canvas is not marked: %s", brown)
	}
	css := inksCSS(document)
	for _, want := range []string{"--fill-000000: #000000;", "--ink-ffffff-on-000000: #ffffff;", "main, main .theorem, main .proof { background: var(--fill-000000); }"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %s in %s", want, css)
		}
	}
	white := rec.Map{"nodes": nodes(), "text_paint": rec.Map{"inks": []any{"#332d14", "#ffffff"}}}
	if !contrast.Hidden(white)["#ffffff"] || strings.Contains(inksCSS(white), "main, main .theorem") {
		t.Errorf("white on white paper is not hidden, or the paper is painted as a canvas")
	}
}
