// Package pagetree reads the pages relatex writes into a tree, and writes a tree back as HTML: the
// reading page for the EPUB package and the chapter files of a split book (decision 081).
package pagetree

import (
	"html"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Node is one piece of a page: an element (name set), or text (name empty). Text is held decoded
// and escaped again when the tree is written as XHTML.
type Node struct {
	Name     string
	Attrs    []Attribute
	Children []*Node
	Text     string
}

// Attribute is one attribute of an element, its value decoded.
type Attribute struct{ Name, Value string }

// voidElements are the HTML elements without content or end tag.
var voidElements = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}

// rawTextElements hold text up to their end tag, never markup.
var rawTextElements = map[string]bool{"script": true, "style": true}

// Attr is the value of the named attribute and whether the element has it.
func (n *Node) Attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// SetAttr gives the element the attribute, replacing its value when it has one.
func (n *Node) SetAttr(name, value string) {
	for index := range n.Attrs {
		if n.Attrs[index].Name == name {
			n.Attrs[index].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, Attribute{name, value})
}

// Walk visits the element and every element below it, in document order.
func (n *Node) Walk(visit func(*Node)) {
	if n.Name == "" {
		return
	}
	visit(n)
	for _, child := range n.Children {
		child.Walk(visit)
	}
}

// TextContent is the text of every descendant, MathML included.
func (n *Node) TextContent() string {
	if n.Name == "" {
		return n.Text
	}
	var builder strings.Builder
	for _, child := range n.Children {
		builder.WriteString(child.TextContent())
	}
	return builder.String()
}

// parser reads the HTML relatex writes: explicit end tags, quoted or bare attributes, void
// elements, raw text in style and script, character references. It is not a general HTML5
// parser; an end tag closes the open element it names (and any left open inside it), and an end
// tag naming no open element is ignored.
type parser struct {
	source string
	at     int
	stack  []*Node
}

// Parse is the page as a tree under a document node (name "#document").
func Parse(source string) *Node {
	p := &parser{source: source, stack: []*Node{{Name: "#document"}}}
	for p.at < len(p.source) {
		p.step()
	}
	return p.stack[0]
}

func (p *parser) top() *Node { return p.stack[len(p.stack)-1] }

func (p *parser) step() {
	rest := p.source[p.at:]
	switch {
	case strings.HasPrefix(rest, "<!--"):
		p.skipPast("-->")
	case strings.HasPrefix(rest, "<!"), strings.HasPrefix(rest, "<?"):
		p.skipPast(">")
	case strings.HasPrefix(rest, "</"):
		p.endTag()
	case len(rest) > 1 && rest[0] == '<' && isNameStart(rest[1]):
		p.startTag()
	default:
		p.textRun()
	}
}

func (p *parser) skipPast(marker string) {
	end := strings.Index(p.source[p.at:], marker)
	if end < 0 {
		p.at = len(p.source)
		return
	}
	p.at += end + len(marker)
}

func (p *parser) textRun() {
	end := strings.IndexByte(p.source[p.at+1:], '<')
	stop := len(p.source)
	if end >= 0 {
		stop = p.at + 1 + end
	}
	p.appendText(html.UnescapeString(p.source[p.at:stop]))
	p.at = stop
}

func (p *parser) appendText(text string) {
	parent := p.top()
	if count := len(parent.Children); count > 0 && parent.Children[count-1].Name == "" {
		parent.Children[count-1].Text += text
		return
	}
	parent.Children = append(parent.Children, &Node{Text: text})
}

func (p *parser) endTag() {
	p.at += 2
	name := p.readName()
	p.skipPast(">")
	for index := len(p.stack) - 1; index > 0; index-- {
		if p.stack[index].Name == name {
			p.stack = p.stack[:index]
			return
		}
	}
}

func (p *parser) startTag() {
	p.at++
	element := &Node{Name: p.readName()}
	selfClosing := p.readAttributes(element)
	parent := p.top()
	parent.Children = append(parent.Children, element)
	foreign := p.inForeign() || element.Name == "svg" || element.Name == "math"
	if selfClosing || (!foreign && voidElements[element.Name]) {
		return
	}
	if rawTextElements[element.Name] && !foreign {
		p.rawText(element)
		return
	}
	p.stack = append(p.stack, element)
}

// inForeign tells whether the open element is inside SVG or MathML, where names keep their case
// and a void HTML name is an ordinary element.
func (p *parser) inForeign() bool {
	for _, open := range p.stack {
		if open.Name == "svg" || open.Name == "math" {
			return true
		}
	}
	return false
}

func (p *parser) rawText(element *Node) {
	closing := "</" + element.Name
	end := strings.Index(strings.ToLower(p.source[p.at:]), closing)
	if end < 0 {
		rec.Fail("page: unterminated <%s> in the page", element.Name)
	}
	if end > 0 {
		element.Children = []*Node{{Text: p.source[p.at : p.at+end]}}
	}
	p.at += end
	p.endTag()
}

func isNameStart(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isNameChar(c byte) bool {
	return isNameStart(c) || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':' || c == '.'
}

func (p *parser) readName() string {
	start := p.at
	for p.at < len(p.source) && isNameChar(p.source[p.at]) {
		p.at++
	}
	return p.source[start:p.at]
}

func (p *parser) skipSpace() {
	for p.at < len(p.source) && strings.IndexByte(" \t\n\r\f", p.source[p.at]) >= 0 {
		p.at++
	}
}

// readAttributes reads up to the end of the start tag and tells whether it was self-closing.
func (p *parser) readAttributes(element *Node) bool {
	for {
		p.skipSpace()
		if p.at >= len(p.source) {
			rec.Fail("page: unterminated <%s> in the page", element.Name)
		}
		switch {
		case p.source[p.at] == '>':
			p.at++
			return false
		case strings.HasPrefix(p.source[p.at:], "/>"):
			p.at += 2
			return true
		}
		name := p.readName()
		if name == "" {
			rec.Fail("page: malformed attribute in <%s>", element.Name)
		}
		element.Attrs = append(element.Attrs, Attribute{name, p.readValue()})
	}
}

// readValue is an attribute's value: quoted, bare, or empty when the attribute has none.
func (p *parser) readValue() string {
	p.skipSpace()
	if p.at >= len(p.source) || p.source[p.at] != '=' {
		return ""
	}
	p.at++
	p.skipSpace()
	if p.at < len(p.source) && (p.source[p.at] == '"' || p.source[p.at] == '\'') {
		quote := p.source[p.at]
		end := strings.IndexByte(p.source[p.at+1:], quote)
		if end < 0 {
			rec.Fail("page: unterminated attribute value")
		}
		value := p.source[p.at+1 : p.at+1+end]
		p.at += end + 2
		return html.UnescapeString(value)
	}
	start := p.at
	for p.at < len(p.source) && strings.IndexByte(" \t\n\r\f>", p.source[p.at]) < 0 {
		p.at++
	}
	return html.UnescapeString(p.source[start:p.at])
}

// Void tells whether an HTML element has no content and no end tag.
func Void(name string) bool { return voidElements[name] }
