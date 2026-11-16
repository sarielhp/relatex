package epub

import (
	"strings"

	"github.com/sarielhp/relatex/internal/pagetree"
	"github.com/sarielhp/relatex/internal/rec"
)

// page is the reading page prepared for the package: no script, its stylesheet taken out into
// style.css, the namespaces of XHTML, MathML and SVG declared.
type page struct {
	html        *pagetree.Node
	head        *pagetree.Node
	body        *pagetree.Node
	style       string
	adjustments Adjustments
}

// StylesheetName is the package's stylesheet, beside the content documents.
const StylesheetName = "style.css"

func preparePage(source string) *page {
	document := pagetree.Parse(source)
	p := &page{}
	for _, child := range document.Children {
		if child.Name == "html" {
			p.html = child
		}
	}
	if p.html == nil {
		rec.Fail("EPUB: the page has no <html> element")
	}
	for _, child := range p.html.Children {
		switch child.Name {
		case "head":
			p.head = child
		case "body":
			p.body = child
		}
	}
	if p.head == nil || p.body == nil {
		rec.Fail("EPUB: the page has no <head> or <body>")
	}
	p.style = p.takeStyle()
	removeScripts(p.html)
	p.adjustments = conform(p.html)
	// The package opens in the light palette: reading systems provide their own night mode, and the
	// source colours were chosen for white paper (the author, 2026-10-29).
	p.html.SetAttr("data-theme", "light")
	declareNamespaces(p.html)
	return p
}

// takeStyle removes the head's style elements and the page's security policy (a browser
// setting that reading systems do not use) and returns the stylesheet text.
func (p *page) takeStyle() string {
	var style strings.Builder
	kept := []*pagetree.Node{}
	for _, child := range p.head.Children {
		equiv, _ := child.Attr("http-equiv")
		switch {
		case child.Name == "style":
			style.WriteString(child.TextContent())
		case child.Name == "meta" && strings.EqualFold(equiv, "Content-Security-Policy"):
		case child.Name == "":
		default:
			kept = append(kept, child)
		}
	}
	p.head.Children = append(kept, &pagetree.Node{Name: "link", Attrs: []pagetree.Attribute{{Name: "rel", Value: "stylesheet"}, {Name: "href", Value: StylesheetName}}})
	return style.String()
}

// removeScripts drops every script: EPUB content holds none (decision 058). The reference
// previews and the theme switch go with them; links stay plain links.
func removeScripts(n *pagetree.Node) {
	kept := n.Children[:0]
	for _, child := range n.Children {
		if child.Name == "script" || child.Name == "noscript" {
			continue
		}
		removeScripts(child)
		kept = append(kept, child)
	}
	n.Children = kept
}

// declareNamespaces gives the root its XHTML namespaces and each formula and drawing its own.
func declareNamespaces(root *pagetree.Node) {
	lang, _ := root.Attr("lang")
	root.SetAttr("xmlns", xhtmlNamespace)
	root.SetAttr("xmlns:epub", opsNamespace)
	if lang != "" {
		root.SetAttr("xml:lang", lang)
	}
	root.Walk(func(n *pagetree.Node) {
		switch n.Name {
		case "math":
			n.SetAttr("xmlns", mathNamespace)
		case "svg":
			n.SetAttr("xmlns", svgNamespace)
			if usesXlink(n) {
				n.SetAttr("xmlns:xlink", xlinkNamespace)
			}
		}
	})
}

func usesXlink(root *pagetree.Node) bool {
	found := false
	root.Walk(func(n *pagetree.Node) {
		for _, a := range n.Attrs {
			found = found || strings.HasPrefix(a.Name, "xlink:")
		}
	})
	return found
}

// title is the text of the head's title element.
func (p *page) title() string {
	for _, child := range p.head.Children {
		if child.Name == "title" {
			return strings.Join(strings.Fields(child.TextContent()), " ")
		}
	}
	return ""
}

// contentDocument is one XHTML file of the package: the body's children it holds.
type contentDocument struct {
	name string
	body []*pagetree.Node
}

// xhtml is the document as a file: the page's head and the document's part of the body.
func (p *page) xhtml(document contentDocument) string {
	var builder strings.Builder
	builder.WriteString(xmlDeclaration + xhtmlDocType + "<html")
	for _, a := range p.html.Attrs {
		builder.WriteString(" " + a.Name + `="` + attributeEscaper.Replace(a.Value) + `"`)
	}
	builder.WriteString(">")
	serialize(&builder, p.head)
	builder.WriteString("<body")
	for _, a := range p.body.Attrs {
		builder.WriteString(" " + a.Name + `="` + attributeEscaper.Replace(a.Value) + `"`)
	}
	builder.WriteString(">")
	for _, child := range document.body {
		serialize(&builder, child)
	}
	builder.WriteString("</body></html>\n")
	return builder.String()
}

// properties are the manifest properties of a content document: mathml when it holds a formula,
// svg when it holds a drawing, remote-resources when it shows a file from the web.
func properties(document contentDocument) []string {
	found := map[string]bool{}
	for _, child := range document.body {
		child.Walk(func(n *pagetree.Node) {
			switch n.Name {
			case "math":
				found["mathml"] = true
			case "svg":
				found["svg"] = true
			}
			if source, ok := n.Attr("src"); ok && isRemote(source) {
				found["remote-resources"] = true
			}
		})
	}
	result := []string{}
	for _, name := range []string{"mathml", "remote-resources", "svg"} {
		if found[name] {
			result = append(result, name)
		}
	}
	return result
}

func isRemote(reference string) bool {
	lower := strings.ToLower(reference)
	return strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:")
}
