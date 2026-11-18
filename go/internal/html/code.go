package html

import (
	"slices"
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
// A float the page break placed between two lines (a table or figure printed where the listing
// continues) is a child too: the listing is then a div of one pre per run of lines, the float
// written between them where it stands.
func (p *Paper) codeNode(identifier string, node rec.Map) string {
	runs := [][]string{{}}
	floats := []string{}
	for _, child := range rec.FetchList(node, "children") {
		id := rec.AsString(child)
		line := p.node(id)
		kind := rec.FetchString(line, "type")
		if slices.Contains(floatTypes, kind) {
			floats = append(floats, p.RenderNode(id))
			runs = append(runs, []string{})
			continue
		}
		if kind != "paragraph" {
			rec.Fail("A code block holds only its lines")
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], p.withLongDescriptions(func() string {
			return "<code id=\"" + id + "\" data-block=\"paragraph\" class=\"code-line\">" +
				p.spans(rec.FetchList(line, "content")) + "</code>"
		}))
	}
	if len(floats) == 0 {
		return "<pre id=\"" + identifier + "\" data-structure=\"code\" class=\"code\">" + strings.Join(runs[0], "\n") + "</pre>"
	}
	var builder strings.Builder
	builder.WriteString("<div id=\"" + identifier + "\" data-structure=\"code\" class=\"code-split\">")
	for index, run := range runs {
		if len(run) > 0 {
			builder.WriteString("<pre class=\"code\">" + strings.Join(run, "\n") + "</pre>")
		}
		if index < len(floats) {
			builder.WriteString(floats[index])
		}
	}
	builder.WriteString("</div>")
	return builder.String()
}
