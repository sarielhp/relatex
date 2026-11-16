// The page's landmarks the HTML outputs and the accessibility report share (plan accessibility
// §7.2.3, decisions 095, 102): the ids of the contents and the main element, and the section the
// bibliography is, a doc-bibliography landmark.

package htmlkit

import "github.com/sarielhp/relatex/internal/rec"

// ContentsID is the id of the table of contents at the end of the page, the one page anchor
// that is not a document node (the browser checks expect it when the document has headings).
const ContentsID = "contents"

// MainID is the id of the page's main element, the skip link's target.
const MainID = "main"

// BibliographySection is bibliographySection for a document.
func BibliographySection(document rec.Map) string {
	entries := entryBlocks(document)
	if len(entries) == 0 {
		return ""
	}
	nodes := rec.FetchMap(document, "nodes")
	parents := map[string]string{}
	for id, value := range nodes {
		children, _ := rec.AsMap(value)["children"].([]any)
		for _, child := range children {
			if name, ok := child.(string); ok {
				parents[name] = id
			}
		}
	}
	section := ""
	for entry := range entries {
		around := enclosingSection(entry, nodes, parents)
		if around == "" || section != "" && around != section {
			return ""
		}
		section = around
	}
	children, _ := rec.Node(nodes, section)["children"].([]any)
	if len(children) == 0 || rec.Node(nodes, children[0])["type"] != "heading" || !entriesLead(children[1:], nodes, entries) {
		return ""
	}
	return section
}

var floatBlocks = map[string]bool{"figure_float": true, "table_float": true}

// entriesLead: the children after the heading, floats aside, are the entries (or lists of them)
// and then only end matter, blocks that are no heading or section and hold no entry (addresses,
// running heads).
func entriesLead(children []any, nodes rec.Map, entries map[string]bool) bool {
	lead, rest := 0, false
	for _, child := range children {
		name, ok := child.(string)
		if !ok {
			return false
		}
		kind, _ := rec.Node(nodes, name)["type"].(string)
		switch {
		case floatBlocks[kind]:
		case !rest && onlyEntries(name, nodes, entries):
			lead++
		case kind == "heading" || kind == "section" || holdsEntry(name, nodes, entries):
			return false
		default:
			rest = true
		}
	}
	return lead > 0
}

// holdsEntry: the node is an entry block or holds one.
func holdsEntry(id string, nodes rec.Map, entries map[string]bool) bool {
	if entries[id] {
		return true
	}
	children, _ := rec.Node(nodes, id)["children"].([]any)
	for _, child := range children {
		if name, ok := child.(string); ok && holdsEntry(name, nodes, entries) {
			return true
		}
	}
	return false
}

// onlyEntries: the node is an entry block, or a list whose blocks all are.
func onlyEntries(id string, nodes rec.Map, entries map[string]bool) bool {
	if entries[id] {
		return true
	}
	node := rec.Node(nodes, id)
	kind, _ := node["type"].(string)
	if !entryContainers[kind] {
		return false
	}
	children, _ := node["children"].([]any)
	for _, child := range children {
		if name, ok := child.(string); !ok || !onlyEntries(name, nodes, entries) {
			return false
		}
	}
	return true
}

// entryBlocks are the blocks of the citations graph's bibliography entries.
func entryBlocks(document rec.Map) map[string]bool {
	blocks := map[string]bool{}
	citations, _ := document["citations"].(rec.Map)
	entries, _ := citations["entries"].([]any)
	for _, value := range entries {
		list, _ := rec.AsMap(value)["blocks"].([]any)
		for _, block := range list {
			if id, ok := block.(string); ok {
				blocks[id] = true
			}
		}
	}
	return blocks
}

// enclosingSection is the nearest section node above id, or "".
func enclosingSection(id string, nodes rec.Map, parents map[string]string) string {
	for seen := 0; seen < 1000; seen++ {
		parent, ok := parents[id]
		if !ok {
			return ""
		}
		if rec.Node(nodes, parent)["type"] == "section" {
			return parent
		}
		id = parent
	}
	return ""
}

var entryContainers = map[string]bool{"unordered_list": true, "ordered_list": true, "list_item": true}
