package epub

import (
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/pagetree"
)

// PageBreakStyle keeps the web page's page-break markers out of sight and out of the layout: an
// empty hr taken out of the flow (a flex or grid parent gives it no place), one pixel and clipped,
// so a screen reader still finds it and reads its name.
const PageBreakStyle = `[role="doc-pagebreak"] { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; ` +
	`border: 0; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }`

// MarkWebPages puts a page-break marker (an hr, the element axe allows role doc-pagebreak on, named "Page " and the page's label;
// plan accessibility A8) before the element of each mark's block in the web page, where the EPUB
// package puts its own (markPages), so a reader can follow "see page 7" on the page as in the
// book. The page is otherwise kept byte for byte: each marker is written before its element's
// start tag. A mark whose block has no place with an id on the page, or whose place is in a paragraph, is
// left out, and the page's
// stylesheet gains PageBreakStyle when a marker is written.
func MarkWebPages(source string, marks []PageMark) string {
	if len(marks) == 0 {
		return source
	}
	document := pagetree.Parse(source)
	var body *pagetree.Node
	taken := map[string]bool{}
	document.Walk(func(n *pagetree.Node) {
		if n.Name == "body" && body == nil {
			body = n
		}
		if id, ok := n.Attr("id"); ok {
			taken[id] = true
		}
	})
	if body == nil {
		return source
	}
	places := blockPlaces([]contentDocument{{body: body.Children}})
	before := map[string]string{}
	order := []string{}
	for _, mark := range marks {
		place, ok := places[mark.Node]
		target, named := "", false
		// an hr cannot stand in a paragraph: the parser would end the paragraph there
		if ok && (place.parent == nil || place.parent.Name != "p") {
			target, named = place.element.Attr("id")
		}
		if !named {
			continue
		}
		label := mark.Label
		if label == "" {
			label = strconv.FormatInt(mark.Page, 10)
		}
		if _, seen := before[target]; !seen {
			order = append(order, target)
		}
		before[target] += `<hr id="` + markerID(mark.Page, taken) + `" role="doc-pagebreak" aria-label="` +
			htmlkit.Escape("Page "+label) + `"/>`
	}
	return insertMarkers(source, order, before)
}

// insertMarkers writes each element's markers before its start tag, and the marker style into
// the page's first stylesheet.
func insertMarkers(source string, order []string, before map[string]string) string {
	written := false
	for _, id := range order {
		at := startTag(source, id)
		if at < 0 {
			continue
		}
		source = source[:at] + before[id] + source[at:]
		written = true
	}
	if !written {
		return source
	}
	if at := strings.Index(source, "</style>"); at >= 0 {
		source = source[:at] + "\n" + PageBreakStyle + "\n" + source[at:]
	}
	return source
}

// startTag is the offset of the start tag that carries the id, -1 when there is none.
func startTag(source, id string) int {
	attribute := ` id="` + htmlkit.Escape(id) + `"`
	for from := 0; ; {
		found := strings.Index(source[from:], attribute)
		if found < 0 {
			return -1
		}
		found += from
		open := strings.LastIndexByte(source[:found], '<')
		if open >= 0 && !strings.ContainsRune(source[open:found], '>') {
			return open
		}
		from = found + len(attribute)
	}
}
