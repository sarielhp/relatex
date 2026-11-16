package epub

import (
	"fmt"
	"strings"

	"github.com/sarielhp/relatex/internal/pagetree"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/split"
)

// FirstDocument is the content document the package opens on.
const FirstDocument = "index.xhtml"

// splitPage divides the body into one content document per top-level section, the front matter
// before them in the first, the notes and the contents after them in the last. It keeps one
// document when the page has fewer than two top-level sections or when a split would leave a
// reference that cannot cross files (an ARIA or table-header id, a drawing's url(#id)). Links to
// another file's ids are rewritten to name the file.
func splitPage(p *page) (documents []contentDocument, reason string) {
	documents, reason = sectionDocuments(p.body)
	return linkedDocuments(p, documents, reason)
}

// splitBookPage divides a book's body into the documents of its split (decision 081): the front
// matter, each part's opening and each chapter, the contents after the last. A paper, or a book
// written with --split none, is divided by its top-level sections (splitPage).
func splitBookPage(p *page, document rec.Map, mode string) (documents []contentDocument, reason string, chapters bool) {
	if mode != "none" {
		if division, _ := split.Divide(document, p.body); division != nil {
			documents, reason = linkedDocuments(p, chapterDocuments(division), "")
			return documents, reason, reason == ""
		}
	}
	documents, reason = splitPage(p)
	return documents, reason, false
}

// chapterDocuments are the split's units as content documents: the first named index.xhtml and
// holding what precedes main, the last also holding what follows it (the contents).
func chapterDocuments(division *split.Division) []contentDocument {
	documents := make([]contentDocument, len(division.Units))
	for index, unit := range division.Units {
		name := strings.TrimSuffix(unit.Name, ".html") + ".xhtml"
		body := []*pagetree.Node{unit.Main}
		if index == 0 {
			name = FirstDocument
			body = append(append([]*pagetree.Node{}, division.Before...), unit.Main)
		}
		if index == len(division.Units)-1 {
			body = append(body, division.After...)
		}
		documents[index] = contentDocument{name: name, body: body}
	}
	return documents
}

// linkedDocuments are the documents with their links into each other, or the whole page and the
// reason when there are none or a reference cannot cross files.
func linkedDocuments(p *page, documents []contentDocument, reason string) ([]contentDocument, string) {
	whole := []contentDocument{{name: FirstDocument, body: p.body.Children}}
	if documents == nil {
		return whole, reason
	}
	owner := idOwners(documents)
	for index, document := range documents {
		if broken := crossReference(document, index, owner); broken != "" {
			return whole, broken
		}
	}
	for index, document := range documents {
		rewriteLinks(document, index, owner, documents)
	}
	return documents, ""
}

// sectionDocuments is the split of the body, or nil and the reason the page stays whole.
func sectionDocuments(body *pagetree.Node) ([]contentDocument, string) {
	mainIndex, main := childNamed(body, "main")
	if main == nil {
		return nil, "the page has no main element"
	}
	wrapperIndex, wrapper := documentWrapper(main)
	if wrapper == nil {
		return nil, "the page has no document element"
	}
	groups := sectionGroups(wrapper)
	if len(groups) < 2 {
		return nil, "the document has fewer than two top-level sections"
	}
	documents := make([]contentDocument, len(groups))
	for index, group := range groups {
		shell := &pagetree.Node{Name: wrapper.Name, Attrs: withoutID(wrapper.Attrs), Children: group}
		if index == 0 {
			shell.Attrs = wrapper.Attrs
		}
		mainChildren := []*pagetree.Node{shell}
		var bodyChildren []*pagetree.Node
		if index == 0 {
			bodyChildren = append(bodyChildren, body.Children[:mainIndex]...)
			mainChildren = append(main.Children[:wrapperIndex:wrapperIndex], shell)
		}
		if index == len(groups)-1 {
			mainChildren = append(mainChildren, main.Children[wrapperIndex+1:]...)
		}
		bodyChildren = append(bodyChildren, &pagetree.Node{Name: "main", Attrs: main.Attrs, Children: mainChildren})
		if index == len(groups)-1 {
			bodyChildren = append(bodyChildren, body.Children[mainIndex+1:]...)
		}
		documents[index] = contentDocument{name: documentName(index), body: bodyChildren}
	}
	return documents, ""
}

func documentName(index int) string {
	if index == 0 {
		return FirstDocument
	}
	return fmt.Sprintf("section-%d.xhtml", index)
}

func childNamed(parent *pagetree.Node, name string) (int, *pagetree.Node) {
	for index, child := range parent.Children {
		if child.Name == name {
			return index, child
		}
	}
	return -1, nil
}

// documentWrapper is the element of the document's root node inside main.
func documentWrapper(main *pagetree.Node) (int, *pagetree.Node) {
	for index, child := range main.Children {
		if structure, _ := child.Attr("data-structure"); structure == "document" {
			return index, child
		}
	}
	return -1, nil
}

func withoutID(attrs []pagetree.Attribute) []pagetree.Attribute {
	result := []pagetree.Attribute{}
	for _, a := range attrs {
		if a.Name != "id" {
			result = append(result, a)
		}
	}
	return result
}

// sectionGroups are the wrapper's children in groups: the front matter before the first section
// (when it holds an element), then each section with what follows it up to the next section.
func sectionGroups(wrapper *pagetree.Node) [][]*pagetree.Node {
	groups := [][]*pagetree.Node{}
	var current []*pagetree.Node
	sections := 0
	for _, child := range wrapper.Children {
		if child.Name == "section" {
			sections++
			if hasElement(current) {
				groups = append(groups, current)
				current = nil
			}
		}
		current = append(current, child)
	}
	if hasElement(current) {
		groups = append(groups, current)
	}
	if sections < 2 {
		return nil
	}
	return groups
}

func hasElement(nodes []*pagetree.Node) bool {
	for _, n := range nodes {
		if n.Name != "" {
			return true
		}
	}
	return false
}

// idOwners maps each id to the index of the document that holds it.
func idOwners(documents []contentDocument) map[string]int {
	owner := map[string]int{}
	for index, document := range documents {
		for _, child := range document.body {
			child.Walk(func(n *pagetree.Node) {
				if id, ok := n.Attr("id"); ok {
					if _, seen := owner[id]; !seen {
						owner[id] = index
					}
				}
			})
		}
	}
	return owner
}

// crossReference names a same-document reference whose target the split put in another file.
func crossReference(document contentDocument, index int, owner map[string]int) string {
	broken := ""
	for _, child := range document.body {
		child.Walk(func(n *pagetree.Node) {
			for _, target := range split.LocalTargets(n) {
				if at, known := owner[target]; known && at != index && broken == "" {
					broken = fmt.Sprintf("<%s> refers to #%s in another section", n.Name, target)
				}
			}
		})
	}
	return broken
}

// rewriteLinks points each link to an id in another document at that document.
func rewriteLinks(document contentDocument, index int, owner map[string]int, documents []contentDocument) {
	for _, child := range document.body {
		child.Walk(func(n *pagetree.Node) {
			href, ok := n.Attr("href")
			if n.Name != "a" || !ok || !strings.HasPrefix(href, "#") {
				return
			}
			if at, known := owner[href[1:]]; known && at != index {
				n.SetAttr("href", documents[at].name+href)
			}
		})
	}
}
