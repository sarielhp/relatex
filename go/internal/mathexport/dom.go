// Package mathexport is the Go port of relatex's math exporters: MathExport with its TeX
// and Typst dialects, MathPresentation, MathLength, MathTablePresentation, MathAlphabet,
// MathProjectionSupport, BrowserMathML, MathAccentFont and MarkdownAuthorMath's macros.
// MathML is read into a tree that keeps what REXML keeps (raw text and attribute values,
// entities as written), so a projected formula serializes byte for byte as Ruby writes it.
package mathexport

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/mathml"
	"github.com/sarielhp/relatex/internal/rec"
)

// Attr is an attribute as REXML holds it: its expanded name and its raw (escaped) value.
type Attr struct {
	Prefix, Name, Raw string
}

// Expanded is the attribute's expanded name.
func (a *Attr) Expanded() string {
	if a.Prefix == "" {
		return a.Name
	}
	return a.Prefix + ":" + a.Name
}

// Value is the unescaped value (REXML's Attribute#value).
func (a *Attr) Value() string { return unescape(a.Raw) }

// Node is an element, or (Text set) a text node holding its raw text.
type Node struct {
	Prefix, Name string
	Attrs        []*Attr
	Children     []*Node
	Parent       *Node
	Text         bool
	Raw          string
}

// ParsePaper is PaperMathML.parse!(source).root: validated, then read as REXML reads it.
func ParsePaper(source string) *Node {
	mathml.ParsePaper(source)
	return parseRaw(source)
}

// Parse is MathML.parse!(source).root.
func Parse(source string) *Node {
	mathml.Parse(source)
	return parseRaw(source)
}

// Expanded is the element's expanded name.
func (n *Node) Expanded() string {
	if n.Prefix == "" {
		return n.Name
	}
	return n.Prefix + ":" + n.Name
}

// Attribute is attributes[name]: the unescaped value of an unprefixed attribute.
func (n *Node) Attribute(name string) (string, bool) {
	for _, attribute := range n.Attrs {
		if attribute.Name == name && attribute.Prefix == "" {
			return attribute.Value(), true
		}
	}
	return "", false
}

// Has reports whether the attribute is present (Ruby's truthy attributes[name]).
func (n *Node) Has(name string) bool {
	_, ok := n.Attribute(name)
	return ok
}

// Get is attributes[name] with "" for a missing attribute.
func (n *Node) Get(name string) string {
	value, _ := n.Attribute(name)
	return value
}

// SetAttribute is add_attribute(name, value): the value is escaped as REXML escapes it.
func (n *Node) SetAttribute(name, value string) {
	raw := escapeAttribute(value)
	for _, attribute := range n.Attrs {
		if attribute.Name == name && attribute.Prefix == "" {
			attribute.Raw = raw
			return
		}
	}
	n.Attrs = append(n.Attrs, &Attr{Name: name, Raw: raw})
}

// RemoveAttribute is delete_attribute(name) for an unprefixed attribute.
func (n *Node) RemoveAttribute(name string) {
	n.Attrs = slices.DeleteFunc(n.Attrs, func(attribute *Attr) bool { return attribute.Name == name && attribute.Prefix == "" })
}

// Elements are the element children (REXML's elements).
func (n *Node) Elements() []*Node {
	result := []*Node{}
	for _, child := range n.Children {
		if !child.Text {
			result = append(result, child)
		}
	}
	return result
}

// Element is elements[index] (from 1), or nil.
func (n *Node) Element(index int) *Node {
	elements := n.Elements()
	if index < 1 || index > len(elements) {
		return nil
	}
	return elements[index-1]
}

// Texts are the values of the text children.
func (n *Node) Texts() []string {
	result := []string{}
	for _, child := range n.Children {
		if child.Text {
			result = append(result, unescape(child.Raw))
		}
	}
	return result
}

// TextValue is node.text: the first text child's value; ok is false for Ruby's nil.
func (n *Node) TextValue() (string, bool) {
	for _, child := range n.Children {
		if child.Text {
			return unescape(child.Raw), true
		}
	}
	return "", false
}

// SetText is node.text = value: the first text child is replaced, or a text child added.
func (n *Node) SetText(value string) {
	raw := escapeText(value)
	for _, child := range n.Children {
		if child.Text {
			child.Raw = raw
			return
		}
	}
	n.Children = append(n.Children, &Node{Text: true, Raw: raw, Parent: n})
}

// Descendants are the descendant elements in document order (XPath .//*).
func (n *Node) Descendants() []*Node {
	result := []*Node{}
	for _, child := range n.Elements() {
		result = append(result, child)
		result = append(result, child.Descendants()...)
	}
	return result
}

// Path is elements[path] for an absolute path of named steps with optional positions
// (/math/mtable/mtr[2]): the first match in document order, or nil.
func (n *Node) Path(path string) *Node {
	steps := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(steps) == 0 || stepName(steps[0]) != n.Name {
		return nil
	}
	return find([]*Node{n}, steps[1:])
}

func stepName(step string) string {
	name, _, _ := strings.Cut(step, "[")
	return name
}

func stepIndex(step string) int {
	_, rest, ok := strings.Cut(step, "[")
	if !ok {
		return 0
	}
	index, err := strconv.Atoi(strings.TrimSuffix(rest, "]"))
	if err != nil {
		return -1
	}
	return index
}

func find(nodes []*Node, steps []string) *Node {
	if len(steps) == 0 {
		if len(nodes) == 0 {
			return nil
		}
		return nodes[0]
	}
	name, index := stepName(steps[0]), stepIndex(steps[0])
	for _, node := range nodes {
		matches := []*Node{}
		for _, child := range node.Elements() {
			if child.Name == name {
				matches = append(matches, child)
			}
		}
		if index > 0 {
			if index > len(matches) {
				continue
			}
			matches = matches[index-1 : index]
		}
		if found := find(matches, steps[1:]); found != nil {
			return found
		}
	}
	return nil
}

// Clone is deep_clone.
func (n *Node) Clone() *Node {
	copied := &Node{Prefix: n.Prefix, Name: n.Name, Text: n.Text, Raw: n.Raw}
	for _, attribute := range n.Attrs {
		value := *attribute
		copied.Attrs = append(copied.Attrs, &value)
	}
	for _, child := range n.Children {
		clone := child.Clone()
		clone.Parent = copied
		copied.Children = append(copied.Children, clone)
	}
	return copied
}

// String is REXML's to_s (Formatters::Default): attributes sorted by name, single quotes,
// an element without children closed with "/>".
func (n *Node) String() string {
	var builder strings.Builder
	n.write(&builder)
	return builder.String()
}

func (n *Node) write(out *strings.Builder) {
	if n.Text {
		out.WriteString(n.Raw)
		return
	}
	out.WriteString("<" + n.Expanded())
	attributes := slices.Clone(n.Attrs)
	slices.SortStableFunc(attributes, func(a, b *Attr) int { return strings.Compare(a.Name, b.Name) })
	for _, attribute := range attributes {
		out.WriteString(" " + attribute.Expanded() + "='" + strings.ReplaceAll(attribute.Raw, "'", "&apos;") + "'")
	}
	if len(n.Children) == 0 {
		out.WriteString("/>")
		return
	}
	out.WriteString(">")
	for _, child := range n.Children {
		child.write(out)
	}
	out.WriteString("</" + n.Expanded() + ">")
}

var (
	textEscapes      = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	attributeEscapes = textEscapes
)

// escapeText is REXML's Text normalization of a new value: carriage returns become line
// feeds, then the five special characters are escaped.
func escapeText(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	return textEscapes.Replace(value)
}

func escapeAttribute(value string) string { return attributeEscapes.Replace(value) }

// unescape is REXML's unnormalize: the predefined entities and character references.
func unescape(raw string) string {
	if !strings.Contains(raw, "&") {
		return raw
	}
	var builder strings.Builder
	for len(raw) > 0 {
		at := strings.IndexByte(raw, '&')
		if at < 0 {
			builder.WriteString(raw)
			break
		}
		builder.WriteString(raw[:at])
		raw = raw[at:]
		end := strings.IndexByte(raw, ';')
		if end < 0 {
			builder.WriteString(raw)
			break
		}
		if decoded, ok := entity(raw[1:end]); ok {
			builder.WriteString(decoded)
		} else {
			builder.WriteString(raw[:end+1])
		}
		raw = raw[end+1:]
	}
	return builder.String()
}

func entity(name string) (string, bool) {
	switch name {
	case "amp":
		return "&", true
	case "lt":
		return "<", true
	case "gt":
		return ">", true
	case "quot":
		return `"`, true
	case "apos":
		return "'", true
	}
	if !strings.HasPrefix(name, "#") {
		return "", false
	}
	base, digits := 10, name[1:]
	if strings.HasPrefix(digits, "x") {
		base, digits = 16, digits[1:]
	}
	code, err := strconv.ParseInt(digits, base, 32)
	if err != nil || !utf8.ValidRune(rune(code)) {
		return "", false
	}
	return string(rune(code)), true
}

// rawParser reads validated MathML keeping raw text and attribute values. The source has
// no declarations, comments, CDATA or processing instructions (MathML.parse! forbids them).
type rawParser struct {
	source string
	at     int
}

func parseRaw(source string) *Node {
	p := &rawParser{source: source}
	for p.at < len(p.source) && p.source[p.at] != '<' {
		p.at++
	}
	if p.at >= len(p.source) {
		rec.Fail("Expected a MathML root")
	}
	return p.element(nil)
}

func (p *rawParser) element(parent *Node) *Node {
	p.at++ // '<'
	name := p.name()
	node := &Node{Parent: parent}
	node.Prefix, node.Name = split(name)
	for {
		p.space()
		switch p.source[p.at] {
		case '/':
			p.at += 2 // "/>"
			return node
		case '>':
			p.at++
			p.content(node)
			return node
		}
		attribute := p.name()
		p.space()
		p.at++ // '='
		p.space()
		quote := p.source[p.at]
		end := strings.IndexByte(p.source[p.at+1:], quote)
		raw := p.source[p.at+1 : p.at+1+end]
		p.at += end + 2
		prefix, local := split(attribute)
		node.Attrs = append(node.Attrs, &Attr{Prefix: prefix, Name: local, Raw: raw})
	}
}

func (p *rawParser) content(node *Node) {
	for p.at < len(p.source) {
		if p.source[p.at] != '<' {
			end := strings.IndexByte(p.source[p.at:], '<')
			if end < 0 {
				end = len(p.source) - p.at
			}
			text := strings.ReplaceAll(strings.ReplaceAll(p.source[p.at:p.at+end], "\r\n", "\n"), "\r", "\n")
			node.Children = append(node.Children, &Node{Text: true, Raw: text, Parent: node})
			p.at += end
			continue
		}
		if strings.HasPrefix(p.source[p.at:], "</") {
			p.at += strings.IndexByte(p.source[p.at:], '>') + 1
			return
		}
		node.Children = append(node.Children, p.element(node))
	}
}

func (p *rawParser) name() string {
	start := p.at
	for p.at < len(p.source) && !strings.ContainsRune(" \t\r\n/>=", rune(p.source[p.at])) {
		p.at++
	}
	return p.source[start:p.at]
}

func (p *rawParser) space() {
	for p.at < len(p.source) && strings.ContainsRune(" \t\r\n", rune(p.source[p.at])) {
		p.at++
	}
}

func split(name string) (string, string) {
	if prefix, local, ok := strings.Cut(name, ":"); ok {
		return prefix, local
	}
	return "", name
}

// RawAttribute is an unprefixed attribute's raw value (REXML's Attribute#to_s).
func (n *Node) RawAttribute(name string) (string, bool) {
	for _, attribute := range n.Attrs {
		if attribute.Name == name && attribute.Prefix == "" {
			return attribute.Raw, true
		}
	}
	return "", false
}

// crash is a Ruby error that is no Relatex::Failure (a NoMethodError on nil): nothing that
// rescues Failures catches it.
func crash(message string) {
	panic(errors.New(message))
}

// ParseRaw is REXML::Document.new(source).root without validation (a projection's output).
func ParseRaw(source string) *Node { return parseRaw(source) }

// Value is a text node's unescaped value (REXML's Text#value).
func (n *Node) Value() string { return unescape(n.Raw) }
