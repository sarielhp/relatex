package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A run of page-numbered paragraphs after a "Contents" paragraph is a tight list; a lone numbered
// paragraph and the prose around it keep the page's paragraph spacing.
func TestContentsEntriesAreTight(t *testing.T) {
	text := func(value string) []any { return []any{rec.Map{"type": "text", "text": value}} }
	order := []any{"c", "e1", "e2", "e3", "prose", "lone"}
	nodes := rec.Map{}
	for id, value := range map[string]string{"c": "Contents", "e1": "1. Introduction 2", "e2": "2. Basics 5",
		"e3": "References 38", "prose": "Some text that ends here.", "lone": "Item 7"} {
		nodes[id] = rec.Map{"id": id, "type": "paragraph", "content": text(value)}
	}
	paper := &Paper{document: rec.Map{"main_block_order": order}, nodes: nodes}
	for id, want := range map[string]bool{"c": false, "e1": true, "e2": true, "e3": true, "prose": false, "lone": false} {
		if got := strings.Contains(paper.RenderNode(id), "data-toc-entry"); got != want {
			t.Errorf("%s: tight %v, want %v", id, got, want)
		}
	}
}
