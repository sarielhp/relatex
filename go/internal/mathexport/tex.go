package mathexport

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

var texStyles = map[string]string{"normal": "mathrm", "italic": "mathit", "bold": "mathbf",
	"bold-italic": "boldsymbol", "double-struck": "mathbb", "script": "mathcal",
	"fraktur": "mathfrak", "sans-serif": "mathsf", "monospace": "mathtt"}

// accents and underAccents are MathExport::ACCENTS and UNDER_ACCENTS.
var (
	accents = map[string]string{"~": "tilde", "˜": "tilde", "¯": "bar", "−": "bar", "^": "hat",
		"ˆ": "hat", "→": "vec", "˙": "dot", "¨": "ddot", "ˇ": "check", "˘": "breve", "‾": "overline"}
	underAccents = map[string]string{"_": "underline", "¯": "underline", "‾": "underline",
		"−": "underline", "⏟": "underbrace", "⎵": "underbrace", "→": "underrightarrow"}
)

var texEscapes = strings.NewReplacer(`\`, `\backslash{}`, "{", `\{`, "}", `\}`, "$", `\$`, "%", `\%`, "#", `\#`,
	"&", `\&`, "_", `\_`, "^", `\^{}`, "~", `\~{}`)

// TeXText is TeXMathExport.text: TeX's special characters escaped.
func TeXText(value string) string { return texEscapes.Replace(value) }

// TeX is TeXMathExport: math as TeX, for Pandoc's reader (dialect "pandoc") or plain TeX.
type TeX struct {
	converter
	dialectName string
}

// NewTeX is TeXMathExport.new(dialect:).
func NewTeX(dialectName string) *TeX {
	if dialectName != "tex" && dialectName != "pandoc" {
		rec.Fail("Unknown TeX math dialect")
	}
	t := &TeX{dialectName: dialectName}
	t.d = t
	return t
}

// Convert is MathExport#convert.
func (t *TeX) Convert(xml string) string { return t.convert(xml) }

func (t *TeX) target() string          { return t.dialectName }
func (t *TeX) separator() string       { return " " }
func (t *TeX) empty(*Node) string      { return "" }
func (t *TeX) group(node *Node) string { return t.plainGroup(node) }

var styleCommands = []string{"displaystyle", "textstyle", "scriptstyle", "scriptscriptstyle"}

func (t *TeX) decorate(node *Node, value string) string {
	if node.Name == "mstyle" || !node.Has("displaystyle") && !node.Has("scriptlevel") && !node.Has("display") {
		return value
	}
	return "{\\" + styleCommands[t.styleLevel(node)] + " " + value + "}"
}

func (t *TeX) styleIdentifier(text, style string, hasStyle bool) string {
	value := TeXText(text)
	if !hasStyle {
		return value
	}
	if t.dialectName == "pandoc" && style == "bold" {
		rec.Fail("Native MathML preserves upright bold characters; Pandoc mathbf makes them italic")
	}
	command, ok := texStyles[style]
	if !ok {
		rec.Fail("Unsupported TeX math variant: %s", style)
	}
	return "\\" + command + "{" + value + "}"
}

func (t *TeX) number(node *Node) string   { return t.alphabet(node, "", false) }
func (t *TeX) operator(node *Node) string { return t.alphabet(node, "", false) }

func (t *TeX) spacedOperator(node *Node, value string) string {
	if !node.Has("lspace") && !node.Has("rspace") {
		return value
	}
	left, right := "0em", "0em"
	if space, ok := node.Attribute("lspace"); ok {
		left = space
	}
	if space, ok := node.Attribute("rspace"); ok {
		right = space
	}
	return t.mathematicalSpace(left) + "\\mathord{" + value + "}" + t.mathematicalSpace(right)
}

func (t *TeX) literal(node *Node) string {
	if variant, ok := t.presentation.Style(node)["mathvariant"]; ok && variant != "normal" {
		rec.Fail("Native MathML preserves styled mathematical text in Markdown")
	}
	return "\\text{" + TeXText(t.token(node)) + "}"
}

func (t *TeX) fraction(node *Node) string {
	parts := t.parts(node, 2)
	if thickness, ok := node.Attribute("linethickness"); ok {
		return "\\genfrac{}{}{" + NewLength(thickness, false).TeX() + "}{}{" + parts[0] + "}{" + parts[1] + "}"
	}
	return "\\frac{" + parts[0] + "}{" + parts[1] + "}"
}

func (t *TeX) subscript(node *Node) string {
	parts := t.parts(node, 2)
	return "{" + parts[0] + "}_{" + parts[1] + "}"
}

func (t *TeX) superscript(node *Node) string {
	parts := t.parts(node, 2)
	return "{" + parts[0] + "}^{" + parts[1] + "}"
}

func (t *TeX) scripts(node *Node) string {
	parts := t.parts(node, 3)
	return "{" + parts[0] + "}_{" + parts[1] + "}^{" + parts[2] + "}"
}

func (t *TeX) squareRoot(node *Node) string { return "\\sqrt{" + t.group(node) + "}" }

func (t *TeX) root(node *Node) string {
	parts := t.parts(node, 2)
	return "\\sqrt[" + parts[1] + "]{" + parts[0] + "}"
}

func (t *TeX) limit(node *Node, over bool) string {
	elements := t.children(node, 2)
	base, script := elements[0], elements[1]
	attribute, table, kind, mark, stack := "accent", accents, "accent", "^", "overset"
	if !over {
		attribute, table, kind, mark, stack = "accentunder", underAccents, "under-accent", "_", "underset"
	}
	if t.accent(node, attribute, script) {
		text, _ := script.TextValue()
		command, ok := table[text]
		if !ok {
			rec.Fail("Native MathML preserves this %s in Markdown", kind)
		}
		return "\\" + command + "{" + t.render(base) + "}"
	}
	if t.movableLimits(base) {
		return "\\mathop{" + t.render(base) + "}\\displaylimits" + mark + "{" + t.render(script) + "}"
	}
	return "\\" + stack + "{" + t.render(script) + "}{" + t.render(base) + "}"
}

func (t *TeX) underOver(node *Node) string {
	elements := t.children(node, 3)
	base, below, above := elements[0], elements[1], elements[2]
	if t.accent(node, "accentunder", below) || t.accent(node, "accent", above) {
		rec.Fail("Native MathML preserves combined upper and lower accents in Markdown")
	}
	if t.movableLimits(base) {
		return "\\mathop{" + t.render(base) + "}\\displaylimits_{" + t.render(below) + "}^{" + t.render(above) + "}"
	}
	return "\\overset{" + t.render(above) + "}{\\underset{" + t.render(below) + "}{" + t.render(base) + "}}"
}

var columnLetters = map[string]string{"left": "l", "center": "c", "right": "r"}

func (t *TeX) matrix(node *Node) string {
	rows := t.matrixRows(node)
	first := t.children(rows[0], -1)
	columns := []string{}
	for index, cell := range first {
		columns = append(columns, columnLetters[Alignment(node, rows[0], cell, index, "columnalign")])
	}
	lines := []string{}
	for _, row := range rows {
		cells := []string{}
		for index, cell := range t.children(row, -1) {
			alignment := columnLetters[Alignment(node, row, cell, index, "columnalign")]
			value := t.renderMatrixCell(cell, row)
			if alignment != columns[index] {
				value = "\\multicolumn{1}{" + alignment + "}{" + value + "}"
			}
			cells = append(cells, value)
		}
		lines = append(lines, strings.Join(cells, " & "))
	}
	if len(columns) != len(first) {
		rec.Fail("Matrix alignment width differs")
	}
	return "\\begin{array}{" + strings.Join(columns, "") + "}" + strings.Join(lines, " \\\\ ") + "\\end{array}"
}

func (t *TeX) space(node *Node) string {
	if node.Has("height") || node.Has("depth") || node.Has("mathbackground") {
		rec.Fail("Native MathML is required to preserve a measured mathematical strut/rule in Markdown")
	}
	width := "0em"
	if value, ok := node.Attribute("width"); ok {
		width = value
	}
	return t.mathematicalSpace(width)
}

func (t *TeX) mathematicalSpace(source string) string {
	length := NewLength(source, false)
	if length.Unit == "mu" {
		return "\\mkern" + length.TeX()
	}
	return "\\hspace{" + length.TeX() + "}"
}

func (t *TeX) phantom(node *Node) string { return "\\phantom{" + t.group(node) + "}" }

func (t *TeX) padded(node *Node) string {
	if len(node.Attrs) > 0 {
		rec.Fail("Native MathML is required to preserve adjusted box geometry in Markdown")
	}
	return t.group(node)
}

func (t *TeX) styled(node *Node) string {
	command := styleCommands[t.styleLevel(node)]
	if node.Has("mathsize") {
		rec.Fail("Native MathML is required to preserve explicit mathsize in Markdown")
	}
	return "{\\" + command + " " + t.group(node) + "}"
}

// splitScripts is a multiscript's post- and pre-scripts after its base (rendered first, as
// Ruby does, by the caller).
func (c *converter) splitScripts(rest []*Node) ([]*Node, []*Node) {
	index := -1
	for position, element := range rest {
		if element.Name == "mprescripts" {
			index = position
			break
		}
	}
	post, pre := rest, []*Node{}
	if index >= 0 {
		post, pre = rest[:index], rest[index+1:]
	}
	if len(pre)%2 != 0 || len(post)%2 != 0 {
		rec.Fail("Unpaired MathML multiscripts")
	}
	return post, pre
}

// multiscriptBase is the rendered base and the remaining scripts.
func (c *converter) multiscriptBase(node *Node) (string, []*Node) {
	elements := c.children(node, -1)
	if len(elements) == 0 {
		rec.Fail("Missing multiscript base")
	}
	return c.render(elements[0]), elements[1:]
}

func (t *TeX) multiscripts(node *Node) string {
	base, rest := t.multiscriptBase(node)
	post, pre := t.splitScripts(rest)
	before := ""
	for index := len(pre) - 2; index >= 0; index -= 2 {
		before += "{}_{" + t.render(pre[index]) + "}^{" + t.render(pre[index+1]) + "}"
	}
	after := ""
	for index := 0; index < len(post); index += 2 {
		if index > 0 {
			after += "{}"
		}
		after += "_{" + t.render(post[index]) + "}^{" + t.render(post[index+1]) + "}"
	}
	return before + "{" + base + "}" + after
}
