package mathexport

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

var typstStyles = map[string]string{"normal": "upright", "italic": "italic", "bold": "bold",
	"double-struck": "bb", "script": "cal", "fraktur": "frak", "sans-serif": "sans", "monospace": "mono"}

var typstColors = map[string]string{"black": "#000000", "white": "#ffffff", "red": "#ff0000",
	"green": "#008000", "blue": "#0000ff", "gray": "#808080", "grey": "#808080",
	"yellow": "#ffff00", "cyan": "#00ffff", "magenta": "#ff00ff"}

var (
	hexColor     = regexp.MustCompile(`\A#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?\z`)
	bareTypst    = regexp.MustCompile(`\A(?:[A-Za-z]|[0-9]+(?:\.[0-9]+)?)\z`)
	typstNumeral = regexp.MustCompile(`\A[0-9]+(?:\.[0-9]+)?\z`)
)

var typstFunctions = []string{"display", "inline", "script", "sscript"}

type fenceReference struct{ body, axis string }

// Typst is TypstMathExport: math as Typst, with relatex's l2d- layout functions.
type Typst struct {
	converter
	references map[*Node]fenceReference
}

// NewTypst is TypstMathExport.new.
func NewTypst() *Typst {
	t := &Typst{}
	t.d = t
	return t
}

// Convert is MathExport#convert.
func (t *Typst) Convert(xml string) string { return t.convert(xml) }

func quote(value string) string {
	text, err := canonjson.Marshal(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return string(text)
}

func (t *Typst) target() string { return "typst" }

// separator: a space adjacent to evaluated content is real glue in Typst math. A block
// comment separates identifiers without adding width to context/box nodes.
func (t *Typst) separator() string  { return "/**/" }
func (t *Typst) empty(*Node) string { return `""` }

func (t *Typst) equation(value string, node *Node) string {
	return "$" + typstFunctions[t.styleLevel(node)] + "(" + value + ")$"
}

func (t *Typst) decorate(node *Node, value string) string {
	if node.Has("displaystyle") || node.Has("scriptlevel") || node.Has("display") {
		value = typstFunctions[t.styleLevel(node)] + "(" + value + ")"
	}
	if size, ok := node.Attribute("mathsize"); ok {
		value = "#context text(size: l2d-length(" + NewLength(size, false).Typst() + ", natural: 1em), " + t.equation(value, node) + ")"
	}
	if color, ok := node.Attribute("mathcolor"); ok {
		value = "#text(fill: " + colorValue(color) + ", " + t.equation(value, node) + ")"
	}
	if background, ok := node.Attribute("mathbackground"); ok && node.Name != "mspace" {
		value = "#box(fill: " + colorValue(background) + ", " + t.equation(value, node) + ")"
	}
	return value
}

func colorValue(value string) string {
	if value == "transparent" {
		return "none"
	}
	color, ok := typstColors[value]
	if !ok {
		color = value
	}
	if !hexColor.MatchString(color) {
		rec.Fail("Unsupported mathematical color")
	}
	return "rgb(" + quote(color) + ")"
}

func (t *Typst) group(node *Node) string {
	elements := t.children(node, -1)
	fenced := slices.ContainsFunc(elements, t.stretchFence)
	if !fenced {
		return t.plainGroup(node)
	}
	t.mixedText(node)
	rendered := map[*Node]string{}
	values := []string{}
	for _, child := range elements {
		if !t.stretchFence(child) {
			rendered[child] = t.render(child)
			values = append(values, rendered[child])
		}
	}
	reference := joined(values, t.separator())
	if reference == "" {
		reference = `""`
	}
	target := fenceReference{body: t.equation(reference, node), axis: t.equation(`#symbol("+")`, node)}
	references := map[*Node]fenceReference{}
	for _, child := range elements {
		if t.stretchFence(child) {
			references[child] = target
		}
	}
	previous := t.references
	t.references = references
	defer func() { t.references = previous }()
	result := []string{}
	for _, child := range elements {
		if value, ok := rendered[child]; ok {
			result = append(result, value)
		} else {
			result = append(result, t.render(child))
		}
	}
	return joined(result, t.separator())
}

func (t *Typst) fencedGlyph(node *Node, value string) string {
	reference, ok := t.references[node]
	if t.references == nil || !ok {
		return value
	}
	fields := []string{"reference: " + reference.body + ", axis-body: " + reference.axis}
	for _, name := range []string{"minsize", "maxsize"} {
		if length, has := node.Attribute(name); has && length != "infinity" {
			fields = append(fields, name+": "+NewLength(length, false).Typst())
		}
	}
	return "#l2d-fenced(" + t.equation(value, node) + ", " + strings.Join(fields, ", ") + ")"
}

func (t *Typst) stretchFence(node *Node) bool {
	if node.Name != "mo" {
		return false
	}
	fence := boolean(node, "fence", strings.Contains(fences, t.token(node)))
	return fence && boolean(node, "stretchy", true)
}

func (t *Typst) styleIdentifier(text, style string, hasStyle bool) string {
	value := text
	if !bareTypst.MatchString(text) {
		value = quote(text)
	}
	if !hasStyle {
		return value
	}
	if style == "bold" || style == "sans-serif" || style == "bold-sans-serif" {
		value = "upright(" + value + ")"
	}
	switch style {
	case "bold-italic":
		return "bold(italic(" + value + "))"
	case "bold-script":
		return "bold(cal(" + value + "))"
	case "bold-fraktur":
		return "bold(frak(" + value + "))"
	case "bold-sans-serif":
		return "bold(sans(" + value + "))"
	case "sans-serif-italic":
		return "italic(sans(" + value + "))"
	case "sans-serif-bold-italic":
		return "bold(italic(sans(" + value + ")))"
	}
	function, ok := typstStyles[style]
	if !ok {
		rec.Fail("Unsupported Typst math variant: %s", style)
	}
	return function + "(" + value + ")"
}

func (t *Typst) number(node *Node) string {
	text := t.token(node)
	if text == "" {
		return t.empty(node)
	}
	value := ""
	for _, part := range runs(text, "", false) {
		value += part.text
	}
	if !typstNumeral.MatchString(value) {
		rec.Fail("Unsupported Typst numeric token")
	}
	return t.alphabet(node, "", false)
}

func (t *Typst) operator(node *Node) string {
	text := t.token(node)
	fence := boolean(node, "fence", strings.Contains(fences, text))
	value := ""
	if fence && utf8.RuneCountInString(text) == 1 {
		value = "#symbol(" + quote(text) + ")"
	} else {
		value = t.alphabet(node, "", false)
	}
	if fence {
		value = t.fencedGlyph(node, value)
	}
	if fence && !boolean(node, "stretchy", true) {
		value = "#box($" + value + "$)"
	}
	return value
}

func (t *Typst) spacedOperator(node *Node, value string) string {
	fence := boolean(node, "fence", strings.Contains(fences, t.token(node)))
	left, hasLeft := node.Attribute("lspace")
	right, hasRight := node.Attribute("rspace")
	if !hasLeft && !hasRight {
		return value
	}
	if !fence {
		value = `class("normal", ` + value + ")"
	}
	before, after := "", ""
	if hasLeft && NewLength(left, false).Value != 0 {
		before = "#h(l2d-length(" + NewLength(left, false).Typst() + "))/**/"
	}
	if hasRight && NewLength(right, false).Value != 0 {
		after = "/**/#h(l2d-length(" + NewLength(right, false).Typst() + "))"
	}
	return before + value + after
}

func (t *Typst) literal(node *Node) string { return t.alphabet(node, "normal", true) }

func (t *Typst) fraction(node *Node) string {
	if thickness, ok := node.Attribute("linethickness"); ok {
		parts := t.parts(node, 2)
		length := NewLength(thickness, false).Typst()
		style := t.presentation.Style(node)
		level, has := style["scriptlevel"]
		if !has {
			panic(rec.Failure{Message: `key not found: "scriptlevel"`, Lookup: true})
		}
		return fmt.Sprintf("#l2d-frac($%s$, $%s$, %s, display: %t, level: %d)", parts[0], parts[1], length,
			style["displaystyle"] == "true", rubyInteger(level))
	}
	return "frac(" + strings.Join(t.parts(node, 2), ", ") + ")"
}

func emptyBase(base string) string {
	if base == "" {
		return `""`
	}
	return base
}

func (t *Typst) subscript(node *Node) string {
	parts := t.parts(node, 2)
	return "scripts(" + emptyBase(parts[0]) + ")_(" + parts[1] + ")"
}

func (t *Typst) superscript(node *Node) string {
	parts := t.parts(node, 2)
	return "scripts(" + emptyBase(parts[0]) + ")^(" + parts[1] + ")"
}

func (t *Typst) scripts(node *Node) string {
	parts := t.parts(node, 3)
	return "scripts(" + emptyBase(parts[0]) + ")_(" + parts[1] + ")^(" + parts[2] + ")"
}

func (t *Typst) squareRoot(node *Node) string { return "sqrt(" + t.group(node) + ")" }

func (t *Typst) root(node *Node) string {
	parts := t.parts(node, 2)
	return "root(" + parts[1] + ", " + parts[0] + ")"
}

func (t *Typst) limit(node *Node, over bool) string {
	elements := t.children(node, 2)
	base, script := elements[0], elements[1]
	attribute, brace, command, attach := "accent", "⏞", "overbrace", "t"
	if !over {
		attribute, brace, command, attach = "accentunder", "⏟", "underbrace", "b"
	}
	if t.accent(node, attribute, script) {
		if over {
			return t.topAccent(t.render(base), script)
		}
		return t.bottomAccent(t.render(base), script)
	}
	if text, ok := script.TextValue(); script.Name == "mo" && ok && text == brace {
		return command + "(" + t.render(base) + ")"
	}
	limits := "true"
	if t.movableLimits(base) {
		limits = "false"
	}
	return "attach(limits(" + t.render(base) + ", inline: #" + limits + "), " + attach + ": " + t.render(script) + ")"
}

func (t *Typst) underOver(node *Node) string {
	elements := t.children(node, 3)
	base, sub, sup := elements[0], elements[1], elements[2]
	value := t.render(base)
	if t.accent(node, "accentunder", sub) {
		value = t.bottomAccent(value, sub)
	}
	if t.accent(node, "accent", sup) {
		value = t.topAccent(value, sup)
	}
	fields := []string{}
	if !t.accent(node, "accentunder", sub) {
		fields = append(fields, "b: "+t.render(sub))
	}
	if !t.accent(node, "accent", sup) {
		fields = append(fields, "t: "+t.render(sup))
	}
	if len(fields) == 0 {
		return value
	}
	limits := "true"
	if t.movableLimits(base) {
		limits = "false"
	}
	return "attach(limits(" + value + ", inline: #" + limits + "), " + strings.Join(fields, ", ") + ")"
}

func decorationText(decoration *Node) string {
	text, ok := decoration.TextValue()
	if !ok {
		return "null"
	}
	return quote(text)
}

func (t *Typst) topAccent(value string, decoration *Node) string {
	if text, ok := decoration.TextValue(); ok && text == "‾" {
		return "overline(" + value + ")"
	}
	size := ""
	if !boolean(decoration, "stretchy", true) {
		size = ", size: #0pt"
	}
	return "accent(" + value + ", " + decorationText(decoration) + size + ")"
}

func (t *Typst) bottomAccent(value string, decoration *Node) string {
	text, _ := decoration.TextValue()
	switch underAccents[text] {
	case "underline":
		return "underline(" + value + ")"
	case "underbrace":
		return "underbrace(" + value + ")"
	case "underrightarrow":
		return "attach(" + value + ", b: stretch(" + decorationText(decoration) + "))"
	}
	rec.Fail("Unsupported native Typst under-accent")
	return ""
}

func (t *Typst) matrix(node *Node) string {
	rows := t.matrixRows(node)
	alignment, vertical, cells := [][]string{}, [][]string{}, [][]string{}
	for _, row := range rows {
		line := []string{}
		for index, cell := range t.children(row, -1) {
			line = append(line, Alignment(node, row, cell, index, "columnalign"))
		}
		alignment = append(alignment, line)
	}
	for _, row := range rows {
		line := []string{}
		for index, cell := range t.children(row, -1) {
			line = append(line, Alignment(node, row, cell, index, "rowalign"))
		}
		vertical = append(vertical, line)
	}
	for _, row := range rows {
		line := []string{}
		for _, cell := range t.children(row, -1) {
			line = append(line, t.renderMatrixCell(cell, row))
		}
		cells = append(cells, line)
	}
	if uniform(alignment) && onlyBaseline(vertical) && !node.Has("columnspacing") && !node.Has("rowspacing") {
		lines := []string{}
		for _, line := range cells {
			lines = append(lines, strings.Join(line, ", "))
		}
		return "mat(delim: #none, align: #" + alignment[0][0] + ", " + strings.Join(lines, "; ") + ")"
	}
	values := []string{}
	for index, line := range cells {
		bodies := []string{}
		for column, part := range line {
			bodies = append(bodies, t.equation(part, t.children(rows[index], -1)[column]))
		}
		values = append(values, "("+strings.Join(bodies, ", ")+",)")
	}
	fields := []string{"alignments: " + stringRows(alignment), "vertical-alignments: " + stringRows(vertical)}
	for _, name := range []string{"columnspacing", "rowspacing"} {
		value, ok := node.Attribute(name)
		if !ok {
			continue
		}
		lengths := []string{}
		for _, length := range strings.Fields(value) {
			lengths = append(lengths, NewLength(length, false).Typst())
		}
		label := "row-gaps"
		if name == "columnspacing" {
			label = "column-gaps"
		}
		fields = append(fields, label+": ("+strings.Join(lengths, ", ")+",)")
	}
	return "#l2d-matrix((" + strings.Join(values, ", ") + ",), " + strings.Join(fields, ", ") + ")"
}

func uniform(rows [][]string) bool {
	first := rows[0][0]
	for _, row := range rows {
		for _, value := range row {
			if value != first {
				return false
			}
		}
	}
	return true
}

func onlyBaseline(rows [][]string) bool {
	for _, row := range rows {
		for _, value := range row {
			if value != "baseline" {
				return false
			}
		}
	}
	return true
}

func stringRows(rows [][]string) string {
	lines := []string{}
	for _, row := range rows {
		values := []string{}
		for _, value := range row {
			values = append(values, quote(value))
		}
		lines = append(lines, "("+strings.Join(values, ", ")+",)")
	}
	return "(" + strings.Join(lines, ", ") + ",)"
}

func (t *Typst) space(node *Node) string {
	width := "0em"
	if value, ok := node.Attribute("width"); ok {
		width = value
	}
	widthValue := NewLength(width, false).Typst()
	fields := []string{}
	for _, name := range []string{"height", "depth"} {
		if value, ok := node.Attribute(name); ok {
			fields = append(fields, name+": "+NewLength(value, false).Typst())
		}
	}
	if color, ok := node.Attribute("mathbackground"); ok {
		fields = append(fields, "fill: "+colorValue(color))
	}
	extra := ""
	if len(fields) > 0 {
		extra = ", " + strings.Join(fields, ", ")
	}
	return "#l2d-space(" + widthValue + extra + ")"
}

func (t *Typst) phantom(node *Node) string {
	return "#hide(" + t.equation(t.group(node), node) + ")"
}

func (t *Typst) padded(node *Node) string {
	fence, size := FenceBox(node)
	value := ""
	if fence != nil {
		value = "#l2d-fenced(" + t.equation(t.operator(fence), node) + ", minsize: " + size.Typst() + ", maxsize: " + size.Typst() + ")"
	} else {
		value = t.group(node)
	}
	fields := []string{}
	for _, name := range []string{"width", "height", "depth", "lspace", "voffset"} {
		if attribute, ok := node.Attribute(name); ok {
			padded := name == "width" || name == "height" || name == "depth"
			fields = append(fields, name+": "+NewLength(attribute, padded).Typst())
		}
	}
	elements := t.children(node, -1)
	if fence == nil && len(elements) == 1 {
		if metrics := t.presentation.FixedBoxMetrics(elements[0]); metrics != nil {
			values := []string{}
			for _, metric := range metrics {
				values = append(values, metric.Name+": l2d-length("+metric.Length.Typst()+")")
			}
			fields = append(fields, "body-metrics: ("+strings.Join(values, ", ")+")")
		}
	}
	extra := ""
	if len(fields) > 0 {
		extra = ", " + strings.Join(fields, ", ")
	}
	return "#l2d-padded(" + t.equation(value, node) + extra + ")"
}

func (t *Typst) styled(node *Node) string { return t.group(node) }

func (t *Typst) multiscripts(node *Node) string {
	base, rest := t.multiscriptBase(node)
	post, pre := t.splitScripts(rest)
	before, after := pairs(pre), pairs(post)
	first := func(list [][2]*Node) []*Node {
		if len(list) == 0 {
			return nil
		}
		return list[0][:]
	}
	central := t.scriptAttachment(base, first(before), first(after))
	ghost := "#box(width: 0pt, hide(" + t.equation(base, node) + "))"
	left := []string{}
	for index := len(before) - 1; index >= 1; index-- {
		left = append(left, t.scriptAttachment(ghost, before[index][:], nil))
	}
	right := []string{}
	for index := 1; index < len(after); index++ {
		right = append(right, t.scriptAttachment(ghost, nil, after[index][:]))
	}
	parts := []string{}
	for _, part := range []string{joined(left, t.separator()), central, joined(right, t.separator())} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return joined(parts, t.separator())
}

func pairs(nodes []*Node) [][2]*Node {
	result := [][2]*Node{}
	for index := 0; index+1 < len(nodes); index += 2 {
		result = append(result, [2]*Node{nodes[index], nodes[index+1]})
	}
	return result
}

func (t *Typst) scriptAttachment(base string, before, after []*Node) string {
	fields := []string{}
	positions := []struct {
		nodes []*Node
		names [2]string
	}{{before, [2]string{"bl", "tl"}}, {after, [2]string{"br", "tr"}}}
	for _, group := range positions {
		for index, element := range group.nodes {
			if element.Name != "none" {
				fields = append(fields, group.names[index]+": "+t.render(element))
			}
		}
	}
	if len(fields) == 0 {
		return "attach(" + base + ")"
	}
	return "attach(" + base + ", " + strings.Join(fields, ", ") + ")"
}
