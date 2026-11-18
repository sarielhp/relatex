package split

import (
	"fmt"
	"strings"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/pagetree"
)

// Check proves a split faithful to the verified page it was cut from (decision 081). With the
// navigation and the previews left out and every link into another file read as a link to its
// id: every file has the page's head (its title aside) and what precedes main; the contents
// page then has what follows main, less each entry's nested list that lies in the entry's own
// file (a chapter's sections); the units' main elements, joined in order (a continued copy of
// an element merged into the element it continues), are the page's main; and every link into
// another file names an id that file holds. The contents page's skip link, which leads to the
// contents, reads as the page's link to main.
func Check(page string, files []File) error {
	whole, err := readPage(page)
	if err != nil {
		return err
	}
	if len(files) < 2 || files[0].Name != ContentsName {
		return fmt.Errorf("the split has no contents page followed by units")
	}
	pages := make([]*checkedPage, len(files))
	ids := map[string]map[string]bool{}
	for index, file := range files {
		if pages[index], err = readPage(file.HTML); err != nil {
			return fmt.Errorf("%s: %w", file.Name, err)
		}
		pages[index].strip()
		ids[file.Name] = pages[index].ids()
	}
	pageIDs := whole.ids()
	contents := shortened(whole.after, fileOwners(files, ids))
	var merged *pagetree.Node
	for index, file := range files {
		p := pages[index]
		if err := p.resolve(file.Name, ids, pageIDs); err != nil {
			return err
		}
		if index == 0 {
			// The contents page's skip link leads to the contents: it has no main (decision 088).
			retargetSkipLink(p.before, htmlkit.ContentsID, htmlkit.MainID)
		}
		if err := p.sameFrame(whole, index == 0, contents); err != nil {
			return fmt.Errorf("%s: %w", file.Name, err)
		}
		if index == 0 {
			continue
		}
		if merged == nil {
			merged = p.main
		} else if err := merge(merged, p.main); err != nil {
			return fmt.Errorf("%s: %w", file.Name, err)
		}
	}
	if pagetree.HTML(merged) != pagetree.HTML(whole.main) {
		return fmt.Errorf("the units' main elements joined in order differ from the page's main")
	}
	return nil
}

// checkedPage is a page read for the check: its head, its body, and the body's children before
// main, main itself (nil on the contents page) and after it.
type checkedPage struct {
	html, head, body *pagetree.Node
	before, after    []*pagetree.Node
	main             *pagetree.Node
}

func readPage(source string) (*checkedPage, error) {
	p := &checkedPage{}
	_, p.html = pagetree.Child(pagetree.Parse(source), "html")
	if p.html == nil {
		return nil, fmt.Errorf("no html element")
	}
	_, p.head = pagetree.Child(p.html, "head")
	_, p.body = pagetree.Child(p.html, "body")
	if p.head == nil || p.body == nil {
		return nil, fmt.Errorf("no head or body")
	}
	p.frame()
	return p, nil
}

func (p *checkedPage) frame() {
	index, main := pagetree.Child(p.body, "main")
	p.main = main
	if main == nil {
		p.before, p.after = p.body.Children, nil
		return
	}
	p.before, p.after = contentsToEnd(p.body.Children[:index], p.body.Children[index+1:])
}

// strip leaves out the navigation and the previews the split added.
func (p *checkedPage) strip() {
	var visit func(n *pagetree.Node)
	visit = func(n *pagetree.Node) {
		kept := n.Children[:0]
		for _, child := range n.Children {
			_, navigation := child.Attr(NavigationAttribute)
			_, previews := child.Attr(PreviewsAttribute)
			if navigation || previews && child.Name == "template" {
				continue
			}
			kept = append(kept, child)
			visit(child)
		}
		n.Children = kept
	}
	visit(p.html)
	p.frame()
}

func (p *checkedPage) ids() map[string]bool {
	ids := map[string]bool{}
	p.body.Walk(func(n *pagetree.Node) {
		if id, ok := n.Attr("id"); ok {
			ids[id] = true
		}
	})
	return ids
}

// resolve checks the file's links: one into another file names an id that file holds, and is
// then read as #id; one inside the file names an id the file holds whenever the page holds it.
func (p *checkedPage) resolve(name string, ids map[string]map[string]bool, pageIDs map[string]bool) error {
	var failure error
	p.body.Walk(func(n *pagetree.Node) {
		href, ok := n.Attr("href")
		if n.Name != "a" || !ok || failure != nil {
			return
		}
		if strings.HasPrefix(href, "#") {
			if pageIDs[href[1:]] && !ids[name][href[1:]] {
				failure = fmt.Errorf("%s: the link %s has no target in the file", name, href)
			}
			return
		}
		file, id, found := strings.Cut(href, "#")
		if _, split := ids[file]; !found || !split {
			return
		}
		if !ids[file][id] || !pageIDs[id] {
			failure = fmt.Errorf("%s: the link %s has no target", name, href)
			return
		}
		n.SetAttr("href", "#"+id)
	})
	return failure
}

// sameFrame compares the file's head and body around main with the page's: the same head but
// for the title, what precedes main, and on the contents page what follows it, shortened (outside).
func (p *checkedPage) sameFrame(whole *checkedPage, contents bool, outside []*pagetree.Node) error {
	if headWithoutTitle(p) != headWithoutTitle(whole) || attributes(p.html) != attributes(whole.html) {
		return fmt.Errorf("the head differs from the page's")
	}
	if attributes(p.body) != attributes(whole.body) {
		return fmt.Errorf("the body's attributes differ from the page's")
	}
	if contents {
		if p.main != nil || joined(p.before) != joined(whole.before)+joined(outside) {
			return fmt.Errorf("the contents page differs from the page outside main")
		}
		return nil
	}
	if p.main == nil || joined(p.before) != joined(whole.before) || joined(p.after) != "" {
		return fmt.Errorf("the body outside main differs from what precedes the page's main")
	}
	return nil
}

func headWithoutTitle(p *checkedPage) string {
	var builder strings.Builder
	for _, child := range p.head.Children {
		if child.Name != "title" {
			pagetree.WriteHTML(&builder, child)
		}
	}
	return builder.String()
}

func attributes(n *pagetree.Node) string {
	return pagetree.HTML(&pagetree.Node{Name: "x", Attrs: n.Attrs})
}

func joined(nodes []*pagetree.Node) string {
	var builder strings.Builder
	for _, n := range nodes {
		pagetree.WriteHTML(&builder, n)
	}
	return builder.String()
}

// merge appends the children of a later file's element to the joined element: a continued copy
// of an element is merged into the child of the joined element it continues, which must carry
// the same attributes but for its id.
func merge(joined, later *pagetree.Node) error {
	for _, child := range later.Children {
		id, continued := child.Attr(ContinuedAttribute)
		if !continued {
			joined.Children = append(joined.Children, child)
			continue
		}
		target := childWithID(joined, id)
		if target == nil {
			return fmt.Errorf("a continued <%s> names #%s, which is not open before it", child.Name, id)
		}
		if target.Name != child.Name || attributes(withoutAttribute(target, "id")) != attributes(withoutAttribute(child, ContinuedAttribute)) {
			return fmt.Errorf("the continued <%s> of #%s differs from the element it continues", child.Name, id)
		}
		if err := merge(target, child); err != nil {
			return err
		}
	}
	return nil
}

func childWithID(parent *pagetree.Node, id string) *pagetree.Node {
	for index := len(parent.Children) - 1; index >= 0; index-- {
		if value, ok := parent.Children[index].Attr("id"); ok && value == id {
			return parent.Children[index]
		}
	}
	return nil
}

func withoutAttribute(n *pagetree.Node, name string) *pagetree.Node {
	copied := &pagetree.Node{Name: n.Name}
	for _, a := range n.Attrs {
		if a.Name != name {
			copied.Attrs = append(copied.Attrs, a)
		}
	}
	return copied
}
