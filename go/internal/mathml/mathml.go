// Package mathml is the Go port of Relatex::MathML and PaperMathML: MathML is parsed and
// unknown markup rejected, rather than mathematical content silently deleted.
package mathml

import (
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Namespace is MathML::NAMESPACE.
const Namespace = "http://www.w3.org/1998/Math/MathML"

// MaxDepth is MathML::MAX_DEPTH: the deepest element a formula may nest. The limit guards against
// runaway recursion, not fidelity; the HoTT book's type-theory inference trees reach 79 levels, so
// it is 128 (it was 64). The independent checker (lib/relatex/mathml.rb) uses the same value.
const MaxDepth = 128

// hbarOverstrike is the kernel spelling of \hbar without amsfonts (fontmath.ltx:
// \mathchar'26\mkern-9mu h): the spacing macron struck over the italic h. LuaMML reads the two
// glyphs and the negative kern as a row; they are one letterlike character, U+210F.
var hbarOverstrike = regexp.MustCompile(
	`<mrow><mi mathvariant="normal">¯</mi><mspace width="-[0-9.]+em"/><mi>ℎ</mi></mrow>`)

// CollapseHbar writes the kernel's overstruck \hbar as the single character ℏ (U+210F) the
// source names. Only that exact row collapses: \bar{h} and \overline{h} are <mover> trees, and
// an amsfonts \hbar is already U+210F.
func CollapseHbar(source string) string {
	if !strings.Contains(source, "¯") {
		return source
	}
	return hbarOverstrike.ReplaceAllString(source, `<mi mathvariant="normal">ℏ</mi>`)
}

// Elements and Attributes are MathML::ELEMENTS and ATTRIBUTES.
var (
	Elements   = []string{"math", "mfrac", "mi", "mn", "mo", "mrow", "mspace", "mtable", "mtd", "mtext", "mtr"}
	Attributes = []string{"xmlns", "class", "columnalign", "columnspacing", "display", "displaystyle", "fence", "intent",
		"lspace", "rspace", "symmetric", "width"}
)

// PaperElements and PaperAttributes are PaperMathML's.
var (
	PaperElements = append(slices.Clone(Elements), "msub", "msup", "msubsup", "mover", "munder", "munderover",
		"mmultiscripts", "mprescripts", "none", "mpadded", "mphantom", "msqrt", "mroot", "ms", "mstyle")
	PaperAttributes = unique(append(slices.Clone(Attributes), "mathvariant", "stretchy", "depth", "height", "movablelimits",
		"accent", "accentunder", "scriptlevel", "mathsize", "voffset", "lspace", "linethickness", "minsize", "maxsize",
		"rowalign", "rowspacing", "columnlines", "rowlines", "frame", "framespacing", "mathcolor", "mathbackground", "dir",
		"bevelled", "rowspacing", "columnspan", "rowspan"))
)

func unique(values []string) []string {
	result := []string{}
	for _, value := range values {
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

// Attribute is an attribute as REXML names it: its prefix and local name.
type Attribute struct {
	Prefix, Name, Value string
}

// Expanded is REXML's expanded_name.
func (a Attribute) Expanded() string {
	if a.Prefix == "" {
		return a.Name
	}
	return a.Prefix + ":" + a.Name
}

// Node is an element (Name set) or a text node.
type Node struct {
	Prefix, Name, Namespace string
	Attrs                   []Attribute
	Children                []*Node
	Text                    string
	text                    bool
}

// Attr is attributes[name] for an unprefixed attribute ("" when absent).
func (n *Node) Attr(name string) string {
	for _, attribute := range n.Attrs {
		if attribute.Prefix == "" && attribute.Name == name {
			return attribute.Value
		}
	}
	return ""
}

// Elements are the element children.
func (n *Node) Elements() []*Node {
	result := []*Node{}
	for _, child := range n.Children {
		if !child.text {
			result = append(result, child)
		}
	}
	return result
}

// Named are the element children with a name.
func (n *Node) Named(name string) []*Node {
	result := []*Node{}
	for _, child := range n.Elements() {
		if child.Name == name {
			result = append(result, child)
		}
	}
	return result
}

// FirstText is REXML's Element#text: the first text child, or nil.
func (n *Node) FirstText() (string, bool) {
	for _, child := range n.Children {
		if child.text {
			return child.Text, true
		}
	}
	return "", false
}

// Row is math.elements["/math/mtable/mtr[index]"]: the index-th row (from 1) of the first
// table that has one.
func (n *Node) Row(index int) *Node {
	for _, table := range n.Named("mtable") {
		if rows := table.Named("mtr"); len(rows) >= index {
			return rows[index-1]
		}
	}
	return nil
}

func controlCharacter(character rune) bool {
	return character <= 0x08 || character == 0x0B || character == 0x0C || (character >= 0x0E && character <= 0x1F) ||
		(character >= 0x7F && character <= 0x9F)
}

// Parse is MathML.parse!.
func Parse(source string) *Node { return ParseWith(source, Elements, Attributes) }

// ParsePaper is PaperMathML.parse!.
func ParsePaper(source string) *Node { return ParseWith(source, PaperElements, PaperAttributes) }

// ParseWith is MathML.parse_with!: the root element, validated.
func ParseWith(source string, elements, attributes []string) *Node {
	if strings.Contains(source, "<!") || strings.Contains(source, "<?") {
		rec.Fail("MathML declarations are forbidden")
	}
	if len(source) > 1_000_000 {
		rec.Fail("MathML size limit")
	}
	if strings.ContainsFunc(source, controlCharacter) {
		rec.Fail("MathML control characters are forbidden")
	}
	roots, err := parse(source)
	if err != nil {
		rec.Fail("Invalid MathML: %s", err.Error())
	}
	if len(roots) != 1 || roots[0].Name != "math" {
		rec.Fail("Expected a MathML root")
	}
	validate(roots[0], 0, elements, attributes)
	return roots[0]
}

func validate(element *Node, depth int, elements, attributes []string) {
	if depth > MaxDepth {
		rec.Fail("MathML depth limit")
	}
	if !slices.Contains(elements, element.Name) || element.Prefix != "" || element.Namespace != Namespace {
		name := element.Name
		if element.Prefix != "" {
			name = element.Prefix + ":" + name
		}
		rec.Fail("Unsupported MathML element: %s", name)
	}
	if depth > 0 && element.Name == "math" {
		rec.Fail("Nested MathML root")
	}
	for _, attribute := range element.Attrs {
		if !slices.Contains(attributes, attribute.Expanded()) {
			rec.Fail("Unsupported MathML attribute: %s", attribute.Expanded())
		}
		if attribute.Name == "xmlns" && attribute.Value != Namespace {
			rec.Fail("Unexpected MathML namespace")
		}
	}
	for _, child := range element.Elements() {
		validate(child, depth+1, elements, attributes)
	}
}

// parse reads the document with its prefixes kept (RawToken) and resolves each element's
// namespace from the xmlns declarations in scope.
func parse(source string) ([]*Node, error) {
	decoder := xml.NewDecoder(strings.NewReader(source))
	document := &Node{}
	stack := []*Node{document}
	scopes := []map[string]string{{"": ""}}
	for {
		token, err := decoder.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		parent := stack[len(stack)-1]
		switch typed := token.(type) {
		case xml.StartElement:
			node, scope := element(typed, scopes[len(scopes)-1])
			parent.Children = append(parent.Children, node)
			stack = append(stack, node)
			scopes = append(scopes, scope)
		case xml.EndElement:
			if len(stack) == 1 || parent.Prefix != typed.Name.Space || parent.Name != typed.Name.Local {
				return nil, errors.New("unexpected end tag")
			}
			stack, scopes = stack[:len(stack)-1], scopes[:len(scopes)-1]
		case xml.CharData:
			if len(stack) > 1 {
				parent.Children = append(parent.Children, &Node{Text: string(typed), text: true})
			} else if strings.TrimSpace(string(typed)) != "" {
				return nil, errors.New("text outside the root element")
			}
		}
	}
	if len(stack) != 1 {
		return nil, errors.New("unclosed element")
	}
	return document.Elements(), nil
}

func element(start xml.StartElement, outer map[string]string) (*Node, map[string]string) {
	scope := map[string]string{}
	for prefix, uri := range outer {
		scope[prefix] = uri
	}
	node := &Node{Prefix: start.Name.Space, Name: start.Name.Local}
	for _, attribute := range start.Attr {
		node.Attrs = append(node.Attrs, Attribute{Prefix: attribute.Name.Space, Name: attribute.Name.Local, Value: attribute.Value})
		if attribute.Name.Space == "" && attribute.Name.Local == "xmlns" {
			scope[""] = attribute.Value
		}
		if attribute.Name.Space == "xmlns" {
			scope[attribute.Name.Local] = attribute.Value
		}
	}
	node.Namespace = scope[node.Prefix]
	return node, scope
}
