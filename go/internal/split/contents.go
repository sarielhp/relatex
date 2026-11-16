package split

import (
	"strings"

	"github.com/sarielhp/relatex/internal/pagetree"
)

// SectionsValue marks a unit's own list of its sections (the NavigationAttribute's value): the
// entries the contents page leaves out, shown at the top of the unit's file.
const SectionsValue = "sections"

// shortenContents leaves out of the contents lists below nodes (what follows main, its links
// still #id) every nested list whose links all land in the file of the entry that holds it: the
// contents page lists the files (the parts and the chapters), and a chapter's sections move to
// the chapter's own file. It returns the lists it left out by the file they belong to, in order.
func shortenContents(nodes []*pagetree.Node, owner map[string]string) map[string][]*pagetree.Node {
	removed := map[string][]*pagetree.Node{}
	for _, n := range nodes {
		n.Walk(func(nav *pagetree.Node) {
			if nav.Name == "nav" {
				shortenEntries(nav, owner, removed)
			}
		})
	}
	return removed
}

// shortenEntries shortens each list entry below n, outermost first; a list it removes is not
// searched further.
func shortenEntries(n *pagetree.Node, owner map[string]string, removed map[string][]*pagetree.Node) {
	if n.Name == "li" {
		if file := entryFile(n, owner); file != "" {
			kept := n.Children[:0]
			for _, child := range n.Children {
				if isList(child) && listWithin(child, file, owner) {
					removed[file] = append(removed[file], child)
					continue
				}
				kept = append(kept, child)
			}
			n.Children = kept
		}
	}
	for _, child := range n.Children {
		if child.Name != "" {
			shortenEntries(child, owner, removed)
		}
	}
}

// entryFile is the file of the entry's own link (its first child link), or "" when it has none.
func entryFile(entry *pagetree.Node, owner map[string]string) string {
	for _, child := range entry.Children {
		if child.Name != "a" {
			continue
		}
		href, _ := child.Attr("href")
		if !strings.HasPrefix(href, "#") {
			return ""
		}
		return owner[href[1:]]
	}
	return ""
}

func isList(n *pagetree.Node) bool { return n.Name == "ol" || n.Name == "ul" }

// listWithin tells whether the list has links and every link lands in the file.
func listWithin(list *pagetree.Node, file string, owner map[string]string) bool {
	links, inside := 0, true
	list.Walk(func(link *pagetree.Node) {
		href, ok := link.Attr("href")
		if link.Name != "a" || !ok {
			return
		}
		links++
		if !strings.HasPrefix(href, "#") || owner[href[1:]] != file {
			inside = false
		}
	})
	return links > 0 && inside
}

// sectionsNavigation is a unit's own list of its sections: the lists the contents page left out
// for the file, its links seen from the file. Check leaves it out like the other navigation.
func (b *book) sectionsNavigation(file string) *pagetree.Node {
	lists := b.sections[file]
	if len(lists) == 0 {
		return nil
	}
	nav := element("nav", []pagetree.Attribute{{Name: "class", Value: "split-sections"},
		{Name: NavigationAttribute, Value: SectionsValue}, {Name: "aria-label", Value: "Sections"}})
	for _, list := range lists {
		copied := pagetree.Clone(list)
		b.relink(copied, file)
		nav.Children = append(nav.Children, copied)
	}
	return nav
}

// fileOwners is the file of each id the units hold and the contents page does not (what precedes
// main is in every file): the first unit in reading order that holds it.
func fileOwners(files []File, ids map[string]map[string]bool) map[string]string {
	owner := map[string]string{}
	for _, file := range files[1:] {
		for id := range ids[file.Name] {
			if _, taken := owner[id]; !taken && !ids[ContentsName][id] {
				owner[id] = file.Name
			}
		}
	}
	return owner
}

// shortened is a copy of what follows the page's main as the contents page shows it.
func shortened(after []*pagetree.Node, owner map[string]string) []*pagetree.Node {
	copies := make([]*pagetree.Node, len(after))
	for index, n := range after {
		copies[index] = pagetree.Clone(n)
	}
	shortenContents(copies, owner)
	return copies
}
