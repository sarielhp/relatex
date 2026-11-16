package epub

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/sarielhp/relatex/internal/pagetree"
)

// PageMark is where a page of the print PDF begins in the reading order: its physical number, its
// label (the printed number) and the main-text block it begins in (papersource.PageList).
type PageMark struct {
	Page  int64
	Label string
	Node  string
}

// PageMarksValue is the marks as a plain value, for the export cache's configuration.
func PageMarksValue(marks []PageMark) []any {
	value := make([]any, len(marks))
	for index, mark := range marks {
		value[index] = []any{mark.Page, mark.Label, mark.Node}
	}
	return value
}

// PrintSource names the print edition the page list follows (dc:source): the PDF's file name and
// SHA-256; "" when the PDF cannot be read.
func PrintSource(pdf string) string {
	file, err := os.Open(pdf)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return filepath.Base(pdf) + ", SHA-256 " + hex.EncodeToString(hash.Sum(nil))
}

// pageBreakSource is the package's pageBreakSource (decision 130): the PDF's identifier (its file
// name and SHA-256, PrintSource), else a statement naming the PDF by the document's title.
func pageBreakSource(printSource, title string) string {
	if printSource != "" {
		return printSource
	}
	if title == "" {
		title = "the document"
	}
	return "relatex: PDF page breaks of " + title
}

// pageEntry is one entry of the page list: the label and the link to its marker.
type pageEntry struct {
	label, href string
}

// flowParents are the elements a page-break marker (a span) may stand in as a child before a block:
// those whose content model takes phrasing content and that a reader does not name from their
// content. A block in any other element (a heading, a list, a table row, a figure whose caption
// must stay first, a formula) is marked before that element instead.
var flowParents = map[string]bool{"div": true, "section": true, "main": true, "article": true, "aside": true,
	"header": true, "footer": true, "blockquote": true, "li": true, "dd": true, "td": true, "th": true, "p": true,
	"details": true}

// markPages puts a page-break marker (epub:type pagebreak, role doc-pagebreak, the label as its
// name) before the element of each mark's block, in the content documents the page was split
// into, and returns the page list: one marker for every mark, in order (decision 130). A mark
// whose block is not on the page goes with the next mark that has a place, so no page is marked
// before its own block would be; a mark without a block (a page after the text) and those still
// waiting at the end go at the end of the last document.
func markPages(documents []contentDocument, marks []PageMark) []pageEntry {
	if len(marks) == 0 || len(documents) == 0 {
		return nil
	}
	places := blockPlaces(documents)
	taken := documentIDs(documents)
	end := endPlace(documents)
	entries := []pageEntry{}
	waiting := []PageMark{}
	for _, mark := range marks {
		at, ok := places[mark.Node]
		if !ok && mark.Node != "" {
			waiting = append(waiting, mark)
			continue
		}
		if !ok {
			at = end
		}
		for _, each := range append(waiting, mark) {
			entries = append(entries, insertMarker(documents, at, each, taken))
		}
		waiting = waiting[:0]
	}
	for _, mark := range waiting {
		entries = append(entries, insertMarker(documents, end, mark, taken))
	}
	return entries
}

// documentIDs are the ids the content documents use.
func documentIDs(documents []contentDocument) map[string]bool {
	taken := map[string]bool{}
	for _, document := range documents {
		for _, child := range document.body {
			child.Walk(func(n *pagetree.Node) {
				if id, ok := n.Attr("id"); ok {
					taken[id] = true
				}
			})
		}
	}
	return taken
}

// insertMarker puts the mark's marker at the place and returns its page-list entry.
func insertMarker(documents []contentDocument, at place, mark PageMark, taken map[string]bool) pageEntry {
	id := markerID(mark.Page, taken)
	label := mark.Label
	if label == "" {
		label = strconv.FormatInt(mark.Page, 10)
	}
	marker := &pagetree.Node{Name: "span", Attrs: []pagetree.Attribute{{Name: "id", Value: id}, {Name: "epub:type", Value: "pagebreak"},
		{Name: "role", Value: "doc-pagebreak"}, {Name: "aria-label", Value: label}}}
	at.insert(marker)
	return pageEntry{label: label, href: documents[at.document].name + "#" + id}
}

// endContainers are the divisions the end of the text is looked for in, from the body down;
// trailers follow the text without being part of it (notes, navigation).
var (
	endContainers = map[string]bool{"main": true, "div": true, "section": true, "article": true}
	trailers      = map[string]bool{"aside": true, "nav": true, "script": true}
)

// endPlace is the end of the text: the end of the innermost division that ends the last
// document, before the notes and navigation that follow it.
func endPlace(documents []contentDocument) place {
	last := len(documents) - 1
	at := place{document: last, body: &documents[last].body}
	children := documents[last].body
	for {
		var final *pagetree.Node
		at.element = nil
		for index := len(children) - 1; index >= 0 && final == nil; index-- {
			switch child := children[index]; {
			case child.Name == "":
			case trailers[child.Name]:
				at.element = child
			default:
				final = child
			}
		}
		if final == nil || !textDivision(final) {
			return at
		}
		at.parent, children = final, final.Children
	}
}

// textDivision: main, a section or an article, or a division of the document's structure (the
// document, a section), not a display (a formula, a code block).
func textDivision(n *pagetree.Node) bool {
	_, structure := n.Attr("data-structure")
	return endContainers[n.Name] && (n.Name != "div" || structure)
}

func markerID(page int64, taken map[string]bool) string {
	id := "page-" + strconv.FormatInt(page, 10)
	for suffix := 2; taken[id]; suffix++ {
		id = "page-" + strconv.FormatInt(page, 10) + "-" + strconv.Itoa(suffix)
	}
	taken[id] = true
	return id
}

// place is where a block's markers go: before the element in its parent (or among the document's
// body children when parent is nil).
type place struct {
	document int
	parent   *pagetree.Node
	body     *[]*pagetree.Node
	element  *pagetree.Node
}

// insert puts the marker just before the element, after markers inserted there before; at the
// end of the parent when the place has no element (endPlace).
func (p place) insert(marker *pagetree.Node) {
	children := p.body
	if p.parent != nil {
		children = &p.parent.Children
	}
	if p.element == nil {
		*children = append(*children, marker)
		return
	}
	for index, child := range *children {
		if child == p.element {
			*children = append((*children)[:index:index], append([]*pagetree.Node{marker}, (*children)[index:]...)...)
			return
		}
	}
}

// blockPlaces maps each element id to its marker place: the element itself when its parent takes
// a marker, else its nearest ancestor whose parent does.
func blockPlaces(documents []contentDocument) map[string]place {
	places := map[string]place{}
	for index := range documents {
		body := &documents[index].body
		for _, child := range *body {
			collectPlaces(child, []*pagetree.Node{nil}, index, body, places)
		}
	}
	return places
}

// collectPlaces records the places of n and its descendants; path holds n's ancestors (nil for
// the body).
func collectPlaces(n *pagetree.Node, path []*pagetree.Node, document int, body *[]*pagetree.Node, places map[string]place) {
	if n.Name == "" {
		return
	}
	if id, ok := n.Attr("id"); ok {
		if _, seen := places[id]; !seen {
			places[id] = markerPlace(append(path, n), document, body)
		}
	}
	if n.Name == "math" || n.Name == "svg" {
		return
	}
	path = append(path[:len(path):len(path)], n)
	for _, child := range n.Children {
		collectPlaces(child, path, document, body, places)
	}
}

// markerPlace: path runs from the body (nil) to the element; the place is the deepest element on
// it whose parent takes a marker, or the outermost element on it that blocks markers.
func markerPlace(path []*pagetree.Node, document int, body *[]*pagetree.Node) place {
	chosen := len(path) - 1
	for index := len(path) - 1; index >= 1; index-- {
		parent := path[index-1]
		if parent == nil || flowParents[parent.Name] {
			chosen = index
			break
		}
	}
	for index := 1; index < chosen; index++ {
		if blocksMarkers(path[index]) {
			chosen = index
			break
		}
	}
	return place{document: document, parent: path[chosen-1], body: body, element: path[chosen]}
}

// blocksMarkers: an element no marker may stand inside, even where its content takes one: a heading
// or a link would read the page number as part of its name, a formula or a drawing cannot hold one,
// a figure keeps its caption first, preformatted text its lines.
func blocksMarkers(n *pagetree.Node) bool {
	switch n.Name {
	case "h1", "h2", "h3", "h4", "h5", "h6", "a", "math", "svg", "figure", "pre", "button", "label":
		return true
	}
	return false
}
