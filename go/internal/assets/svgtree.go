package assets

import (
	"fmt"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"slices"
	"strings"
)

// rxNode is a node of REXML's tree: an element (name, raw attributes, children), or text,
// a comment, CDATA or a processing instruction (kind and raw text).
type rxNode struct {
	kind     string
	name     string
	attrs    [][2]string
	children []*rxNode
	text     string
}

type rxParser struct {
	source string
	at     int
}

// parseREXML reads a document's root element the way REXML::Document.new keeps it: raw
// text and raw attribute values, entities intact.
func parseREXML(source string) *rxNode {
	p := &rxParser{source: source}
	for {
		p.skipSpace()
		switch {
		case p.at >= len(p.source):
			rec.Fail("Invalid MathML: no root element")
		case strings.HasPrefix(p.source[p.at:], "<?"):
			p.until("?>")
		case strings.HasPrefix(p.source[p.at:], "<!--"):
			p.until("-->")
		case strings.HasPrefix(p.source[p.at:], "<!"):
			rec.Fail("SVG declarations/entities are forbidden")
		case p.source[p.at] == '<':
			p.at++
			return p.element()
		default:
			rec.Fail("Invalid SVG: content before the root element")
		}
	}
}

func (p *rxParser) skipSpace() {
	for p.at < len(p.source) && strings.IndexByte(" \t\r\n", p.source[p.at]) >= 0 {
		p.at++
	}
}

// until moves past the next marker and returns the text before it.
func (p *rxParser) until(marker string) string {
	index := strings.Index(p.source[p.at:], marker)
	if index < 0 {
		rec.Fail("Invalid SVG: unterminated %s", marker)
	}
	text := p.source[p.at : p.at+index]
	p.at += index + len(marker)
	return text
}

func (p *rxParser) name() string {
	start := p.at
	for p.at < len(p.source) && strings.IndexByte(" \t\r\n/>=", p.source[p.at]) < 0 {
		p.at++
	}
	if p.at == start {
		rec.Fail("Invalid SVG: missing name")
	}
	return p.source[start:p.at]
}

func (p *rxParser) element() *rxNode {
	node := &rxNode{kind: "element", name: p.name()}
	for {
		p.skipSpace()
		if strings.HasPrefix(p.source[p.at:], "/>") {
			p.at += 2
			return node
		}
		if strings.HasPrefix(p.source[p.at:], ">") {
			p.at++
			break
		}
		name := p.name()
		p.skipSpace()
		if p.at >= len(p.source) || p.source[p.at] != '=' {
			rec.Fail("Invalid SVG: attribute without value")
		}
		p.at++
		p.skipSpace()
		if p.at >= len(p.source) || (p.source[p.at] != '"' && p.source[p.at] != '\'') {
			rec.Fail("Invalid SVG: unquoted attribute")
		}
		quote := string(p.source[p.at])
		p.at++
		node.attrs = append(node.attrs, [2]string{name, p.until(quote)})
	}
	p.content(node)
	return node
}

func (p *rxParser) content(node *rxNode) {
	for p.at < len(p.source) {
		rest := p.source[p.at:]
		switch {
		case strings.HasPrefix(rest, "</"):
			p.at += 2
			p.name()
			p.until(">")
			return
		case strings.HasPrefix(rest, "<!--"):
			p.at += 4
			node.children = append(node.children, &rxNode{kind: "comment", text: p.until("-->")})
		case strings.HasPrefix(rest, "<![CDATA["):
			p.at += 9
			node.children = append(node.children, &rxNode{kind: "cdata", text: p.until("]]>")})
		case strings.HasPrefix(rest, "<?"):
			p.at += 2
			node.children = append(node.children, &rxNode{kind: "instruction", text: p.until("?>")})
		case rest[0] == '<':
			p.at++
			node.children = append(node.children, p.element())
		default:
			p.text(node)
		}
	}
	rec.Fail("Invalid SVG: unclosed element %s", node.name)
}

func (p *rxParser) text(node *rxNode) {
	end := strings.IndexByte(p.source[p.at:], '<')
	if end < 0 {
		end = len(p.source) - p.at
	}
	text := strings.ReplaceAll(strings.ReplaceAll(p.source[p.at:p.at+end], "\r\n", "\n"), "\r", "\n")
	p.at += end
	if count := len(node.children); count > 0 && node.children[count-1].kind == "text" {
		node.children[count-1].text += text
		return
	}
	node.children = append(node.children, &rxNode{kind: "text", text: text})
}

// attribute is REXML's attributes.fetch(name).to_s: the raw value; ok is false without one.
func (n *rxNode) attribute(name string) (string, bool) {
	for _, attr := range n.attrs {
		if attr[0] == name {
			return attr[1], true
		}
	}
	return "", false
}

// setAttribute is add_attribute(name, value): the value is kept as given.
func (n *rxNode) setAttribute(name, value string) {
	for index, attr := range n.attrs {
		if attr[0] == name {
			n.attrs[index][1] = value
			return
		}
	}
	n.attrs = append(n.attrs, [2]string{name, value})
}

func localName(name string) string {
	if index := strings.LastIndexByte(name, ':'); index >= 0 {
		return name[index+1:]
	}
	return name
}

// write is REXML::Formatters::Default: attributes sorted by local name, single quotes,
// childless elements closed with "/>".
func (n *rxNode) write(out *strings.Builder) {
	switch n.kind {
	case "text":
		out.WriteString(n.text)
		return
	case "comment":
		out.WriteString("<!--" + n.text + "-->")
		return
	case "cdata":
		out.WriteString("<![CDATA[" + n.text + "]]>")
		return
	case "instruction":
		out.WriteString("<?" + n.text + "?>")
		return
	}
	out.WriteString("<" + n.name)
	attrs := slices.Clone(n.attrs)
	slices.SortStableFunc(attrs, func(a, b [2]string) int { return strings.Compare(localName(a[0]), localName(b[0])) })
	for _, attr := range attrs {
		out.WriteString(" " + attr[0] + "='" + strings.ReplaceAll(attr[1], "'", "&apos;") + "'")
	}
	if len(n.children) == 0 {
		out.WriteString("/>")
		return
	}
	out.WriteString(">")
	for _, child := range n.children {
		child.write(out)
	}
	out.WriteString("</" + n.name + ">")
}

// rubyString is Ruby's to_s of a JSON number (Integer or Float#to_s).
func rubyString(value any) string {
	switch typed := value.(type) {
	case int64:
		return fmt.Sprint(typed)
	case canonjson.Integer:
		return string(typed)
	}
	number, ok := rec.Number(value)
	if !ok {
		return fmt.Sprint(value)
	}
	text, err := canonjson.Marshal(number)
	if err != nil {
		rec.Fail("%v", err)
	}
	return string(text)
}
