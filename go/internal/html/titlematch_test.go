package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// titlePaper is a document with a front matter title and one paragraph that shows it.
func titlePaper(title, paragraph string) *Paper {
	nodes := rec.Map{
		"root": rec.Map{"id": "root", "type": "document", "children": []any{"t"}},
		"t": rec.Map{"id": "t", "type": "paragraph",
			"content": []any{rec.Map{"type": "text", "text": paragraph}}},
	}
	document := rec.Map{"root": "root", "nodes": nodes, "frontmatter": rec.Map{"title": title},
		"main_block_order": []any{"t"}}
	return &Paper{document: document, nodes: nodes}
}

// A title the class typesets in upper case (amsart's \uppercasenonmath) is still the page's h1,
// and the heading keeps the paragraph's own text, not the front matter's.
func TestTitleBlockMatchesIgnoringCase(t *testing.T) {
	paper := titlePaper("Anisotropic 2D FUP and quantum open baker’s map",
		" ANISOTROPIC 2D FUP AND QUANTUM OPEN BAKER’S MAP")
	if got := paper.titleBlock(); got != "t" {
		t.Fatalf("title block %q, want t", got)
	}
	page := paper.RenderNode("root")
	if !strings.Contains(page, `<h1 id="t" data-block="paragraph" class="paragraph title">`) ||
		!strings.Contains(page, "ANISOTROPIC 2D FUP AND QUANTUM OPEN BAKER’S MAP") {
		t.Errorf("title heading: %s", page)
	}
}

// A paragraph that only begins with the title is not the title.
func TestTitleBlockRequiresTheWholeTitle(t *testing.T) {
	paper := titlePaper("Anisotropic 2D FUP and quantum open baker’s map", "ANISOTROPIC 2D FUP")
	if got := paper.titleBlock(); got != "" {
		t.Fatalf("title block %q, want none", got)
	}
}
