package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A heading of several lines (decision 126: a beamer title broken with \\, title and subtitle, an
// IEEEtran appendix's mark and title) is one h element: the lines are separated by the space every
// reader reads and a <br>. A one-line heading is written as before.
func TestAHeadingOfSeveralLinesIsOneElement(t *testing.T) {
	document := rec.Map{
		"root": "doc",
		"nodes": rec.Map{
			"doc": rec.Map{"type": "document", "children": []any{"s1", "s2"}},
			"s1":  rec.Map{"type": "section", "children": []any{"h1"}},
			"s2":  rec.Map{"type": "section", "children": []any{"h2"}},
			"h1": rec.Map{"type": "heading", "content": []any{
				rec.Map{"type": "text", "text": "Appendix A"},
				rec.Map{"type": "text", "text": " ", "line_break": true},
				rec.Map{"type": "text", "text": "Proof of the Lemma"},
			}},
			"h2": rec.Map{"type": "heading", "content": []any{rec.Map{"type": "text", "text": "Introduction"}}},
		},
	}
	paper := &Paper{document: document, nodes: rec.FetchMap(document, "nodes")}
	two := paper.plainNode("h1", rec.FetchMap(paper.nodes, "h1"), "heading")
	if strings.Count(two, "<h2") != 1 || !strings.Contains(two, `Appendix A</span><span data-line-break data-kind="text"> <br></span><span`) ||
		!strings.Contains(two, "Proof of the Lemma") {
		t.Errorf("two lines, one heading: %s", two)
	}
	one := paper.plainNode("h2", rec.FetchMap(paper.nodes, "h2"), "heading")
	if strings.Contains(one, "<br>") || strings.Count(one, "<h2") != 1 {
		t.Errorf("a one-line heading has no break: %s", one)
	}
}
