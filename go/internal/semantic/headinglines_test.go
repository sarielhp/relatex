package semantic

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A heading set in several lines (decision 126) breaks visibly in Typst and stays one line in
// Markdown, whose headings cannot break; both read the lines joined by the span's space.
func TestHeadingLinesBreakInTypstOnly(t *testing.T) {
	nodes := rec.Map{"h": rec.Map{"id": "h", "type": "heading", "children": []any{}, "content": []any{
		rec.Map{"type": "text", "text": "Appendix A"},
		rec.Map{"type": "text", "text": " ", "line_break": true},
		rec.Map{"type": "text", "text": "Proof"},
	}}}
	md := &markdown{}
	md.export = &export{w: md, nodes: nodes}
	if got, want := md.export.renderNode("h", 0), "<a id=\"h\"></a>\n\n## Appendix A Proof"; got != want {
		t.Errorf("markdown = %q, want %q", got, want)
	}
	ty := &typst{}
	ty.export = &export{w: ty, nodes: nodes}
	want := `#heading(level: 2)[#text("Appendix A")#text(" ")#linebreak()#text("Proof")] <h>`
	if got := ty.export.renderNode("h", 0); got != want {
		t.Errorf("typst = %q, want %q", got, want)
	}
}
