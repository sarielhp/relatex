// Package split cuts the verified reading page of a book into one file per chapter (decision
// 081): the front matter, each part's opening, each chapter, appendix, bibliography and index.
// The page itself is built and verified whole; the split is mechanical, and Check proves it
// faithful.
//
// It is an output library (decision 102), over pagetree only: the html output publishes the split
// book and the EPUB divides its content documents with it.
package split

import (
	"strings"

	"github.com/sarielhp/relatex/internal/pagetree"
	"github.com/sarielhp/relatex/internal/rec"
)

// NoChapters is the reason a document without \chapter stays one page: a paper, not a book.
const NoChapters = "the document has no chapters"

// Modes are the values of --split: one file per chapter for a book (the default), or the whole page.
var Modes = []string{"chapters", "none"}

// ContinuedAttribute marks the copy of an element opened in an earlier file (the document, a
// part): it holds that element's id, which the copy leaves to the file that opens it.
const ContinuedAttribute = "data-split-continued"

// Unit is one file of the split: its name, title and kind (front, part or chapter), and the
// main element it shows.
type Unit struct {
	Name  string
	Title string
	Kind  string
	Main  *pagetree.Node
}

// Division is the page cut into units: what precedes main in the body (every file repeats it),
// the units in reading order, and what follows main (the contents, kept for the contents page).
type Division struct {
	Before []*pagetree.Node
	Units  []Unit
	After  []*pagetree.Node
}

// piece is a unit before it is named and shelled: nodes at the document level before a part
// (lead), the part it lies in (nil at the document level), its own nodes, and document-level
// nodes after the part (trail).
type piece struct {
	kind                 string
	part                 *pagetree.Node
	lead, content, trail []*pagetree.Node
}

// Divide cuts the page body into units, or tells why the page stays whole: the document has no
// \chapter, the page has no main or document element, or an element a later file continues
// has no id.
func Divide(document rec.Map, body *pagetree.Node) (*Division, string) {
	counters := headingCounters(document)
	if !hasCounter(counters, "chapter") {
		return nil, NoChapters
	}
	mainIndex, main := pagetree.Child(body, "main")
	if main == nil {
		return nil, "the page has no main element"
	}
	wrapperIndex := -1
	for index, child := range main.Children {
		if structure, _ := child.Attr("data-structure"); structure == "document" {
			wrapperIndex = index
			break
		}
	}
	if wrapperIndex < 0 {
		return nil, "the page has no document element"
	}
	wrapper := main.Children[wrapperIndex]
	pieces := documentPieces(wrapper, counters)
	if len(pieces) < 2 {
		return nil, "the document has fewer than two chapters"
	}
	if reason := shellable(wrapper, pieces); reason != "" {
		return nil, reason
	}
	division := &Division{Before: body.Children[:mainIndex], After: body.Children[mainIndex+1:]}
	assigned := assignNotes(pieces, main.Children[wrapperIndex+1:])
	names := newNamer()
	for index, p := range pieces {
		shell := &pagetree.Node{Name: main.Name, Attrs: main.Attrs}
		if index == 0 {
			shell.Children = append(shell.Children, main.Children[:wrapperIndex]...)
		}
		shell.Children = append(shell.Children, p.shelled(wrapper, index == 0))
		shell.Children = append(shell.Children, assigned[index]...)
		name, title := names.name(p)
		division.Units = append(division.Units, Unit{Name: name, Title: title, Kind: p.kind, Main: shell})
	}
	return division, ""
}

// headingCounters are the sectioning counters of the main text's headings, by node id.
func headingCounters(document rec.Map) map[string]string {
	counters := map[string]string{}
	nodes, _ := document["nodes"].(rec.Map)
	order, _ := document["main_block_order"].([]any)
	for _, id := range order {
		node := rec.AsMap(nodes[rec.AsString(id)])
		if node["type"] != "heading" {
			continue
		}
		semantics, _ := node["semantics"].(rec.Map)
		if counter, ok := semantics["counter"].(string); ok {
			counters[rec.AsString(id)] = counter
		}
	}
	return counters
}

func hasCounter(counters map[string]string, wanted string) bool {
	for _, counter := range counters {
		if counter == wanted {
			return true
		}
	}
	return false
}

// documentPieces are the units of the document element's children: the front matter before the
// first section, each top-level section, and each part's opening and sections.
func documentPieces(wrapper *pagetree.Node, counters map[string]string) []*piece {
	var pieces []*piece
	var pending []*pagetree.Node
	var last *piece
	for _, child := range wrapper.Children {
		switch {
		case child.Name == "section" && counters[leadingHeadingID(child)] == "part":
			pieces = append(pieces, partPieces(child, pending)...)
			pending, last = nil, nil
		case child.Name == "section":
			last = &piece{kind: "chapter", content: append(pending, child)}
			pieces = append(pieces, last)
			pending = nil
		case last != nil:
			last.content = append(last.content, child)
		case len(pieces) == 0 && child.Name != "":
			last = &piece{kind: "front", content: append(pending, child)}
			pieces = append(pieces, last)
			pending = nil
		default:
			pending = append(pending, child)
		}
	}
	if len(pending) > 0 && len(pieces) > 0 {
		pieces[len(pieces)-1].trail = pending
	}
	return pieces
}

// partPieces are a part's opening (its heading and what precedes its first section) and one
// piece per section inside it.
func partPieces(part *pagetree.Node, lead []*pagetree.Node) []*piece {
	opening := &piece{kind: "part", part: part, lead: lead}
	pieces := []*piece{opening}
	current := opening
	for _, child := range part.Children {
		if child.Name == "section" {
			current = &piece{kind: "chapter", part: part, content: []*pagetree.Node{child}}
			pieces = append(pieces, current)
			continue
		}
		current.content = append(current.content, child)
	}
	return pieces
}

// leadingHeadingID is the id of a section's first heading.
func leadingHeadingID(section *pagetree.Node) string {
	for _, child := range leadingHeadings(section) {
		id, _ := child.Attr("id")
		return id
	}
	return ""
}

// leadingHeadings are the headings a section opens with (a chapter's number line and its title).
// The page's page marker comes before each section's heading (decision 081), and a marker hides
// no heading: it is skipped as white space is, while any other element ends the run.
func leadingHeadings(section *pagetree.Node) []*pagetree.Node {
	var headings []*pagetree.Node
	for _, child := range section.Children {
		if (child.Name == "" && strings.TrimSpace(child.Text) == "") || isPageMarker(child) {
			continue
		}
		if len(child.Name) != 2 || child.Name[0] != 'h' || child.Name[1] < '1' || child.Name[1] > '6' {
			break
		}
		headings = append(headings, child)
	}
	return headings
}

// isPageMarker is the page's page-break marker: an hr with role doc-pagebreak, and nothing else.
func isPageMarker(n *pagetree.Node) bool {
	role, _ := n.Attr("role")
	return n.Name == "hr" && role == "doc-pagebreak"
}

// shellable names an element a later file continues that has no id to continue it by.
func shellable(wrapper *pagetree.Node, pieces []*piece) string {
	if _, ok := wrapper.Attr("id"); !ok {
		return "the document element has no id"
	}
	for _, p := range pieces {
		if p.part == nil {
			continue
		}
		if _, ok := p.part.Attr("id"); !ok {
			return "a part has no id"
		}
	}
	return ""
}

// shelled is the piece inside copies of the elements around it: the document element, opened
// by the first unit, and its part, opened by the part's opening.
func (p *piece) shelled(wrapper *pagetree.Node, first bool) *pagetree.Node {
	outer := shell(wrapper, first)
	if p.part == nil {
		outer.Children = concat(p.lead, p.content, p.trail)
		return outer
	}
	inner := shell(p.part, p.kind == "part")
	inner.Children = p.content
	outer.Children = concat(p.lead, []*pagetree.Node{inner}, p.trail)
	return outer
}

func concat(lists ...[]*pagetree.Node) []*pagetree.Node {
	var result []*pagetree.Node
	for _, list := range lists {
		result = append(result, list...)
	}
	return result
}

// shell is an empty copy of the element: with its id in the file that opens it, else marked as
// the continuation of that id.
func shell(element *pagetree.Node, opens bool) *pagetree.Node {
	if opens {
		return &pagetree.Node{Name: element.Name, Attrs: element.Attrs}
	}
	id, _ := element.Attr("id")
	attrs := []pagetree.Attribute{}
	for _, a := range element.Attrs {
		if a.Name != "id" {
			attrs = append(attrs, a)
		}
	}
	return &pagetree.Node{Name: element.Name, Attrs: append(attrs, pagetree.Attribute{Name: ContinuedAttribute, Value: id})}
}

// assignNotes gives each element after the document element (the notes) to the unit of the first
// link to it, never to a unit before the previous note's, so the notes keep their order.
func assignNotes(pieces []*piece, notes []*pagetree.Node) [][]*pagetree.Node {
	referrer := map[string]int{}
	for index, p := range pieces {
		for _, n := range concat(p.lead, p.content, p.trail) {
			n.Walk(func(element *pagetree.Node) {
				if href, ok := element.Attr("href"); ok && element.Name == "a" && strings.HasPrefix(href, "#") {
					if _, seen := referrer[href[1:]]; !seen {
						referrer[href[1:]] = index
					}
				}
			})
		}
	}
	assigned := make([][]*pagetree.Node, len(pieces))
	current := 0
	for _, note := range notes {
		if id, ok := note.Attr("id"); ok {
			if at, known := referrer[id]; known && at > current {
				current = at
			}
		}
		assigned[current] = append(assigned[current], note)
	}
	return assigned
}
