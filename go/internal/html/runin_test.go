package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A run-in heading's title is a span with role heading one level below its section's heading,
// inside its paragraph; a paragraph whose title is all of it closes the span at its end.
func TestRuninTitleIsAHeadingInItsParagraph(t *testing.T) {
	text := func(values ...string) []any {
		spans := []any{}
		for _, value := range values {
			spans = append(spans, rec.Map{"type": "text", "text": value})
		}
		return spans
	}
	runin := func(count int64) rec.Map {
		return rec.Map{"kind": "heading", "counter": "paragraph", "level": int64(4), "runin": true, "title_spans": count}
	}
	nodes := rec.Map{
		"root":  rec.Map{"id": "root", "type": "document", "children": []any{"s1"}},
		"s1":    rec.Map{"id": "s1", "type": "section", "children": []any{"h1", "p1", "p2", "p3"}},
		"h1":    rec.Map{"id": "h1", "type": "heading", "content": text("1 Intro")},
		"p1":    rec.Map{"id": "p1", "type": "paragraph", "content": text("Setup.", " We start."), "semantics": runin(1)},
		"p2":    rec.Map{"id": "p2", "type": "paragraph", "content": text("Alone."), "semantics": runin(1)},
		"p3":    rec.Map{"id": "p3", "type": "paragraph", "content": text("Plain."), "semantics": runin(2)},
		"after": rec.Map{"id": "after", "type": "paragraph", "content": text("Body")},
	}
	paper := &Paper{document: rec.Map{"root": "root", "nodes": nodes, "main_block_order": []any{"h1", "p1", "p2", "p3"}}, nodes: nodes}
	page := paper.RenderNode("root")
	open := `<span class="runin-heading" role="heading" aria-level="3">`
	if !strings.Contains(page, `<p id="p1" data-block="paragraph" class="paragraph">`+open) {
		t.Errorf("run-in title: %s", page)
	}
	if strings.Count(page, open) != 2 || strings.Count(page, "</span></span>") != 2 {
		t.Errorf("titles and their ends: %s", page)
	}
	if strings.Contains(paper.RenderNode("p3"), "runin-heading") {
		t.Errorf("a title of more spans than the paragraph has is no title")
	}
}
