package html

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// Heading levels, the title and the skip link (decision 088).

// SkipLink is the body's first element: a link to the main content, shown when it has the focus.
const SkipLink = `<a class="skip-link" href="#` + htmlkit.MainID + `">Skip to main content</a>`

// titleMarks are the marks a title may end with (a \thanks call) that the front matter's title
// leaves out.
const titleMarks = "∗*†‡§¶"

// headingTag is a heading's element: h(1 + the number of sections around it), from h2 for a
// top-level unit (a paper's \section, a book's \part or \chapter) to h6, so the levels follow
// the document's nesting and never skip; the title is the page's h1.
func (p *Paper) headingTag(identifier string) string {
	if p.headingDepths == nil {
		p.headingDepths = map[string]int{}
		p.nestHeadings(rec.FetchString(p.document, "root"), 0)
	}
	level := min(max(1+p.headingDepths[identifier], 2), 6)
	return "h" + strconv.Itoa(level)
}

// headingName is the accessible name a heading without text needs (axe and Ace's empty-heading):
// a heading of formulas only is named by the text its formulas show (as its contents entry is),
// and a heading of rules only (titlesec's rule after a title, a box before an appendix's title)
// is decoration, hidden from assistive technology. A heading with text needs nothing.
func headingName(node rec.Map, content string) string {
	decoration := true
	for _, value := range rec.FetchList(node, "content") {
		span := rec.AsMap(value)
		if text, _ := span["text"].(string); strings.TrimSpace(text) != "" {
			return ""
		}
		decoration = decoration && (span["type"] == "rule" || span["type"] == "text")
	}
	if name := visibleText(content); name != "" {
		return " aria-label=\"" + htmlkit.Escape(name) + "\""
	}
	if decoration {
		return " aria-hidden=\"true\""
	}
	return ""
}

// nestHeadings records the depth of each heading below a node: the sections that hold it.
func (p *Paper) nestHeadings(identifier string, depth int) {
	node := p.node(identifier)
	switch node["type"] {
	case "heading":
		p.headingDepths[identifier] = depth
		return
	case "paragraph":
		if RuninSpans(node) > 0 {
			p.headingDepths[identifier] = depth
		}
		return
	case "section":
		depth++
	}
	children, _ := node["children"].([]any)
	for _, child := range children {
		if id, ok := child.(string); ok {
			p.nestHeadings(id, depth)
		}
	}
}

// titleBlock is the id of the paragraph that shows the document's title: the first paragraph
// before the first heading of the main text whose text is the front matter's title (a trailing
// \thanks mark and letter case aside, as amsart sets the title in upper case), or "" when no
// paragraph is.
func (p *Paper) titleBlock() string {
	if p.title != nil {
		return *p.title
	}
	found := ""
	p.title = &found
	front, _ := p.document["frontmatter"].(rec.Map)
	title, _ := front["title"].(string)
	title = TitleText(title)
	if title == "" {
		return found
	}
	order, _ := p.document["main_block_order"].([]any)
	for _, value := range order {
		id, _ := value.(string)
		node := p.node(id)
		if node["type"] == "heading" {
			break
		}
		if node["type"] == "paragraph" && strings.EqualFold(TitleText(paragraphText(node)), title) {
			found = id
			break
		}
	}
	return found
}

// paragraphText is the text of a paragraph's text spans.
func paragraphText(node rec.Map) string {
	var builder strings.Builder
	content, _ := node["content"].([]any)
	for _, value := range content {
		span := rec.AsMap(value)
		if text, ok := span["text"].(string); ok && span["type"] == "text" {
			builder.WriteString(text)
		}
	}
	return builder.String()
}

// TitleText is a title compared as text: white space runs as one space, trimmed, without the
// marks it ends with.
func TitleText(text string) string {
	return strings.TrimRight(strings.Join(strings.FieldsFunc(text, unicode.IsSpace), " "), titleMarks+" ")
}
