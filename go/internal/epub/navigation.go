package epub

import (
	"strings"

	"github.com/sarielhp/relatex/internal/pagetree"
)

// NavigationName is the navigation document: table of contents and landmarks.
const NavigationName = "nav.xhtml"

// navigation is nav.xhtml: the table of contents from the page's own (its headings, each link
// pointing into the content document that holds the heading), and the landmarks: the contents,
// the start of the text, and the bibliography when the document has one; and the page list of
// the print edition when the text marks its pages.
func navigation(p *page, documents []contentDocument, pages []pageEntry, title string) string {
	owner := idOwners(documents)
	var builder strings.Builder
	lang, _ := p.html.Attr("lang")
	builder.WriteString(xmlDeclaration + xhtmlDocType + `<html xmlns="` + xhtmlNamespace + `" xmlns:epub="` + opsNamespace + `"`)
	if lang != "" {
		builder.WriteString(` lang="` + attributeEscaper.Replace(lang) + `" xml:lang="` + attributeEscaper.Replace(lang) + `"`)
	}
	builder.WriteString(`><head><meta charset="utf-8"/><title>` + textEscaper.Replace(title) + `</title>` +
		`<link rel="stylesheet" href="` + StylesheetName + `"/></head><body>`)
	builder.WriteString(`<nav epub:type="toc" role="doc-toc" id="toc"><h1>Contents</h1>`)
	serialize(&builder, contentsList(documents, owner, title))
	builder.WriteString(`</nav>`)
	builder.WriteString(`<nav epub:type="landmarks" id="landmarks" hidden="hidden"><h2>Landmarks</h2><ol>` +
		`<li><a epub:type="bodymatter" href="` + FirstDocument + `">Start of content</a></li>`)
	if target := bibliographyTarget(documents, owner); target != "" {
		builder.WriteString(`<li><a epub:type="bibliography" href="` + attributeEscaper.Replace(target) + `">Bibliography</a></li>`)
	}
	builder.WriteString("</ol></nav>")
	writePageList(&builder, pages)
	builder.WriteString("</body></html>\n")
	return builder.String()
}

// writePageList writes the page-list navigation: each page of the print edition, by its label,
// linked to its page-break marker; nothing without marks.
func writePageList(builder *strings.Builder, pages []pageEntry) {
	if len(pages) == 0 {
		return
	}
	builder.WriteString(`<nav epub:type="page-list" role="doc-pagelist" id="page-list" hidden="hidden"><h2>Pages</h2><ol>`)
	for _, entry := range pages {
		builder.WriteString(`<li><a href="` + attributeEscaper.Replace(entry.href) + `">` + textEscaper.Replace(entry.label) + `</a></li>`)
	}
	builder.WriteString(`</ol></nav>`)
}

// contentsList is the table of contents as a list: a copy of the page's contents list with its
// links into the package, or one entry for the whole document when the page has none.
func contentsList(documents []contentDocument, owner map[string]int, title string) *pagetree.Node {
	list := pageContents(documents)
	if list == nil {
		if title == "" {
			title = "Document"
		}
		link := &pagetree.Node{Name: "a", Attrs: []pagetree.Attribute{{Name: "href", Value: FirstDocument}}, Children: []*pagetree.Node{{Text: title}}}
		return &pagetree.Node{Name: "ol", Children: []*pagetree.Node{{Name: "li", Children: []*pagetree.Node{link}}}}
	}
	list = pagetree.Clone(list)
	headings := idElements(documents)
	list.Walk(func(n *pagetree.Node) {
		href, ok := n.Attr("href")
		if n.Name != "a" || !ok || !strings.HasPrefix(href, "#") {
			return
		}
		n.SetAttr("href", packageLink(documents, owner, href[1:]))
		if strings.TrimSpace(n.TextContent()) == "" {
			n.Children = []*pagetree.Node{{Text: fallbackLabel(headings[href[1:]])}}
		}
	})
	return list
}

// fallbackLabel names a heading whose contents entry has no text (a heading of formulas only).
func fallbackLabel(heading *pagetree.Node) string {
	if heading != nil {
		if text := strings.Join(strings.Fields(heading.TextContent()), " "); text != "" {
			return text
		}
	}
	return "Untitled section"
}

func packageLink(documents []contentDocument, owner map[string]int, id string) string {
	at, known := owner[id]
	if !known {
		return FirstDocument
	}
	return documents[at].name + "#" + id
}

// pageContents is the list of the page's contents navigation (htmlkit.ContentsID), if any.
func pageContents(documents []contentDocument) *pagetree.Node {
	var list *pagetree.Node
	for _, document := range documents {
		for _, child := range document.body {
			child.Walk(func(n *pagetree.Node) {
				if id, _ := n.Attr("id"); n.Name == "nav" && id == "contents" && list == nil {
					_, list = childNamed(n, "ol")
				}
			})
		}
	}
	return list
}

func idElements(documents []contentDocument) map[string]*pagetree.Node {
	elements := map[string]*pagetree.Node{}
	for _, document := range documents {
		for _, child := range document.body {
			child.Walk(func(n *pagetree.Node) {
				if id, ok := n.Attr("id"); ok {
					elements[id] = n
				}
			})
		}
	}
	return elements
}

// bibliographyTarget is the link to the bibliography: the section that holds the first
// bibliography entry, else the nearest element with an id around it; "" without entries.
func bibliographyTarget(documents []contentDocument, owner map[string]int) string {
	for _, document := range documents {
		for _, child := range document.body {
			if id := entryContainer(child, nil); id != "" {
				return packageLink(documents, owner, id)
			}
		}
	}
	return ""
}

// entryContainer searches n for a bibliography entry, given the elements around n.
func entryContainer(n *pagetree.Node, ancestors []*pagetree.Node) string {
	if n.Name == "" {
		return ""
	}
	path := append(ancestors[:len(ancestors):len(ancestors)], n)
	if _, ok := n.Attr("data-bibliography-entry"); ok {
		return containerID(path)
	}
	for _, child := range n.Children {
		if id := entryContainer(child, path); id != "" {
			return id
		}
	}
	return ""
}

func containerID(path []*pagetree.Node) string {
	nearest := ""
	for index := len(path) - 1; index >= 0; index-- {
		id, ok := path[index].Attr("id")
		if !ok {
			continue
		}
		if path[index].Name == "section" {
			return id
		}
		if nearest == "" {
			nearest = id
		}
	}
	return nearest
}
