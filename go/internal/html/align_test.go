package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A paragraph's recorded alignment is an attribute the page's style sets; the default has none.
func TestParagraphAlignIsAnAttribute(t *testing.T) {
	text := []any{rec.Map{"type": "text", "text": "Authors"}}
	nodes := rec.Map{
		"c": rec.Map{"id": "c", "type": "paragraph", "content": text, "align": "center"},
		"r": rec.Map{"id": "r", "type": "paragraph", "content": text, "align": "right"},
		"l": rec.Map{"id": "l", "type": "paragraph", "content": text},
	}
	paper := &Paper{document: rec.Map{"main_block_order": []any{"c", "r", "l"}}, nodes: nodes}
	for id, want := range map[string]string{"c": ` data-align="center"`, "r": ` data-align="right"`, "l": ""} {
		if got := strings.Contains(paper.RenderNode(id), "data-align"); got != (want != "") {
			t.Errorf("%s: %s", id, paper.RenderNode(id))
		}
		if want != "" && !strings.Contains(paper.RenderNode(id), want) {
			t.Errorf("%s lacks %s", id, want)
		}
	}
}

// A block holding a table is a div; it carries the alignment too (papers5/1403, papers2/0512 authors).
func TestTableBlockCarriesAlign(t *testing.T) {
	content := []any{rec.Map{"type": "table_grid", "id": "t1", "grid": rec.Map{}}}
	node := rec.Map{"id": "b", "type": "paragraph", "content": content, "align": "center"}
	if got := alignAttribute(node); got != ` data-align="center"` || alignAttribute(rec.Map{}) != "" {
		t.Errorf("attribute %q", got)
	}
}
