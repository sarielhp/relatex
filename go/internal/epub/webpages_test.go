package epub

import (
	"strings"
	"testing"
)

// The web page marks each print page before its block, where the EPUB package does: in a flow
// parent before the block itself, before a list rather than inside it, never inside a heading. The
// page is kept byte for byte around the markers, and its stylesheet keeps them out of sight.
func TestWebPageMarksEachPrintPage(t *testing.T) {
	page := `<!DOCTYPE html><html><head><style>p{}</style></head><body><main id="main"><section id="s" data-structure="section">` +
		`<h2 id="h">One</h2><p id="a" data-block="paragraph">x</p><ol id="l"><li id="i"><p id="b">y</p></li></ol></section></main></body></html>`
	marks := []PageMark{{Page: 1, Label: "i", Node: "h"}, {Page: 2, Label: "7", Node: "a"}, {Page: 3, Label: "", Node: "b"},
		{Page: 4, Label: "9", Node: "missing"}}
	marked := MarkWebPages(page, marks)
	for _, want := range []string{
		`<hr id="page-1" role="doc-pagebreak" aria-label="Page i"/><h2 id="h">`,
		`<hr id="page-2" role="doc-pagebreak" aria-label="Page 7"/><p id="a"`,
		`<li id="i"><hr id="page-3" role="doc-pagebreak" aria-label="Page 3"/><p id="b">`,
		PageBreakStyle,
	} {
		if !strings.Contains(marked, want) {
			t.Errorf("missing %s in %s", want, marked)
		}
	}
	if strings.Contains(marked, "page-4") || strings.Count(marked, "doc-pagebreak") != 4 {
		t.Errorf("a mark without its block is written: %s", marked)
	}
	if MarkWebPages(page, nil) != page {
		t.Errorf("a page without marks changed")
	}
}
