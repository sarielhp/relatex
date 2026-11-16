package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A formula in a heading is part of its contents entry: "5. Spectral α(C) for ..." keeps the
// formula's characters (papers/0208's contents and EPUB navigation lost them).
func TestContentsEntryKeepsAHeadingsFormula(t *testing.T) {
	heading := rec.Map{"type": "heading", "semantics": rec.Map{"counter": "section"}, "content": []any{
		rec.Map{"type": "text", "text": "5. Spectral "},
		rec.Map{"type": "math", "node": "m"},
		rec.Map{"type": "text", "text": " for hypercones"}}}
	math := rec.Map{"type": "math", "children": []any{}, "mathml": `<math xmlns="http://www.w3.org/1998/Math/MathML"><mrow><mi>α</mi><mo>(</mo><mi>C</mi><mo>)</mo></mrow></math>`}
	paper := &Paper{document: rec.Map{"main_block_order": []any{"h1"}}, nodes: rec.Map{"h1": heading, "m": math}}
	got := paper.contents()
	if !strings.Contains(got, `<a href="#h1">5. Spectral α(C) for hypercones</a>`) {
		t.Errorf("contents %s", got)
	}
}

// A contents entry whose heading shows no text gets a name; an entry with text needs none.
func TestContentsEntryWithoutTextIsNamed(t *testing.T) {
	nodes := rec.Map{"h1": headingNode(" ", "section"), "h2": headingNode("2 Next", "section")}
	paper := &Paper{document: rec.Map{"main_block_order": []any{"h1", "h2"}}, nodes: nodes}
	got := paper.contents()
	if !strings.Contains(got, `<a href="#h1" aria-label="Heading"> </a>`) || !strings.Contains(got, `<a href="#h2">2 Next</a>`) {
		t.Errorf("contents %s", got)
	}
}
