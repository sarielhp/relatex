package semantic

import (
	"strings"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

// spokenText is a text span's text, or for a glyph with no character but a name (marvosym's
// Tumbler) the name in brackets, since Markdown and Typst have no label to carry it beside U+FFFD.
func spokenText(span rec.Map) string {
	if name, named := span["glyph_name"].(string); named {
		return "[" + inline.GlyphLabel(name) + "]"
	}
	return rec.FetchString(span, "text")
}

// renderedContent is a node's content spans rendered by a writer, and whether the node has any:
// a page total that carries its own printed content renders it instead of its count.
func renderedContent(node rec.Map, render func(rec.Map, rec.Map) string) (string, bool) {
	content, ok := node["content"].([]any)
	if !ok || len(content) == 0 {
		return "", false
	}
	var rendered strings.Builder
	for _, span := range content {
		rendered.WriteString(render(rec.AsMap(span), node))
	}
	return rendered.String(), true
}
