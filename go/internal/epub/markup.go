package epub

import (
	"strings"

	"github.com/sarielhp/relatex/internal/pagetree"
)

// Namespaces of the content documents.
const (
	xhtmlNamespace = "http://www.w3.org/1999/xhtml"
	mathNamespace  = "http://www.w3.org/1998/Math/MathML"
	svgNamespace   = "http://www.w3.org/2000/svg"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
	opsNamespace   = "http://www.idpf.org/2007/ops"
	xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"
	xhtmlDocType   = "<!DOCTYPE html>\n"
)

var (
	textEscaper      = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attributeEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "\n", "&#10;",
		"\r", "&#13;", "\t", "&#9;")
)

// serialize writes the element as XML: every element closed, every attribute quoted, text escaped.
func serialize(builder *strings.Builder, n *pagetree.Node) {
	if n.Name == "" {
		builder.WriteString(textEscaper.Replace(stripControls(n.Text)))
		return
	}
	builder.WriteString("<" + n.Name)
	for _, a := range n.Attrs {
		builder.WriteString(" " + a.Name + `="` + attributeEscaper.Replace(stripControls(a.Value)) + `"`)
	}
	if len(n.Children) == 0 && pagetree.Void(n.Name) {
		builder.WriteString("/>")
		return
	}
	builder.WriteString(">")
	for _, child := range n.Children {
		serialize(builder, child)
	}
	builder.WriteString("</" + n.Name + ">")
}

// stripControls drops the characters XML 1.0 forbids (C0 controls other than tab, newline and
// carriage return); a page never shows them.
func stripControls(text string) string {
	if strings.IndexFunc(text, isForbidden) < 0 {
		return text
	}
	return strings.Map(func(r rune) rune {
		if isForbidden(r) {
			return -1
		}
		return r
	}, text)
}

func isForbidden(r rune) bool {
	return r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0xFFFE || r == 0xFFFF
}
