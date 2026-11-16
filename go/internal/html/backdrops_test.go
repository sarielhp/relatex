package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/rec"
)

// White text the PDF sets on a fill of its picture (a tcolorbox title, slides 0018; decision 113)
// is shown on that fill: the span paints it (data-fill-box), its ink is the ink on the fill, and
// it is never hidden. White text elsewhere stays the author's hiding. A yellow as light as the
// paper is no hiding (slides 0013): it is adjusted to read.
func TestTextOnABackdropIsPaintedOnIt(t *testing.T) {
	document := rec.Map{
		"nodes": rec.Map{
			"title": rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": " Example", "color": "#ffffff"}}},
			"note":  rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": "hidden", "color": "#ffffff"}}},
			"code":  rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": "travel", "color": "#ffff00"}}},
		},
		"text_paint": rec.Map{"inks": []any{"#ffff00", "#ffffff"},
			"backdrops": []any{rec.Map{"ink": "#ffffff", "fill": "#1c377d", "text": "ExampleExample"}}},
	}
	document[contrast.Field] = contrast.Adjustments(document)
	if hidden := contrast.Hidden(document); !hidden["#ffffff"] || hidden["#ffff00"] {
		t.Errorf("hidden = %v", hidden)
	}
	paper := &Paper{document: document, nodes: rec.FetchMap(document, "nodes")}
	title := paper.spans(rec.FetchList(rec.FetchMap(paper.nodes, "title"), "content"))
	for _, want := range []string{`data-fill-box="1c377d"`, "var(--ink-ffffff-on-1c377d)"} {
		if !strings.Contains(title, want) || strings.Contains(title, "aria-hidden") {
			t.Errorf("missing %s or hidden: %s", want, title)
		}
	}
	note := paper.spans(rec.FetchList(rec.FetchMap(paper.nodes, "note"), "content"))
	if !strings.Contains(note, `aria-hidden="true"`) || strings.Contains(note, "data-fill-box") {
		t.Errorf("white on the paper = %s", note)
	}
	css := inksCSS(document)
	if !strings.Contains(css, `[data-fill-box="1c377d"] { background: var(--fill-1c377d) !important; }`) {
		t.Errorf("the backdrop is not painted: %s", css)
	}
	adjusted := contrast.Recorded(document)
	if _, ok := adjusted["#ffff00"]; !ok {
		t.Errorf("yellow is not adjusted: %v", adjusted)
	}
}

// A link inside text the author hid leaves the tab order (axe aria-hidden-focus, slides 0027).
func TestLinksInHiddenTextAreNotFocusable(t *testing.T) {
	paper := &Paper{document: rec.Map{"nodes": rec.Map{"p": rec.Map{"type": "paragraph",
		"content": []any{rec.Map{"type": "text", "text": "x", "color": "#ffffff"}}}}}}
	html := paper.hiddenText(`<span data-kind="text"><a href="https://fius.de">x</a></span>`, "#ffffff")
	if !strings.Contains(html, `aria-hidden="true"`) || !strings.Contains(html, `<a tabindex="-1" href=`) {
		t.Errorf("hidden link = %s", html)
	}
}
