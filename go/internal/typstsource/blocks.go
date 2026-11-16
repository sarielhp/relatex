package typstsource

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/rec"
)

// Block structure of the source-faithful writer (TypstSourceBlocks): displays, theorems,
// proofs, lists, floats and tables.

var (
	alignment     = regexp.MustCompile(`<mtable class="(?:align|gather|alignat|flalign|multline|split)`)
	boldHead      = regexp.MustCompile(`\A[ \t\n\v\f\r]*(\p{L}+\.?)[ \t\n\v\f\r]+([\p{L}\p{N}][\p{L}\p{N}.]*?)[ \t\n\v\f\r]*\z`)
	itemLabel     = regexp.MustCompile(`\A[ \t\n\v\f\r]*(\(?)([0-9ivxlcIVXLC]+|[a-zA-Z])([.)])?[ \t\n\v\f\r]+`)
	bullet        = regexp.MustCompile(`\A[ \t\n\v\f\r]*[•·∙◦▪–\-*][ \t\n\v\f\r]+`)
	captionHead   = regexp.MustCompile(`\A[ \t\n\v\f\r]*(\p{L}+\.?)[ \t\n\v\f\r]+([\p{L}\p{N}][\p{L}\p{N}.]*?)[ \t\n\v\f\r]*([:.])[ \t\n\v\f\r]*`)
	proofHead     = regexp.MustCompile(`\A[ \t\n\v\f\r]*(Proof(?: of [^.]*)?)\.[ \t\n\v\f\r]*`)
	proofEnd      = regexp.MustCompile(`[ \t\n\v\f\r]*(?:□|∎|\\?\[?QED\\?\]?)[ \t\n\v\f\r]*\z`)
	nonIdentifier = regexp.MustCompile(`[^a-z0-9]`)
)

// note is TypstSourceBlocks::NOTE: a parenthesized or bracketed theorem note.
const note = `\(([^()]*(?:\([^()]*\)[^()]*)*)\)|\[([^\[\]]*)\]`

var restated = []string{"restatable", "document"}

var reserved = []string{"align", "block", "box", "cite", "figure", "footnote", "heading", "image", "link", "list", "enum",
	"table", "terms", "text", "ref", "proof", "abstract", "bibitem", "math", "par", "emph", "strong", "raw", "quote",
	"outline", "title", "grid", "stack", "page", "place"}

var cellAlignments = map[string]string{"l": "left", "c": "center", "r": "right"}

// ---- displays ----

func (e *exporter) display(node rec.Map) string {
	spans := content(node)
	index := e.displayFormula(spans)
	if index < 0 {
		return e.inline(spans, node, 0, false)
	}
	math := e.node(rec.Fetch(spans[index], "node"))
	rows := []rec.Map{}
	for _, id := range children(math) {
		if child := e.node(id); child["type"] == "math_row" {
			rows = append(rows, child)
		}
	}
	tagged := []rec.Map{}
	for _, row := range rows {
		if rec.Strip(toS(row["tag"])) != "" {
			tagged = append(tagged, row)
		}
	}
	if len(tagged) > 1 {
		return e.displayRows(node, math, rows)
	}
	tags := slices.Clone(e.tags[toS(node["id"])])
	for _, row := range tagged {
		tags = append(tags, rowTag(row))
	}
	for _, span := range spans {
		if text := rec.Strip(toS(span["text"])); span["type"] == "text" && tagPattern.MatchString(text) {
			tags = append(tags, text)
		}
	}
	tags = append(tags, e.mathTags(spans, index)...)
	var tag *string
	if len(tags) > 0 {
		tag = &tags[0]
	}
	candidates := e.displayCandidates(node, rows)
	label := e.pickLabel(candidates, tag)
	return joinPresent("\n", e.equation(e.displayBody(math), tag, label), e.anchorsOf(candidates, ""))
}

// displayCandidates: the labels a display may carry: its own, its tag paragraph's ((16) set
// beside it) and its rows'.
func (e *exporter) displayCandidates(node rec.Map, rows []rec.Map) []string {
	id := toS(node["id"])
	names := slices.Concat(e.labelsByTarget[id], e.tagLabels[id])
	for _, row := range rows {
		names = append(names, e.labelsByTarget[toS(row["id"])]...)
	}
	return names
}

// pickLabel: the element's label among the candidates: the first whose LaTeX number is the
// printed tag (a subequation row's (9a) over its group's (9)), else the first. A claimed label
// is another element's.
func (e *exporter) pickLabel(candidates []string, tag *string) string {
	free := slices.DeleteFunc(slices.Clone(candidates), func(name string) bool { return e.claimedLabels[name] })
	if len(free) == 0 {
		return ""
	}
	if tag != nil {
		for _, name := range free {
			if comparableNumber(e.labelNumber(name)) == comparableNumber(*tag) {
				return name
			}
		}
	}
	return free[0]
}

// displayFormula is the index of the display's own formula: its block formula, not an inline
// formula of a tag typeset with math (\tag{$A$} reads "(" 𝐴 ")" beside it); else its first formula.
func (e *exporter) displayFormula(spans []rec.Map) int {
	first := -1
	for index, span := range spans {
		if span["type"] != "math" {
			continue
		}
		if strings.Contains(toS(e.node(rec.Fetch(span, "node"))["mathml"]), `display="block"`) {
			return index
		}
		if first < 0 {
			first = index
		}
	}
	return first
}

// mathTags: the spans before or after the display formula that hold a formula and print "(…)"
// are a tag typeset with math, read as its characters ((A) for \tag{$A$}).
func (e *exporter) mathTags(spans []rec.Map, display int) []string {
	tags := []string{}
	for _, part := range [][]rec.Map{spans[:display], spans[display+1:]} {
		if !slices.ContainsFunc(part, func(span rec.Map) bool { return span["type"] == "math" }) {
			continue
		}
		values := make([]any, len(part))
		for index, span := range part {
			values[index] = span
		}
		if tag, ok := e.tagText(rec.Map{"content": values}); ok {
			tags = append(tags, tag)
		}
	}
	return tags
}

func rowTag(row rec.Map) string {
	tag := rec.Strip(toS(row["tag"]))
	return "(" + strings.TrimSuffix(strings.TrimPrefix(tag, "("), ")") + ")"
}

// displayRows: one display per numbered row: Typst numbers whole equations.
func (e *exporter) displayRows(node, math rec.Map, rows []rec.Map) string {
	cells, ok := e.math.rows(rec.FetchString(math, "mathml"))
	candidates := e.displayCandidates(node, rows)
	// Rows the MathML does not give one by one: one equation, its row numbers not written
	// (counted in the report); its labels are link targets after it.
	if !ok || len(cells) != len(rows) {
		e.collapsed++
		return joinPresent("\n", e.equation(e.displayBody(math), nil, ""), e.anchorsOf(candidates, ""))
	}
	equations := make([]string, len(rows), len(rows)+1)
	for index, row := range rows {
		var tag *string
		if rec.Strip(toS(row["tag"])) != "" {
			text := rowTag(row)
			tag = &text
		}
		label := e.pickLabel(e.labelsByTarget[toS(row["id"])], tag)
		equations[index] = e.equation(strings.Join(cells[index], " & "), tag, label)
	}
	// The display's own labels and the rows' others follow as link targets.
	if anchors := e.anchorsOf(candidates, ""); anchors != "" {
		equations = append(equations, anchors)
	}
	return strings.Join(equations, "\n")
}

func (e *exporter) displayBody(math rec.Map) (body string) {
	defer e.lost(rec.Fetch(math, "id"), &body)
	mathml := rec.FetchString(math, "mathml")
	if alignment.MatchString(mathml) {
		if rows, ok := e.math.rows(mathml); ok {
			joined := make([]string, len(rows))
			for index, row := range rows {
				joined[index] = strings.Join(row, " & ")
			}
			return strings.Join(joined, " \\\n  ")
		}
	}
	return e.math.convert(mathml)
}

func (e *exporter) equation(body string, tag *string, label string) string {
	if tag == nil {
		return "$ " + body + " $ <ltd-nonumber>"
	}
	directive := e.equations.next(*tag)
	lines := []string{}
	if directive.mode != nil {
		lines = append(lines, e.scoped("#set math.equation(numbering: "+e.equations.numbering(*directive.mode)+")"))
		if directive.mode.kind == "sectioned" {
			e.resetBySection("counter(math.equation)")
		}
	}
	if directive.update != nil {
		lines = append(lines, "#counter(math.equation).update("+directive.update.String()+")")
	}
	// A footnote is set with the document's first styles, before the numbering's #set: its
	// equations print and count only with a numbering of their own.
	override := directive.override
	if override == nil && e.noteDepth > 0 {
		override = tag
	}
	if label != "" {
		e.labelNumbers[label] = strings.TrimSuffix(strings.TrimPrefix(*tag, "("), ")")
		// A numbering function prints the whole tag, parentheses included, where a reference
		// to it is set: (@eq) would print ((5a)).
		if override != nil {
			e.labelNumbers[label] = *override
		}
	}
	suffix := e.labelSuffix(label, true, tag)
	if override != nil {
		lines = append(lines, "#math.equation(block: true, numbering: _ => "+typstString(*override)+", $"+body+"$)"+suffix)
	} else {
		lines = append(lines, "$ "+body+" $"+suffix)
	}
	return strings.Join(lines, "\n")
}

func (e *exporter) resetBySection(counter string) {
	if !slices.Contains(e.sectioned, counter) {
		e.sectioned = append(e.sectioned, counter)
	}
}

// ---- theorems and proofs ----

func (e *exporter) nodesOf(node rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, id := range children(node) {
		result = append(result, e.node(id))
	}
	return result
}

func (e *exporter) theorem(node rec.Map) string {
	if output, ok := e.beamerBlock(node); ok {
		return output
	}
	parts := e.nodesOf(node)
	index := slices.IndexFunc(parts, func(part rec.Map) bool {
		_, ok := e.theoremHead(node, part)
		return ok
	})
	if index < 0 {
		return e.children(node)
	}
	first, rest := parts[index], parts[index+1:]
	head, _ := e.theoremHead(node, first)
	label := e.elementLabel(node, head.number)
	// A run-in heading TeX set inside the theorem's first paragraph is printed before it.
	outputs := []string{}
	for _, part := range parts[:index] {
		if output := e.renderNode(toS(part["id"])); output != "" {
			outputs = append(outputs, output)
		}
	}
	outputs = append(outputs, e.theoremCall(node, first, rest, head, label))
	return strings.Join(outputs, "\n\n")
}

func (e *exporter) theoremCall(node, first rec.Map, rest []rec.Map, head theoremHead, label string) string {
	helper := e.theoremHelper(node, head)
	series := e.seriesFor(helper.kind)
	isRestated := dig(node, "semantics", "environment") == "document" || (head.number != nil && series.seen(*head.number))
	directive := seriesDirective{}
	if head.number != nil && !isRestated {
		directive = series.next(*head.number)
	}
	lines := e.theoremDirectives(helper, directive)
	if label != "" && head.number != nil {
		e.labelNumbers[label] = *head.number
	}
	body := e.uprightBody(helper.italic, func() string {
		return e.nested(func() string {
			parts := []string{}
			// The theorem's other labels and its head paragraph's ("Claim 8.2.6." holds the
			// \label) are link targets at the body's start.
			anchors := e.anchorsOf(slices.Concat(e.labelsByTarget[toS(node["id"])], e.labelsByTarget[toS(first["id"])]), label)
			if text := joinPresent(" ", anchors, rec.Strip(e.inline(content(first), first, head.skip, false))); text != "" {
				parts = append(parts, text)
			}
			for _, child := range rest {
				if output := e.renderNode(toS(child["id"])); output != "" {
					parts = append(parts, output)
				}
			}
			return strings.Join(parts, "\n\n")
		})
	})
	args := theoremArguments(head, directive, isRestated)
	call := "#" + helper.name
	if len(args) > 0 {
		call += "(" + strings.Join(args, ", ") + ")"
	}
	lines = append(lines, call+"["+body+"]"+e.labelSuffix(label, head.number != nil, head.number))
	return strings.Join(lines, "\n")
}

// uprightBody: a theorem helper sets its body in italics: the class's italics are not the
// author's markup.
func (e *exporter) uprightBody(active bool, render func() string) string {
	saved := e.upright
	e.upright = active || saved
	defer func() { e.upright = saved }()
	return render()
}

func theoremArguments(head theoremHead, directive seriesDirective, isRestated bool) []string {
	args := []string{}
	if head.title != nil {
		args = append(args, "title: ["+*head.title+"]")
	}
	if head.number == nil {
		args = append(args, "number: none")
	}
	if directive.override != nil {
		args = append(args, "number: "+typstString(*directive.override))
	}
	// A restatement shows its theorem's number again without counting.
	if isRestated {
		args = append(args, "number: "+typstString(toS(derefOrNil(head.number)))+", restated: true")
	}
	return args
}

func derefOrNil(text *string) any {
	if text == nil {
		return nil
	}
	return *text
}

// elementLabel: the element's label: its own, or one set on a block inside it that prints
// its number.
func (e *exporter) elementLabel(node rec.Map, number *string) string {
	if own := e.ownLabel(node); own != "" {
		return own
	}
	found := ""
	e.walk(toS(node["id"]), func(inner rec.Map) {
		if found != "" || number == nil {
			return
		}
		for _, name := range e.labelsByTarget[toS(inner["id"])] {
			if e.labelNumber(name) == *number {
				found = name
				return
			}
		}
	})
	if found != "" {
		e.claimedLabels[found] = true
	}
	return found
}

// theoremHead is "Theorem 1.3 (Key)." as supplement, number, note, and the characters it
// takes.
type theoremHead struct {
	supplement string
	number     *string
	title      *string
	skip       int
	italic     bool
}

func (e *exporter) theoremHead(node, first rec.Map) (theoremHead, bool) {
	if first == nil || first["type"] != "paragraph" {
		return theoremHead{}, false
	}
	text := marked(first)
	title, ok := dig(node, "semantics", "title").(string)
	if !ok || !truthy(dig(node, "semantics", "title")) {
		title, ok = theoremWord(text)
		if !ok {
			return theoremHead{}, false
		}
	}
	pattern := regexp.MustCompile(`(?i)\A[ \t\n\v\f\r]*(` + regexp.QuoteMeta(title) + `)(?:[ \t\n\v\f\r]+([\p{L}\p{N}]+(?:\.[\p{L}\p{N}]+)*))?` +
		`[ \t\n\v\f\r]*(?:` + note + `)?[ \t\n\v\f\r]*[.:]?[ \t\n\v\f\r]*`)
	match := pattern.FindStringSubmatchIndex(text)
	if match == nil {
		return theoremHead{}, false
	}
	head := theoremHead{supplement: text[match[2]:match[3]], skip: runeOffset(text, match[1])}
	if match[4] >= 0 {
		number := text[match[4]:match[5]]
		head.number = &number
	}
	group := 3
	if match[6] < 0 {
		group = 4
	}
	if start := match[2*group]; start >= 0 {
		noted := text[start:match[2*group+1]]
		spans := takeText(dropText(content(first), runeOffset(text, start)), size(noted))
		titled := rec.Strip(e.inline(spans, first, 0, false))
		head.title = &titled
	}
	for _, span := range content(first) {
		shape := dig(span, "style", "shape")
		head.italic = head.italic || (span["type"] == "text" && (shape == "it" || shape == "sl"))
	}
	return head, true
}

// theoremWord is text[/\A\s*(\p{L}[\p{L} ]*?)(?=\s+[\dA-Z]|\s*[.(]|\z)/, 1].
func theoremWord(text string) (string, bool) {
	runes := []rune(text)
	start := leadingSpace(runes)
	if start >= len(runes) || !unicode.IsLetter(runes[start]) {
		return "", false
	}
	for end := start + 1; ; end++ {
		if theoremWordEnds(runes, end) {
			return string(runes[start:end]), true
		}
		if end >= len(runes) || !(unicode.IsLetter(runes[end]) || runes[end] == ' ') {
			return "", false
		}
	}
}

func theoremWordEnds(runes []rune, at int) bool {
	if at == len(runes) {
		return true
	}
	spaces := at
	for spaces < len(runes) && isSpace(runes[spaces]) {
		spaces++
	}
	if spaces == len(runes) {
		return false
	}
	next := runes[spaces]
	if spaces > at && (isDigit(next) || (next >= 'A' && next <= 'Z')) {
		return true
	}
	return next == '.' || next == '('
}

// theoremCounter: environments sharing a LaTeX counter register share a kind; a thm-restate
// restatement (set under a dummy counter) joins the kind of its counter's name.
func (e *exporter) theoremCounter(semantics rec.Map) any {
	if !e.registersReady {
		e.counterRegister = map[string]any{}
		for _, id := range e.sortedIDs() {
			own, ok := e.node(id)["semantics"].(rec.Map)
			if !ok || own["kind"] != "theorem" || slices.Contains(restated, toS(own["environment"])) {
				continue
			}
			key := toS(own["counter"]) + "\x00" + fmt.Sprintf("%T", own["counter"])
			if !truthy(e.counterRegister[key]) {
				e.counterRegister[key] = own["counter_register"]
			}
		}
		e.registersReady = true
	}
	if slices.Contains(restated, toS(semantics["environment"])) {
		key := toS(semantics["counter"]) + "\x00" + fmt.Sprintf("%T", semantics["counter"])
		if found := e.counterRegister[key]; truthy(found) {
			return found
		}
		return semantics["counter_register"]
	}
	if truthy(semantics["counter_register"]) {
		return semantics["counter_register"]
	}
	return semantics["counter"]
}

// helper is a theorem helper: one Typst name per environment, supplement and kind.
type helper struct {
	key        string
	name       string
	kind       string
	supplement string
	italic     bool
	defined    bool
	used       bool
}

func (e *exporter) theoremHelper(node rec.Map, head theoremHead) *helper {
	semantics, _ := node["semantics"].(rec.Map)
	counter := e.theoremCounter(semantics)
	kind := "ltd-thm-" + downcase(head.supplement)
	if truthy(counter) {
		kind = "ltd-thm-" + toS(counter)
	}
	environment := semantics["environment"]
	if slices.Contains(restated, toS(semantics["environment"])) {
		environment = semantics["counter"]
	}
	name := head.supplement
	if truthy(environment) {
		name = toS(environment)
	}
	name = nonIdentifier.ReplaceAllString(downcase(name), "")
	if slices.Contains(reserved, name) || name == "" || isDigit([]rune(name)[0]) {
		name = "env-" + name
	}
	key := name + "\x00" + head.supplement + "\x00" + kind
	for _, existing := range e.theoremKinds {
		if existing.key == key {
			return existing
		}
	}
	created := e.newHelper(key, name, kind, head)
	e.theoremKinds = append(e.theoremKinds, created)
	return created
}

func (e *exporter) newHelper(key, name, kind string, head theoremHead) *helper {
	taken := []string{}
	for _, existing := range e.theoremKinds {
		taken = append(taken, existing.name)
	}
	base := name
	for suffix := 2; slices.Contains(taken, name); suffix++ {
		name = fmt.Sprintf("%s%d", base, suffix)
	}
	return &helper{key: key, name: name, kind: kind, supplement: head.supplement, italic: head.italic}
}

// theoremDirectives: a helper is defined where it is first used, and every helper of a kind
// again where the kind's numbering pattern changes: environments sharing a LaTeX counter
// share it.
func (e *exporter) theoremDirectives(current *helper, directive seriesDirective) []string {
	kind := current.kind
	if directive.mode != nil {
		e.kindNumbering[kind] = e.seriesFor(kind).numbering(*directive.mode)
		if directive.mode.kind == "sectioned" {
			e.resetBySection("counter(figure.where(kind: " + typstString(kind) + "))")
		}
		for _, other := range e.theoremKinds {
			if other.kind == kind {
				other.defined = false
			}
		}
	}
	lines := []string{}
	for _, other := range e.theoremKinds {
		if other.kind == kind && !other.defined && (other == current || other.used) {
			lines = append(lines, e.scoped(e.theoremDefinition(other)))
		}
	}
	current.used = true
	if directive.update != nil {
		lines = append(lines, "#counter(figure.where(kind: "+typstString(kind)+")).update("+directive.update.String()+")")
	}
	return lines
}

func (e *exporter) theoremDefinition(current *helper) string {
	current.defined = true
	style := ", body-style: it => it"
	if current.italic {
		style = ""
	}
	numbering, ok := e.kindNumbering[current.kind]
	if !ok {
		numbering = `"1"`
	}
	return "#let " + current.name + " = ltd-theorem(" + typstString(current.kind) + ", [" + escape(current.supplement) + "], " +
		"numbering: " + numbering + style + ")"
}

func (e *exporter) proof(node rec.Map) string {
	parts := e.nodesOf(node)
	if len(parts) == 0 {
		return "#proof[" + e.nested(func() string { return e.children(node) }) + "]"
	}
	first, rest := parts[0], parts[1:]
	if _, ok := first["content"]; !ok {
		return "#proof[" + e.nested(func() string { return e.children(node) }) + "]"
	}
	text := marked(first)
	head := proofHead.FindStringSubmatchIndex(text)
	skip := 0
	if head != nil {
		skip = runeOffset(text, head[1])
	}
	outputs := e.nestedList(func() []string {
		outputs := []string{rec.Strip(e.inline(content(first), first, skip, false))}
		for _, child := range rest {
			outputs = append(outputs, e.renderNode(toS(child["id"])))
		}
		return outputs
	})
	body := proofEnd.ReplaceAllString(joinPresent("\n\n", outputs...), "")
	title := ""
	if head != nil && text[head[2]:head[3]] != "Proof" {
		title = "(title: [" + escape(text[head[2]:head[3]]) + "])"
	}
	return "#proof" + title + "[" + body + "]"
}

func (e *exporter) nestedList(render func() []string) []string {
	e.scopeDepth++
	defer func() { e.scopeDepth-- }()
	return render()
}

// ---- lists ----

type itemLabelForm struct {
	open, close string
	kind        numeralKind
	value       value
}

func (e *exporter) list(node rec.Map) string {
	ordered := node["type"] == "ordered_list"
	type rendered struct {
		body  string
		label *itemLabelForm
	}
	items := []rendered{}
	e.nestedList(func() []string {
		for _, item := range e.nodesOf(node) {
			body, label := e.listItem(item, ordered)
			items = append(items, rendered{body, label})
		}
		return nil
	})
	bodies := make([]string, len(items))
	for index, item := range items {
		bodies[index] = "[" + item.body + "]"
	}
	joined := strings.Join(bodies, ", ")
	if !ordered {
		return "#list(" + joined + ")"
	}
	pattern, start := `"1."`, "1"
	if len(items) > 0 && items[0].label != nil {
		first := items[0].label
		pattern = typstString(first.open + patterns[first.kind] + first.close)
		start = first.value.String()
	}
	return "#enum(numbering: " + pattern + ", start: " + start + ", " + joined + ")"
}

// listItem: the item's content without its printed label, and that label's form.
func (e *exporter) listItem(item rec.Map, ordered bool) (string, *itemLabelForm) {
	parts := e.nodesOf(item)
	if len(parts) == 0 || parts[0]["type"] != "paragraph" {
		return e.children(item), nil
	}
	first, rest := parts[0], parts[1:]
	text := marked(first)
	pattern := bullet
	if ordered {
		pattern = itemLabel
	}
	match := pattern.FindStringSubmatchIndex(text)
	var label *itemLabelForm
	if ordered && match != nil {
		label = labelForm(text, match)
	}
	skip := 0
	if match != nil && (label != nil || !ordered) {
		skip = runeOffset(text, match[1])
	}
	lead := joinPresent(" ", e.anchors(item), e.anchors(first), rec.Strip(e.inline(content(first), first, skip, false)))
	outputs := []string{lead}
	for _, child := range rest {
		outputs = append(outputs, e.renderNode(toS(child["id"])))
	}
	return joinPresent("\n\n", outputs...), label
}

func labelForm(text string, match []int) *itemLabelForm {
	found, ok := parseComponent(text[match[4]:match[5]])
	if !ok {
		return nil
	}
	form := &itemLabelForm{open: text[match[2]:match[3]], kind: found.kind, value: found.value}
	if match[6] >= 0 {
		form.close = text[match[6]:match[7]]
	}
	return form
}

// ---- floats and tables ----

func (e *exporter) float(node rec.Map) string {
	parts := e.nodesOf(node)
	var caption rec.Map
	for index := len(parts) - 1; index >= 0; index-- {
		if parts[index]["type"] == "float_caption" {
			caption = parts[index]
			break
		}
	}
	if caption == nil {
		return e.uncaptionedFloat(node, parts)
	}
	textNode := e.node(rec.Index(rec.FetchList(caption, "children"), 0))
	text := marked(textNode)
	match := captionHead.FindStringSubmatchIndex(text)
	if match == nil {
		match = bareCaptionHead(textNode, text)
	}
	if match == nil {
		return e.children(node)
	}
	supplement, number, separator := text[match[2]:match[3]], text[match[4]:match[5]], text[match[6]:match[7]]
	kind := e.floatKind(node)
	figure := e.floatBody(parts, toS(caption["id"]))
	ownSupplement := e.floatStyle(kind, supplement, separator)
	directive := e.seriesFor(kind).next(number)
	lines := e.floatDirectives(kind, directive)
	label := e.ownLabel(node)
	for _, owner := range []rec.Map{caption, textNode} {
		if names := e.labelsByTarget[toS(owner["id"])]; label == "" && len(names) > 0 {
			label = names[0]
		}
	}
	if label != "" {
		e.labelNumbers[label] = number
	}
	captionText := e.nested(func() string {
		return rec.Strip(e.inline(content(textNode), textNode, runeOffset(text, match[1]), false))
	})
	numbering := ownSupplement
	if directive.override != nil {
		numbering = ", numbering: _ => " + typstString(*directive.override)
	}
	lines = append(lines, "#figure(\n  "+figure+",\n  caption: ["+captionText+"],\n  kind: "+kind+numbering+",\n)"+
		e.labelSuffix(label, true, &number))
	return strings.Join(lines, "\n")
}

func (e *exporter) floatKind(node rec.Map) string {
	if node["type"] == "table_float" {
		return "table"
	}
	return "image"
}

func (e *exporter) floatBody(parts []rec.Map, skipID string) string {
	e.floatDepth++
	defer func() { e.floatDepth-- }()
	body := e.nested(func() string {
		outputs := []string{}
		for _, part := range parts {
			if skipID != "" && toS(part["id"]) == skipID {
				continue
			}
			if output := e.renderNode(toS(part["id"])); output != "" {
				outputs = append(outputs, output)
			}
		}
		return strings.Join(outputs, "\n")
	})
	if body == "" {
		return "[]"
	}
	return "[" + body + "]"
}

func (e *exporter) uncaptionedFloat(node rec.Map, parts []rec.Map) string {
	kind := e.floatKind(node)
	figure := e.floatBody(parts, "")
	label := e.ownLabel(node)
	suffix := e.labelSuffix(label, false, nil)
	if extra := e.anchorsOf(e.labelsByTarget[toS(node["id"])], label); extra != "" {
		suffix += "\n" + extra
	}
	return "#figure(\n  " + figure + ",\n  kind: " + kind + ", numbering: none,\n)" + suffix
}

// floatStyle: the kind's first float sets its supplement and separator; a later float printing
// another word (an algorithm float among the figures) names its own: the figure's argument.
func (e *exporter) floatStyle(kind, supplement, separator string) string {
	index := slices.IndexFunc(e.floatStyles, func(style floatStyle) bool { return style.kind == kind })
	if index < 0 {
		e.floatStyles = append(e.floatStyles, floatStyle{kind, supplement, separator + " "})
		return ""
	}
	if e.floatStyles[index].supplement == supplement {
		return ""
	}
	return ", supplement: [" + escape(supplement) + "]"
}

// bareCaptionHead: a caption head with no separator after it, in captionHead's match layout
// with an empty separator: the caption's whole text, or its first span set in bold with text
// going on after a space ("Fig. 9" then the caption: Springer's and Elsevier's styles).
func bareCaptionHead(textNode rec.Map, text string) []int {
	// A caption that is its head alone (an empty \caption{}: "Figure 2", papers4/1108).
	if whole := boldHead.FindStringSubmatchIndex(text); whole != nil {
		return []int{0, len(text), whole[2], whole[3], whole[4], whole[5], whole[5], whole[5]}
	}
	spans := content(textNode)
	if len(spans) < 2 || spans[0]["type"] != "text" || !strings.HasPrefix(toS(dig(spans[0], "style", "series")), "b") {
		return nil
	}
	head := rec.FetchString(spans[0], "text")
	match := boldHead.FindStringSubmatchIndex(head)
	if match == nil || !strings.HasPrefix(text, head) {
		return nil
	}
	rest := text[len(head):]
	trimmed := strings.TrimLeft(rest, " \t\n\v\f\r")
	spaced := len(trimmed) < len(rest) || strings.TrimRight(head, " \t\n\v\f\r") != head
	if trimmed == "" || !spaced {
		return nil
	}
	end := len(text) - len(trimmed)
	return []int{0, end, match[2], match[3], match[4], match[5], match[5], match[5]}
}

func (e *exporter) floatDirectives(kind string, directive seriesDirective) []string {
	lines := []string{}
	if directive.mode != nil {
		lines = append(lines, e.scoped("#show figure.where(kind: "+kind+"): set figure(numbering: "+
			e.seriesFor(kind).numbering(*directive.mode)+")"))
		if directive.mode.kind == "sectioned" {
			e.resetBySection("counter(figure.where(kind: " + kind + "))")
		}
	}
	if directive.update != nil {
		lines = append(lines, "#counter(figure.where(kind: "+kind+")).update("+directive.update.String()+")")
	}
	return lines
}
