package mathexport

import (
	"slices"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// fences is MathExport::FENCES.
const fences = "()[]{}⟨⟩⌊⌋⌈⌉|‖⎰⎱⟮⟯⦅⦆"

// dialect is what a renderer-specific representation adds to MathExport: the handlers it
// writes its own way.
type dialect interface {
	target() string
	separator() string
	empty(node *Node) string
	decorate(node *Node, value string) string
	styleIdentifier(text, style string, hasStyle bool) string
	number(node *Node) string
	operator(node *Node) string
	spacedOperator(node *Node, value string) string
	literal(node *Node) string
	fraction(node *Node) string
	subscript(node *Node) string
	superscript(node *Node) string
	scripts(node *Node) string
	squareRoot(node *Node) string
	root(node *Node) string
	limit(node *Node, over bool) string
	underOver(node *Node) string
	matrix(node *Node) string
	space(node *Node) string
	phantom(node *Node) string
	padded(node *Node) string
	styled(node *Node) string
	multiscripts(node *Node) string
	group(node *Node) string
}

// converter is MathExport: renderer-specific representations, never described as
// recovered source TeX.
type converter struct {
	presentation *Presentation
	context      []*Node
	d            dialect
}

// convert is MathExport#convert.
func (c *converter) convert(xml string) string {
	c.presentation = NewPresentation(xml)
	c.context = nil
	validateSupport(c.presentation, c.d.target())
	return c.render(c.presentation.Root)
}

// located prefixes a Failure raised under node with the node's path, as render's rescue does.
func (c *converter) located(node *Node) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, ok := recovered.(rec.Failure)
	if !ok || failure.Lookup {
		panic(recovered)
	}
	panic(rec.Failure{Message: c.presentation.Path(node) + ": " + failure.Message})
}

func (c *converter) render(node *Node) (value string) {
	defer c.located(node)
	operator := c.coreOperator(node)
	owns := operator != nil && (len(c.context) == 0 || c.context[len(c.context)-1] != operator)
	if owns {
		c.context = append(c.context, operator)
		defer func() { c.context = c.context[:len(c.context)-1] }()
	}
	value = c.d.decorate(node, c.handle(node))
	if owns {
		value = c.d.spacedOperator(operator, value)
	}
	return value
}

func (c *converter) handle(node *Node) string {
	d := c.d
	handlers := map[string]func(*Node) string{
		"math": d.group, "mrow": d.group, "mtd": d.group, "mi": c.identifier, "mn": d.number, "mo": d.operator,
		"mtext": d.literal, "ms": d.literal, "mfrac": d.fraction, "msub": d.subscript, "msup": d.superscript,
		"msubsup": d.scripts, "msqrt": d.squareRoot, "mroot": d.root,
		"mover":  func(n *Node) string { return d.limit(n, true) },
		"munder": func(n *Node) string { return d.limit(n, false) }, "munderover": d.underOver, "mtable": d.matrix,
		"mspace": d.space, "mphantom": d.phantom, "mpadded": d.padded, "mmultiscripts": d.multiscripts,
		"none": d.empty, "mstyle": d.styled,
	}
	handler, ok := handlers[node.Name]
	if !ok {
		rec.Fail("Unsupported export MathML: %s", node.Name)
	}
	return handler(node)
}

func (c *converter) children(node *Node, count int) []*Node {
	elements := node.Elements()
	if count >= 0 && len(elements) != count {
		rec.Fail("Wrong export MathML arity: %s", node.Name)
	}
	return elements
}

func (c *converter) parts(node *Node, count int) []string {
	result := []string{}
	for _, child := range c.children(node, count) {
		result = append(result, c.render(child))
	}
	return result
}

// plainGroup is MathExport#group.
func (c *converter) plainGroup(node *Node) string {
	c.mixedText(node)
	value := joined(c.parts(node, -1), c.d.separator())
	if value == "" {
		return c.d.empty(node)
	}
	return value
}

func (c *converter) mixedText(node *Node) {
	for _, text := range node.Texts() {
		if rec.Strip(text) != "" {
			rec.Fail("Mixed unstructured MathML text")
		}
	}
}

func joined(values []string, separator string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += separator
		}
		result += value
	}
	return result
}

func (c *converter) token(node *Node) string {
	if len(node.Elements()) > 0 {
		rec.Fail("MathML token has element children")
	}
	return joined(node.Texts(), "")
}

func (c *converter) identifier(node *Node) string {
	if utf8.RuneCountInString(c.token(node)) > 1 {
		return c.alphabet(node, "normal", true)
	}
	return c.alphabet(node, "", false)
}

func (c *converter) alphabet(node *Node, fallback string, hasFallback bool) string {
	text := c.token(node)
	if text == "" {
		return c.d.empty(node)
	}
	variant, has := c.presentation.Style(node)["mathvariant"]
	if !has {
		variant, has = fallback, hasFallback
	}
	values := []string{}
	for _, part := range runs(text, variant, has) {
		values = append(values, c.d.styleIdentifier(part.text, part.variant, part.hasVariant))
	}
	return joined(values, c.d.separator())
}

func boolean(node *Node, name string, fallback bool) bool {
	if value, ok := node.Attribute(name); ok {
		return value == "true"
	}
	return fallback
}

func (c *converter) accent(owner *Node, name string, decoration *Node) bool {
	return c.presentation.Accent(owner, name, decoration)
}

var (
	wrappers    = []string{"math", "mrow", "mstyle", "mpadded"}
	attachments = []string{"msub", "msup", "msubsup", "mmultiscripts", "mover", "munder", "munderover"}
)

func (c *converter) coreOperator(node *Node) *Node {
	for {
		if node.Name == "mo" {
			return node
		}
		wrapper := slices.Contains(wrappers, node.Name) && len(node.Elements()) == 1
		if !wrapper && !slices.Contains(attachments, node.Name) {
			return nil
		}
		child := node.Element(1)
		if child == nil {
			return nil
		}
		node = child
	}
}

func (c *converter) movableLimits(base *Node) bool {
	operator := c.coreOperator(base)
	return operator != nil && boolean(operator, "movablelimits", false)
}

func (c *converter) matrixRows(node *Node) []*Node {
	rows := c.children(node, -1)
	valid := len(rows) > 0
	for _, row := range rows {
		valid = valid && row.Name == "mtr"
	}
	if !valid {
		rec.Fail("MathML matrix has no rows or a non-row child")
	}
	for _, row := range rows {
		for _, cell := range c.children(row, -1) {
			if cell.Name != "mtd" {
				rec.Fail("Non-cell MathML matrix child")
			}
		}
	}
	width := len(c.children(rows[0], -1))
	for _, row := range rows {
		if len(c.children(row, -1)) != width {
			rec.Fail("Ragged or empty MathML matrix")
		}
	}
	if width == 0 {
		rec.Fail("Ragged or empty MathML matrix")
	}
	return rows
}

func (c *converter) renderMatrixCell(cell, row *Node) string {
	return c.d.decorate(row, c.render(cell))
}

// styleLevel is MathExport#style_level: display, text, script or scriptscript (0 to 3).
func (c *converter) styleLevel(node *Node) int {
	style := c.presentation.Style(node)
	level := 0
	if value, ok := style["scriptlevel"]; ok {
		level = rubyInteger(value)
	}
	if level < 0 || level > 2 {
		rec.Fail("Invalid mathematical script level")
	}
	if level > 0 {
		return level + 1
	}
	if style["displaystyle"] == "true" {
		return 0
	}
	return 1
}
