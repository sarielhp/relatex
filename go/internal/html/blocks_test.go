package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func blockDocument(semantics rec.Map) *Paper {
	nodes := rec.Map{
		"b": rec.Map{"id": "b", "type": "theorem", "children": []any{"t", "p"}, "semantics": semantics},
		"t": rec.Map{"id": "t", "type": "paragraph", "children": []any{}, "content": []any{rec.Map{"type": "text", "text": "Title"}}},
		"p": rec.Map{"id": "p", "type": "paragraph", "children": []any{}, "content": []any{rec.Map{"type": "text", "text": "Body"}}},
	}
	return &Paper{document: rec.Map{"nodes": nodes}, nodes: nodes}
}

// A beamer block is a theorem box named by its title paragraph; its family names its border
// (decision 083, amendment 1).
func TestBlockIsAGroupNamedByItsTitle(t *testing.T) {
	paper := blockDocument(rec.Map{"kind": "theorem", "beamer_block": "alerted"})
	html := paper.RenderNode("b")
	for _, want := range []string{`data-structure="theorem"`, `class="theorem block block-alerted"`, `role="group" aria-labelledby="t"`, `>Title<`, `>Body<`} {
		if !strings.Contains(html, want) {
			t.Errorf("block lacks %s: %s", want, html)
		}
	}
	if !strings.Contains(paper.stylesheet(), ".block-alerted") {
		t.Error("the stylesheet lacks the block families")
	}
}

// An untitled block has no title to name it, its family does; an unknown family reads as a plain block; a
// theorem is no block, and a page without blocks keeps its stylesheet.
func TestUntitledUnknownAndOrdinaryTheorems(t *testing.T) {
	untitled := blockDocument(rec.Map{"kind": "theorem", "beamer_block": "custom", "untitled": true}).RenderNode("b")
	if strings.Contains(untitled, "aria-labelledby") || !strings.Contains(untitled, `class="theorem block" role="group" aria-label="Block"`) {
		t.Errorf("untitled block = %s", untitled)
	}
	theorem := blockDocument(rec.Map{"kind": "theorem", "environment": "theorem"})
	if html := theorem.RenderNode("b"); !strings.HasPrefix(html, `<div id="b" data-structure="theorem" class="theorem">`) {
		t.Errorf("theorem = %s", html)
	}
	if strings.Contains(theorem.stylesheet(), ".block-alerted") {
		t.Error("a page without blocks carries the block stylesheet")
	}
}
