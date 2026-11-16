// The bibliography's run of entries when no section holds it alone (decision 111): the entries
// that share a section with prose, or sit at the document's top level, still form one landmark.

package htmlkit

import (
	"fmt"

	"github.com/sarielhp/relatex/internal/rec"
)

// BibliographyRun is the parent and the first and last child (indices into the parent's
// children) of the bibliography entries' run in document, and whether there is one: every entry
// block lies in one parent (a section, or the document itself) as one contiguous run of entry
// blocks or lists whose blocks all are entries, at least two of them, and no section landmark
// applies (BibliographySection wins). A float among the entries does not break the run.
func BibliographyRun(document rec.Map) (parent string, first, last int, ok bool) {
	if BibliographySection(document) != "" {
		return "", 0, 0, false
	}
	entries := entryBlocks(document)
	if len(entries) < 2 {
		return "", 0, 0, false
	}
	nodes := rec.FetchMap(document, "nodes")
	parents := map[string]string{}
	for id, value := range nodes {
		children, _ := rec.AsMap(value)["children"].([]any)
		for _, child := range children {
			if name, isName := child.(string); isName {
				parents[name] = id
			}
		}
	}
	first = -1
	for entry := range entries {
		top, owner, found := entryOwner(entry, nodes, parents)
		if !found || !onlyEntries(top, nodes, entries) {
			return "", 0, 0, false
		}
		if parent != "" && owner != parent {
			return "", 0, 0, false
		}
		parent = owner
		index := childIndex(nodes, parent, top)
		if index < 0 {
			return "", 0, 0, false
		}
		if first < 0 || index < first {
			first = index
		}
		if index > last {
			last = index
		}
	}
	children, _ := rec.Node(nodes, parent)["children"].([]any)
	for index := first; index <= last; index++ {
		name, isName := children[index].(string)
		if !isName {
			return "", 0, 0, false
		}
		kind, _ := rec.Node(nodes, name)["type"].(string)
		if floatBlocks[kind] || onlyEntries(name, nodes, entries) {
			continue
		}
		return "", 0, 0, false
	}
	return parent, first, last, true
}

// entryOwner walks up from an entry block through its lists to the child of the section or of the
// document that holds it.
func entryOwner(entry string, nodes rec.Map, parents map[string]string) (top, owner string, ok bool) {
	child := entry
	for seen := 0; seen < 1000; seen++ {
		above, found := parents[child]
		if !found {
			return "", "", false
		}
		kind, _ := rec.Node(nodes, above)["type"].(string)
		switch {
		case kind == "section" || kind == "document":
			return child, above, true
		case entryContainers[kind]:
			child = above
		default:
			return "", "", false
		}
	}
	return "", "", false
}

// childIndex is the position of child in the parent's children, or -1.
func childIndex(nodes rec.Map, parent, child string) int {
	children, _ := rec.Node(nodes, parent)["children"].([]any)
	for index, value := range children {
		if name, isName := value.(string); isName && name == child {
			return index
		}
	}
	return -1
}

// BibliographyID is the wrapper's id in document: "bibliography", or "bibliography-N" when the
// document already uses that id (a node, the contents or main).
func BibliographyID(document rec.Map) string {
	nodes := rec.FetchMap(document, "nodes")
	id := "bibliography"
	for suffix := 2; bibliographyIDTaken(nodes, id); suffix++ {
		id = fmt.Sprintf("bibliography-%d", suffix)
	}
	return id
}

func bibliographyIDTaken(nodes rec.Map, id string) bool {
	if id == ContentsID || id == MainID {
		return true
	}
	_, taken := nodes[id]
	return taken
}
