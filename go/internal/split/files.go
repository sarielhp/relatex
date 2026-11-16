package split

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/pagetree"
	"github.com/sarielhp/relatex/internal/rec"
)

// NavigationAttribute marks what the split adds to a file for reading (the previous, contents
// and next links, their style, the contents page's title): Check leaves it out.
const NavigationAttribute = "data-split-navigation"

// PreviewsAttribute marks the template that holds a file's copies of its links' targets in other
// files, for the reference previews (preview.js): Check leaves it out.
const PreviewsAttribute = "data-split-previews"

// PreviewTargetAttribute names the id whose copy a preview entry holds.
const PreviewTargetAttribute = "data-preview-for"

// File is one file of a split book: its name, its title and its HTML.
type File struct {
	Name  string
	Title string
	Kind  string
	HTML  string
}

// navigationStyle lays out the previous, contents and next links.
const navigationStyle = `.split-nav { display: flex; flex-wrap: wrap; gap: .5rem 1.5rem; justify-content: space-between; }
.split-nav[data-split-navigation="top"] { border-top: 0; margin: 0 0 1rem; padding-top: 0; }`

// idReferenceAttributes hold the ids of elements in the same document: a split that puts the
// element and its target in different files would break them.
var idReferenceAttributes = map[string]bool{"aria-labelledby": true, "aria-describedby": true, "aria-controls": true,
	"aria-owns": true, "aria-flowto": true, "aria-activedescendant": true, "aria-details": true, "aria-errormessage": true,
	"headers": true, "for": true, "form": true, "list": true}

var localURL = regexp.MustCompile(`url\(\s*['"]?#([^'")\s]+)`)

// LocalTargets are the ids an element refers to within its own document, links aside: these
// cannot cross files.
func LocalTargets(n *pagetree.Node) []string {
	targets := []string{}
	for _, a := range n.Attrs {
		if idReferenceAttributes[a.Name] {
			targets = append(targets, strings.Fields(a.Value)...)
		}
		for _, match := range localURL.FindAllStringSubmatch(a.Value, -1) {
			targets = append(targets, match[1])
		}
		if (a.Name == "xlink:href" || a.Name == "href" && n.Name != "a") && strings.HasPrefix(a.Value, "#") {
			targets = append(targets, a.Value[1:])
		}
	}
	return targets
}

// book is the page being split: its tree, the division, the file of each id, and each element
// with an id and its parent (for the previews).
type book struct {
	root     *pagetree.Node
	html     *pagetree.Node
	head     *pagetree.Node
	body     *pagetree.Node
	division *Division
	owner    map[string]string
	elements map[string]*pagetree.Node
	parents  map[*pagetree.Node]*pagetree.Node
	sections map[string][]*pagetree.Node
	title    string
}

// Write is the split of the verified page: the contents page and the units' files in reading
// order, or nil and the reason the page stays whole.
func Write(document rec.Map, page string) ([]File, string) {
	b, reason := newBook(document, page)
	if b == nil {
		return nil, reason
	}
	units := b.division.Units
	files := make([]File, 0, len(units)+1)
	files = append(files, File{Name: ContentsName, Title: "Contents", Kind: "contents", HTML: b.contentsPage(units[0])})
	for index, unit := range units {
		previous := Unit{Name: ContentsName, Title: "Contents"}
		if index > 0 {
			previous = units[index-1]
		}
		var next *Unit
		if index+1 < len(units) {
			next = &units[index+1]
		}
		files = append(files, File{Name: unit.Name, Title: unit.Title, Kind: unit.Kind, HTML: b.unitPage(unit, &previous, next)})
	}
	return files, ""
}

func newBook(document rec.Map, page string) (*book, string) {
	b := &book{root: pagetree.Parse(page), owner: map[string]string{}, elements: map[string]*pagetree.Node{},
		parents: map[*pagetree.Node]*pagetree.Node{}}
	_, b.html = pagetree.Child(b.root, "html")
	if b.html == nil {
		return nil, "the page has no html element"
	}
	_, b.head = pagetree.Child(b.html, "head")
	_, b.body = pagetree.Child(b.html, "body")
	body := b.body
	if b.head == nil || body == nil {
		return nil, "the page has no head or body"
	}
	if _, title := pagetree.Child(b.head, "title"); title != nil {
		b.title = strings.Join(strings.Fields(title.TextContent()), " ")
	}
	division, reason := Divide(document, body)
	if division == nil {
		return nil, reason
	}
	b.division = division
	b.index(body)
	if reason := b.crossReference(); reason != "" {
		return nil, reason
	}
	return b, ""
}

// index records each id's element, its parent, and the file that shows it: a unit's file, the
// contents page for what follows main; what precedes main is in every file.
func (b *book) index(body *pagetree.Node) {
	var visit func(parent, n *pagetree.Node)
	visit = func(parent, n *pagetree.Node) {
		b.parents[n] = parent
		if id, ok := n.Attr("id"); ok {
			if _, seen := b.elements[id]; !seen {
				b.elements[id] = n
			}
		}
		for _, child := range n.Children {
			if child.Name != "" {
				visit(n, child)
			}
		}
	}
	visit(nil, body)
	// Every unit's file has a main element: its id (the skip link's target, decision 088) is
	// in each file, owned by none.
	for _, unit := range b.division.Units {
		for _, child := range unit.Main.Children {
			if child.Name != "" {
				b.own(child, unit.Name)
			}
		}
	}
	for _, n := range b.division.After {
		b.own(n, ContentsName)
	}
}

func (b *book) own(n *pagetree.Node, name string) {
	n.Walk(func(element *pagetree.Node) {
		if id, ok := element.Attr("id"); ok {
			if _, seen := b.owner[id]; !seen {
				b.owner[id] = name
			}
		}
	})
}

// crossReference names a same-document reference (not a link) whose target lies in another file.
func (b *book) crossReference() string {
	broken := ""
	check := func(n *pagetree.Node, name string) {
		n.Walk(func(element *pagetree.Node) {
			for _, target := range LocalTargets(element) {
				if at, known := b.owner[target]; known && at != name && broken == "" {
					broken = fmt.Sprintf("<%s> refers to #%s in another file", element.Name, target)
				}
			}
		})
	}
	for _, unit := range b.division.Units {
		check(unit.Main, unit.Name)
	}
	for _, n := range b.division.After {
		check(n, ContentsName)
	}
	return broken
}

// relink points the links below n at their targets as seen from the file: a target in another
// file by that file's name, one in the same file (or in every file) by its id alone.
func (b *book) relink(n *pagetree.Node, file string) {
	n.Walk(func(element *pagetree.Node) {
		href, ok := element.Attr("href")
		if element.Name != "a" || !ok {
			return
		}
		id, inside := b.linkTarget(href)
		if !inside {
			return
		}
		if at, known := b.owner[id]; known && at != file {
			element.SetAttr("href", at+"#"+id)
		} else {
			element.SetAttr("href", "#"+id)
		}
	})
}

// linkTarget is the id a link names inside the book (#id, or FILE#id after a relink).
func (b *book) linkTarget(href string) (string, bool) {
	if strings.HasPrefix(href, "#") {
		return href[1:], true
	}
	file, id, found := strings.Cut(href, "#")
	if !found || !strings.HasSuffix(file, ".html") || strings.Contains(file, "/") {
		return "", false
	}
	if owner, known := b.owner[id]; known && owner == file {
		return id, true
	}
	return "", false
}

// page is a file's HTML: the page's head with the file's title and the navigation style, and
// the body.
func (b *book) page(title string, body *pagetree.Node) string {
	head := &pagetree.Node{Name: "head", Attrs: b.head.Attrs}
	for _, child := range b.head.Children {
		if child.Name == "title" {
			child = &pagetree.Node{Name: "title", Children: []*pagetree.Node{{Text: title}}}
		}
		head.Children = append(head.Children, child)
	}
	head.Children = append(head.Children, &pagetree.Node{Name: "style",
		Attrs: []pagetree.Attribute{{Name: NavigationAttribute, Value: "style"}}, Children: []*pagetree.Node{{Text: navigationStyle}}})
	document := &pagetree.Node{Name: "html", Attrs: b.html.Attrs}
	for _, child := range b.html.Children {
		switch child.Name {
		case "head":
			child = head
		case "body":
			child = body
		}
		document.Children = append(document.Children, child)
	}
	var builder strings.Builder
	builder.WriteString("<!DOCTYPE html>\n")
	pagetree.WriteHTML(&builder, document)
	builder.WriteString("\n")
	return builder.String()
}

// fileTitle is a file's title: its own, then the book's.
func (b *book) fileTitle(own string) string {
	if b.title == "" {
		return own
	}
	return own + " — " + b.title
}

// before is a copy of what precedes main, with its links seen from the file.
func (b *book) before(file string) []*pagetree.Node {
	copies := make([]*pagetree.Node, len(b.division.Before))
	for index, n := range b.division.Before {
		copies[index] = pagetree.Clone(n)
		b.relink(copies[index], file)
	}
	return copies
}

// contentsPage is index.html: what precedes main, the book's title, the link to the first unit,
// and the page's contents with links into the files, shortened to the files (shortenContents).
func (b *book) contentsPage(first Unit) string {
	body := &pagetree.Node{Name: "body", Attrs: b.body.Attrs, Children: b.before(ContentsName)}
	retargetSkipLink(body.Children, htmlkit.MainID, htmlkit.ContentsID)
	// The book's title opens the page's banner (decision 095): one header, so one banner landmark.
	title := element("h1", []pagetree.Attribute{{Name: NavigationAttribute, Value: "title"}}, text(b.title))
	if banner := pageBanner(body.Children); banner != nil {
		banner.Children = append([]*pagetree.Node{title}, banner.Children...)
	} else {
		body.Children = append(body.Children, element("header", []pagetree.Attribute{{Name: NavigationAttribute, Value: "title"}}, title))
	}
	body.Children = append(body.Children, navigation(nil, nil, &first, "top"))
	b.sections = shortenContents(b.division.After, b.owner)
	for _, n := range b.division.After {
		b.relink(n, ContentsName)
		body.Children = append(body.Children, n)
	}
	return b.page(b.fileTitle("Contents"), body)
}

// pageBanner is the page's header among the body's children (html.pageHeader), or nil for a page
// from before the banner.
func pageBanner(nodes []*pagetree.Node) *pagetree.Node {
	for _, n := range nodes {
		if n.Name == "header" {
			return n
		}
	}
	return nil
}

// retargetSkipLink points the skip link among nodes from one id to another: the contents page
// has no main element, so its skip link leads to the contents (decision 088).
func retargetSkipLink(nodes []*pagetree.Node, from, target string) {
	for _, n := range nodes {
		n.Walk(func(element *pagetree.Node) {
			class, _ := element.Attr("class")
			href, _ := element.Attr("href")
			if element.Name == "a" && class == skipLinkClass && href == "#"+from {
				element.SetAttr("href", "#"+target)
			}
		})
	}
}

// skipLinkClass is the class of the page's skip link (html.SkipLink).
const skipLinkClass = "skip-link"

// unitPage is a unit's file: what precedes main, the navigation, its sections, the unit's main, the navigation
// again, and the previews of its links into other files.
func (b *book) unitPage(unit Unit, previous, next *Unit) string {
	b.relink(unit.Main, unit.Name)
	body := &pagetree.Node{Name: "body", Attrs: b.body.Attrs, Children: b.before(unit.Name)}
	contents := &Unit{Name: ContentsName, Title: "Contents"}
	body.Children = append(body.Children, navigation(previous, contents, next, "top"))
	if sections := b.sectionsNavigation(unit.Name); sections != nil {
		body.Children = append(body.Children, sections)
	}
	body.Children = append(body.Children, unit.Main, navigation(previous, contents, next, "bottom"))
	if previews := b.previews(unit); previews != nil {
		body.Children = append(body.Children, previews)
	}
	return b.page(b.fileTitle(unit.Title), body)
}

// navigation is the previous, contents and next links of a file.
func navigation(previous, contents, next *Unit, position string) *pagetree.Node {
	label := "Chapter navigation"
	if position == "bottom" {
		label = "Chapter navigation, end of page"
	}
	nav := element("nav", []pagetree.Attribute{{Name: "class", Value: "split-nav"}, {Name: NavigationAttribute, Value: position},
		{Name: "aria-label", Value: label}})
	add := func(unit *Unit, rel, prefix string) {
		if unit == nil {
			return
		}
		attrs := []pagetree.Attribute{{Name: "href", Value: unit.Name}}
		if rel != "" {
			attrs = append(attrs, pagetree.Attribute{Name: "rel", Value: rel})
		}
		nav.Children = append(nav.Children, element("a", attrs, text(prefix+unit.Title)))
	}
	add(previous, "prev", "Previous: ")
	add(contents, "", "")
	add(next, "next", "Next: ")
	return nav
}

// previews is the template of copies of the targets in other files of the unit's previewed
// links (main's links, back references from notes aside), or nil when it has none. A heading's
// copy also holds the two elements after it, as the previews of the page show them.
func (b *book) previews(unit Unit) *pagetree.Node {
	template := element("template", []pagetree.Attribute{{Name: PreviewsAttribute, Value: ""}})
	seen := map[string]bool{}
	unit.Main.Walk(func(link *pagetree.Node) {
		href, ok := link.Attr("href")
		if _, back := link.Attr("data-note-backref"); link.Name != "a" || !ok || back || strings.HasPrefix(href, "#") {
			return
		}
		id, inside := b.linkTarget(href)
		if !inside || seen[id] {
			return
		}
		seen[id] = true
		entry := element("div", []pagetree.Attribute{{Name: PreviewTargetAttribute, Value: id}})
		for _, target := range b.previewNodes(b.elements[id]) {
			copied := pagetree.Clone(target)
			stripIDs(copied)
			b.relink(copied, unit.Name)
			entry.Children = append(entry.Children, copied)
		}
		template.Children = append(template.Children, entry)
	})
	if len(template.Children) == 0 {
		return nil
	}
	return template
}

// previewNodes are the target, and for a heading the two elements after it up to the next heading.
func (b *book) previewNodes(target *pagetree.Node) []*pagetree.Node {
	nodes := []*pagetree.Node{target}
	if !isHeading(target) || b.parents[target] == nil {
		return nodes
	}
	following := false
	for _, sibling := range b.parents[target].Children {
		switch {
		case sibling == target:
			following = true
		case !following || sibling.Name == "":
		case isHeading(sibling) || len(nodes) == 3:
			return nodes
		default:
			nodes = append(nodes, sibling)
		}
	}
	return nodes
}

func isHeading(n *pagetree.Node) bool {
	return len(n.Name) == 2 && n.Name[0] == 'h' && n.Name[1] >= '1' && n.Name[1] <= '6'
}

func stripIDs(n *pagetree.Node) {
	n.Walk(func(element *pagetree.Node) {
		kept := element.Attrs[:0]
		for _, a := range element.Attrs {
			if a.Name != "id" {
				kept = append(kept, a)
			}
		}
		element.Attrs = kept
	})
}

func element(name string, attrs []pagetree.Attribute, children ...*pagetree.Node) *pagetree.Node {
	return &pagetree.Node{Name: name, Attrs: attrs, Children: children}
}

func text(value string) *pagetree.Node { return &pagetree.Node{Text: value} }
