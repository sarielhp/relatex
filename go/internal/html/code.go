package html

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// codeCSS sets a listing as its PDF prints it: the lines' own text styles name the font, so the
// user agent's monospace for pre and code is not imposed, and a long line wraps instead of
// scrolling the page sideways.
const codeCSS = `pre.code { font: inherit; margin: 1rem 0; white-space: pre-wrap; }
pre.code > code { font: inherit; }
`

// codeNode is a listing (decision 120): one pre whose lines are its paragraph blocks, each a code
// element, separated by line breaks. The lines keep the text the PDF prints (line numbers too).
func (p *Paper) codeNode(identifier string, node rec.Map) string {
	lines := []string{}
	for _, child := range rec.FetchList(node, "children") {
		id := rec.AsString(child)
		line := p.node(id)
		if rec.FetchString(line, "type") != "paragraph" {
			rec.Fail("A code block holds only its lines")
		}
		lines = append(lines, p.withLongDescriptions(func() string {
			return "<code id=\"" + id + "\" data-block=\"paragraph\" class=\"code-line\">" +
				p.spans(rec.FetchList(line, "content")) + "</code>"
		}))
	}
	return "<pre id=\"" + identifier + "\" data-structure=\"code\" class=\"code\">" + strings.Join(lines, "\n") + "</pre>"
}
