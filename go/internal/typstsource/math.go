package typstsource

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// mathNames is TypstSourceMath::NAMES: symbol names checked against the installed Typst; any
// other character is written as itself, which every Typst version accepts.
var mathNames = map[string]string{
	"α": "alpha", "β": "beta", "γ": "gamma", "δ": "delta", "ε": "epsilon",
	"ϵ": "epsilon.alt", "ζ": "zeta", "η": "eta", "θ": "theta", "ϑ": "theta.alt",
	"ι": "iota", "κ": "kappa", "λ": "lambda", "μ": "mu", "ν": "nu", "ξ": "xi",
	"π": "pi", "ρ": "rho", "ϱ": "rho.alt", "σ": "sigma", "ς": "sigma.alt",
	"τ": "tau", "υ": "upsilon", "φ": "phi", "ϕ": "phi.alt", "χ": "chi",
	"ψ": "psi", "ω": "omega", "Γ": "Gamma", "Δ": "Delta", "Θ": "Theta",
	"Λ": "Lambda", "Ξ": "Xi", "Π": "Pi", "Σ": "Sigma", "Υ": "Upsilon", "Φ": "Phi",
	"Ψ": "Psi", "Ω": "Omega", "∈": "in", "∉": "in.not", "⊂": "subset",
	"⊆": "subset.eq", "⊃": "supset", "⊇": "supset.eq", "∪": "union", "×": "times",
	"⋅": "dot.op", "∑": "sum", "∏": "product", "∫": "integral", "∂": "partial",
	"∇": "nabla", "±": "plus.minus", "∓": "minus.plus", "≈": "approx",
	"∼": "tilde.op", "≡": "equiv", "∀": "forall", "∃": "exists", "¬": "not",
	"∧": "and", "∨": "or", "∘": "compose", "…": "dots.h", "⋯": "dots.c",
	"⋮": "dots.v", "⋱": "dots.down", "∞": "oo", "∅": "emptyset", "ℓ": "ell",
	"→": "->", "←": "<-", "↔": "<->", "⇒": "=>", "⇐": "arrow.l.double",
	"⇔": "<=>", "↦": "|->", "↑": "arrow.t", "↓": "arrow.b", "∥": "parallel",
	"∖": "without", "⟨": "chevron.l", "⟩": "chevron.r", "⌊": "floor.l",
	"⌋": "floor.r", "⌈": "ceil.l", "⌉": "ceil.r", "≤": "<=", "≥": ">=",
	"≠": "!=", "≪": "<<", "≫": ">>", "≺": "prec", "≻": "succ", "⊢": "tack.r",
	"⋃": "union.big", "⊔": "union.sq", "⋀": "and.big",
	"⋁": "or.big", "†": "dagger", "‡": "dagger.double", "⋆": "star.op",
	"∗": "ast.op", "•": "bullet", "⊤": "top", "⊥": "bot", "≲": "lt.tilde",
	"≳": "gt.tilde", "−": "-", "‖": "||",
}

// mathEscaped are the characters with a meaning of their own in Typst math.
const mathEscaped = "\\#$\"_^&/@';"

var (
	accents      = map[string]string{"^": "hat", "ˆ": "hat", "˜": "tilde", "~": "tilde", "¯": "macron", "‾": "overline", "→": "arrow", "˙": "dot", "¨": "dot.double", "˘": "breve", "ˇ": "caron", "⏞": "overbrace"}
	underAccents = map[string]string{"_": "underline", "‾": "underline", "⏟": "underbrace"}
	operators    = []string{"arccos", "arcsin", "arctan", "arg", "cos", "cosh", "cot", "coth", "csc", "deg", "det", "dim", "exp",
		"gcd", "hom", "inf", "ker", "lg", "lim", "liminf", "limsup", "ln", "log", "max", "min", "mod", "sec", "sin", "sinh",
		"sup", "tan", "tanh", "tr"}
	large = []string{"∑", "∏", "⋃", "⋂", "⋀", "⋁", "⨁", "⨂", "⨆", "∐", "lim", "max", "min", "sup", "inf", "liminf", "limsup"}
	// alphabets: mathematical alphanumeric blocks (U+1D400...) by style.
	alphabets = []struct {
		kind  string
		first rune
	}{{"bold", 0x1D400}, {"", 0x1D434}, {"bold", 0x1D468}, {"cal", 0x1D49C}, {"cal", 0x1D4D0},
		{"frak", 0x1D504}, {"bb", 0x1D538}, {"frak", 0x1D56C}, {"sans", 0x1D5A0}, {"sans", 0x1D5D4},
		{"sans", 0x1D608}, {"sans", 0x1D63C}, {"mono", 0x1D670}}
	letterlike = map[string][2]string{"ℝ": {"bb", "R"}, "ℕ": {"bb", "N"}, "ℤ": {"bb", "Z"}, "ℚ": {"bb", "Q"},
		"ℂ": {"bb", "C"}, "ℍ": {"bb", "H"}, "ℙ": {"bb", "P"}, "ℎ": {"", "h"},
		"ℬ": {"cal", "B"}, "ℰ": {"cal", "E"}, "ℱ": {"cal", "F"}, "ℋ": {"cal", "H"},
		"ℐ": {"cal", "I"}, "ℒ": {"cal", "L"}, "ℳ": {"cal", "M"}, "ℛ": {"cal", "R"},
		"ℭ": {"frak", "C"}, "ℌ": {"frak", "H"}, "ℑ": {"frak", "I"}, "ℜ": {"frak", "R"},
		"ℨ": {"frak", "Z"}}
)

const greekItalic = 0x1D6E2

var (
	operatorToken = regexp.MustCompile(`\A"([a-z]+)"\z`)
	wordToken     = regexp.MustCompile(`\A(?:upright\(([\p{L}\p{N}])\)|"([^"\\]*)")\z`)
	quotedToken   = regexp.MustCompile(`\A"([^"\\]*)"\z`)
	numberText    = regexp.MustCompile(`\A[0-9.]+\z`)
	asciiAlnum    = regexp.MustCompile(`\A[A-Za-z0-9]\z`)
	asciiLetter   = regexp.MustCompile(`\A[A-Za-z]\z`)
	primes        = regexp.MustCompile(`\A(?:prime ?)+\z`)
	closer        = regexp.MustCompile(`\A(?:[)\]}|]|\|\||\\.)\z`)
	atomText      = regexp.MustCompile(`\A(?:[\p{L}\p{N}.']+|[a-z.]+\([^()]*\)|"[^"]*")\z`)
	callStart     = regexp.MustCompile(`\A[a-z.]+\(`)
	single        = regexp.MustCompile(`\A[\p{L}\p{N}]\z|\A[0-9]+\z`)
)

// mathWriter is TypstSourceMath: MathML (as LuaMML writes it) to the Typst math an author
// would type: `cal(C) subset RR^(n+1)`, not a layout reproduction. Unsupported markup is a
// failure; the caller degrades.
type mathWriter struct {
	display bool
}

func parseMath(mathml string) *xmltree.Node {
	root, err := xmltree.Parse(mathml)
	if err != nil {
		return nil
	}
	return root
}

// convert is TypstSourceMath#convert.
func (m *mathWriter) convert(mathml string) string {
	root := parseMath(mathml)
	if root == nil || root.Name != "math" {
		rec.Fail("Unsupported MathML root")
	}
	m.display = root.Attrs["display"] == "block"
	return m.sequence(root.Elements())
}

// rows is TypstSourceMath#rows: an alignment's rows, each row's cells without its tag;
// false when the MathML is no single table.
func (m *mathWriter) rows(mathml string) ([][]string, bool) {
	root := parseMath(mathml)
	if root == nil {
		return nil, false
	}
	children := root.Elements()
	index := slices.IndexFunc(children, func(child *xmltree.Node) bool { return child.Name == "mtable" })
	if len(children) != 1 || index < 0 {
		return nil, false
	}
	m.display = true
	result := [][]string{}
	for _, row := range children[index].Elements() {
		cells := []string{}
		for _, cell := range row.Elements() {
			if intent, ok := cell.Attrs["intent"]; ok && intent == ":equation-label" {
				continue
			}
			cells = append(cells, m.sequence(cell.Elements()))
		}
		result = append(result, cells)
	}
	return result, true
}

func (m *mathWriter) sequence(nodes []*xmltree.Node) string {
	tokens := []string{}
	for index := 0; index < len(nodes); {
		if consumed, text := m.delimitedTable(nodes, index); consumed > 0 {
			tokens = append(tokens, text)
			index += consumed
			continue
		}
		tokens = append(tokens, m.node(nodes[index]))
		index++
	}
	present := []string{}
	for _, token := range tokens {
		if token != "" {
			present = append(present, token)
		}
	}
	merged := mergeWords(present)
	for index, token := range merged {
		merged[index] = operatorWord(token)
	}
	return strings.Join(merged, " ")
}

// operatorWord: an operator name set letter by letter (\log in some fonts) is the operator.
func operatorWord(token string) string {
	if match := operatorToken.FindStringSubmatch(token); match != nil && slices.Contains(operators, match[1]) {
		return match[1]
	}
	return token
}

// mergeWords: \mathrm{if} or a URL arrives letter by letter: adjacent upright letters and
// quoted pieces are one word.
func mergeWords(tokens []string) []string {
	merged := []string{}
	for _, token := range tokens {
		match := wordToken.FindStringSubmatch(token)
		if match == nil {
			merged = append(merged, token)
			continue
		}
		word := match[2]
		if strings.HasPrefix(token, "upright(") {
			word = match[1]
		}
		if len(merged) > 0 {
			if previous := quotedToken.FindStringSubmatch(merged[len(merged)-1]); previous != nil {
				merged[len(merged)-1] = `"` + previous[1] + word + `"`
				continue
			}
		}
		if strings.HasPrefix(token, "upright(") {
			merged = append(merged, `"`+word+`"`)
		} else {
			merged = append(merged, token)
		}
	}
	return merged
}

// delimitedTable: (matrix), [matrix], {cases written as a brace and an array.
func (m *mathWriter) delimitedTable(nodes []*xmltree.Node, index int) (int, string) {
	if index+1 >= len(nodes) || nodes[index].Name != "mo" || nodes[index+1].Name != "mtable" {
		return 0, ""
	}
	open, table := nodes[index], nodes[index+1]
	var closing *xmltree.Node
	if index+2 < len(nodes) {
		closing = nodes[index+2]
	}
	if open.Content() == "{" && (closing == nil || closing.Name != "mo") {
		return 2, "cases(" + m.tableRows(table, " & ", ", ") + ")"
	}
	if closing == nil || closing.Name != "mo" {
		return 0, ""
	}
	delimiter, ok := map[string]string{"(": `"("`, "[": `"["`, "|": `"|"`, "‖": `"||"`, "{": `"{"`}[open.Content()]
	if !ok {
		return 0, ""
	}
	return 3, "mat(delim: " + delimiter + ", " + m.tableRows(table, ", ", "; ") + ")"
}

func (m *mathWriter) tableRows(table *xmltree.Node, cellSeparator, rowSeparator string) string {
	rows := []string{}
	for _, row := range table.Elements() {
		cells := []string{}
		for _, cell := range row.Elements() {
			cells = append(cells, argument(m.sequence(cell.Elements())))
		}
		rows = append(rows, strings.Join(cells, cellSeparator))
	}
	return strings.Join(rows, rowSeparator)
}

func (m *mathWriter) node(element *xmltree.Node) string {
	switch element.Name {
	case "mi":
		return m.identifier(element)
	case "mn":
		return m.number(element)
	case "mo":
		return operator(element.Content())
	case "mtext":
		return mathText(element.Content())
	case "mrow", "mpadded":
		return m.sequence(element.Elements())
	case "mstyle":
		return m.style(element)
	case "mspace":
		return space(element)
	case "msub", "msup", "msubsup":
		return m.scripts(element)
	case "munder", "mover", "munderover":
		return m.limits(element)
	case "mfrac":
		return "frac(" + argument(m.child(element, 0)) + ", " + argument(m.child(element, 1)) + ")"
	case "msqrt":
		return "sqrt(" + argument(m.sequence(element.Elements())) + ")"
	case "mtable":
		return "mat(delim: #none, " + m.tableRows(element, ", ", "; ") + ")"
	}
	rec.Fail("Unsupported MathML element %s", element.Name)
	return ""
}

func (m *mathWriter) child(element *xmltree.Node, index int) string {
	children := element.Elements()
	if index >= len(children) {
		rec.Fail("Incomplete MathML %s", element.Name)
	}
	return m.node(children[index])
}

func (m *mathWriter) style(element *xmltree.Node) string {
	inner := m.sequence(element.Elements())
	style, ok := element.Attrs["displaystyle"]
	if !ok || (style == "true") == m.display {
		return inner
	}
	if style == "true" {
		return "display(" + argument(inner) + ")"
	}
	return "inline(" + argument(inner) + ")"
}

func (m *mathWriter) identifier(element *xmltree.Node) string {
	text := rec.Strip(element.Content())
	if text == "" {
		return ""
	}
	upright := element.Attrs["mathvariant"] == "normal"
	letters := 0
	for _, char := range text {
		if letter(string(char)) {
			letters++
		}
	}
	if letters > 1 {
		return lettersOf(text, upright)
	}
	return symbol(text, upright)
}

func (m *mathWriter) number(element *xmltree.Node) string {
	text := rec.Strip(element.Content())
	if numberText.MatchString(text) {
		return text
	}
	return lettersOf(text, true)
}

// lettersOf is TypstSourceMath#letters: a multi-letter identifier: an operator name (sin), a
// styled word (bold("KD")) or upright text.
func lettersOf(text string, upright bool) string {
	styles := [][2]string{}
	kinds := []string{}
	var plain strings.Builder
	for _, char := range text {
		style := decompose(string(char))
		styles = append(styles, style)
		if !slices.Contains(kinds, style[0]) {
			kinds = append(kinds, style[0])
		}
		plain.WriteString(style[1])
	}
	word := plain.String()
	unstyled := len(kinds) == 1 && kinds[0] == ""
	switch {
	case unstyled && slices.Contains(operators, word):
		return word
	case unstyled && slices.Contains([]string{"Re", "Im", "Pr"}, word):
		return "op(" + quote(word) + ")"
	case unstyled && (upright || size(word) > 1):
		return quote(word)
	case styledKinds(kinds) == 1 && kinds[0] != "":
		// Array#one? counts the styled kinds: a word whose first letter is styled takes its style.
		return kinds[0] + "(" + quote(word) + ")"
	}
	parts := make([]string, len(styles))
	for index, style := range styles {
		if style[0] != "" {
			parts[index] = style[0] + "(" + style[1] + ")"
		} else {
			parts[index] = symbol(style[1], upright)
		}
	}
	return strings.Join(parts, " ")
}

func styledKinds(kinds []string) int {
	count := 0
	for _, kind := range kinds {
		if kind != "" {
			count++
		}
	}
	return count
}

func symbol(char string, upright bool) string {
	style := decompose(char)
	kind, base := style[0], style[1]
	name, named := mathNames[base]
	if !named {
		name = base
	}
	if !named && !asciiAlnum.MatchString(base) {
		name = mathEscape(name)
	}
	if upright && kind == "" && asciiLetter.MatchString(base) {
		name = "upright(" + name + ")"
	}
	if kind != "" {
		return kind + "(" + name + ")"
	}
	return name
}

// decompose: a math alphanumeric character as [style function or "", plain character].
func decompose(char string) [2]string {
	if style, ok := letterlike[char]; ok {
		return style
	}
	code := []rune(char)[0]
	if code >= 0x1D400 && code <= 0x1D6A3 {
		index := len(alphabets) - 1
		for alphabets[index].first > code {
			index--
		}
		offset := code - alphabets[index].first
		if offset < 26 {
			return [2]string{alphabets[index].kind, string('A' + offset)}
		}
		return [2]string{alphabets[index].kind, string('a' + offset - 26)}
	}
	if code >= greekItalic && code <= greekItalic+57 {
		return [2]string{"", greek(code)}
	}
	if code >= 0x1D7CE && code <= 0x1D7D7 {
		return [2]string{"bold", string(code - 0x1D7CE + '0')}
	}
	return [2]string{"", char}
}

func greek(code rune) string {
	offset := code - greekItalic
	if offset < 25 {
		return string(0x391 + offset)
	}
	return string(0x3B1 + offset - 26)
}

func letter(char string) bool {
	for _, current := range decompose(char)[1] {
		if unicode.IsLetter(current) {
			return true
		}
	}
	return false
}

func operator(text string) string {
	text = rec.Strip(text)
	if text == "" {
		return ""
	}
	if text == "′" {
		return "prime"
	}
	if name, ok := mathNames[text]; ok {
		return name
	}
	return mathEscape(text)
}

// mathText is TypstSourceMath#text.
func mathText(value string) string {
	value = strings.ReplaceAll(value, "\u00A0", " ")
	if rec.Strip(value) == "" {
		return ""
	}
	return quote(value)
}

func quote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

// mathEscape: bracket pieces (an underbrace LuaMML drew from ⎰ ⎱) are delimiters to Typst too.
func mathEscape(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if strings.ContainsRune(mathEscaped, char) || (char >= 0x239B && char <= 0x23B3) {
			builder.WriteByte('\\')
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

// space: Typst's named spaces by width, in points at a 10pt size.
func space(element *xmltree.Node) string {
	if _, ok := element.Attrs["mathbackground"]; ok {
		return ""
	}
	width := length(element.Attrs["width"])
	switch {
	case width < 1.2:
		return ""
	case width < 2.0:
		return "thin"
	case width < 2.5:
		return "med"
	case width < 6.0:
		return "thick"
	case width < 15:
		return "quad"
	}
	return "wide"
}

var ten = 10.0

func length(text string) float64 {
	number := rubyToF(text)
	if strings.HasSuffix(text, "em") {
		return number * ten
	}
	return number
}

var floatPrefix = regexp.MustCompile(`\A[ \t\n\v\f\r]*[+-]?(?:[0-9]+(?:_[0-9]+)*)?(?:\.[0-9]+(?:_[0-9]+)*)?(?:[eE][+-]?[0-9]+)?`)

// rubyToF is String#to_f: the leading decimal number, 0.0 without one.
func rubyToF(text string) float64 {
	match := strings.ReplaceAll(rec.Strip(floatPrefix.FindString(text)), "_", "")
	if number, err := strconv.ParseFloat(match, 64); err == nil {
		return number
	}
	// A mantissa without digits before its exponent ("e5", ".e1") is no number.
	for end := len(match); end > 0; end-- {
		if number, err := strconv.ParseFloat(match[:end], 64); err == nil {
			return number
		}
	}
	return 0
}

func (m *mathWriter) scripts(element *xmltree.Node) string {
	children := element.Elements()
	expected := 1
	if element.Name == "msubsup" {
		expected = 2
	}
	if len(children) == 0 || len(children)-1 != expected {
		rec.Fail("Incomplete MathML %s", element.Name)
	}
	base, rest := children[0], children[1:]
	var sub, sup string
	if element.Name != "msup" {
		sub = m.node(rest[0])
	}
	if element.Name != "msub" {
		sup = m.node(rest[len(rest)-1])
	}
	if sup != "" && primes.MatchString(sup) {
		return m.node(base) + strings.Repeat("'", len(fields(sup)))
	}
	return attach(m.node(base), sub, sup)
}

func attach(base, sub, sup string) string {
	if base == "" {
		base = `""`
	}
	if !atom(base) {
		parts := []string{}
		if sub != "" {
			parts = append(parts, "b: "+argument(sub))
		}
		if sup != "" {
			parts = append(parts, "t: "+argument(sup))
		}
		return "attach(" + argument(base) + ", " + strings.Join(parts, ", ") + ")"
	}
	return attachLimits(base, sub, sup)
}

func (m *mathWriter) limits(element *xmltree.Node) string {
	children := element.Elements()
	expected := 1
	if element.Name == "munderover" {
		expected = 2
	}
	if len(children) == 0 || len(children)-1 != expected {
		rec.Fail("Incomplete MathML %s", element.Name)
	}
	base, rest := children[0], children[1:]
	var under, over *xmltree.Node
	if element.Name != "mover" {
		under = rest[0]
	}
	if element.Name != "munder" {
		over = rest[len(rest)-1]
	}
	if accent, ok := m.accented(base, under, over); ok {
		return accent
	}
	if under != nil && m.node(under) == "" {
		under = nil
	}
	if over != nil && m.node(over) == "" {
		over = nil
	}
	inner := m.node(base)
	isLarge := slices.Contains(large, rec.Strip(base.Content())) || slices.Contains(operators, inner)
	if !isLarge || !atom(inner) {
		inner = "limits(" + argument(inner) + ")"
	}
	if !atom(inner) {
		inner = "(" + inner + ")"
	}
	var underText, overText string
	if under != nil {
		underText = m.node(under)
	}
	if over != nil {
		overText = m.node(over)
	}
	return attachLimits(inner, underText, overText)
}

func attachLimits(base, under, over string) string {
	result := base
	if under != "" {
		result += "_" + group(under)
	}
	if over != "" {
		result += "^" + group(over)
	}
	return result
}

func (m *mathWriter) accented(base, under, over *xmltree.Node) (string, bool) {
	mark := over
	if mark == nil {
		mark = under
	}
	if mark == nil || (under != nil && over != nil) {
		return "", false
	}
	text := rec.Strip(mark.Content())
	table := underAccents
	if over != nil {
		table = accents
	}
	name, ok := table[text]
	if !ok || (mark.Name != "mo" && mark.Name != "mi") {
		return "", false
	}
	return name + "(" + argument(m.node(base)) + ")", true
}

func atom(text string) bool {
	return text == `""` || closer.MatchString(text) || atomText.MatchString(text) || balancedGroup(text)
}

func balancedGroup(text string) bool {
	if !callStart.MatchString(text) || !strings.HasSuffix(text, ")") {
		return false
	}
	depth := 0
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && index < len(text)-1 {
			return false
		}
	}
	return true
}

func group(text string) string {
	if single.MatchString(text) || balancedGroup(text) {
		return text
	}
	return "(" + balance(text) + ")"
}

// argument: a function argument: its top-level commas and semicolons are content, not
// separators.
func argument(text string) string {
	runes := []rune(balance(text))
	if len(runes) == 0 {
		return `""`
	}
	var builder strings.Builder
	depth := 0
	quoted := false
	for index, char := range runes {
		escaped := index > 0 && runes[index-1] == '\\'
		if char == '"' && !escaped {
			quoted = !quoted
		}
		if !quoted && !escaped && strings.ContainsRune("([{", char) {
			depth++
		}
		if !quoted && !escaped && strings.ContainsRune(")]}", char) {
			depth--
		}
		if depth == 0 && !quoted && strings.ContainsRune(",;", char) && !escaped {
			builder.WriteByte('\\')
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

// balance: delimiters without a partner (a half-open interval [0, 1)) are matched across
// kinds, as Typst pairs them; any left over is escaped so it cannot close a function call.
func balance(text string) string {
	runes := []rune(text)
	open := []int{}
	unmatched := []int{}
	quoted := false
	for index, char := range runes {
		if index > 0 && runes[index-1] == '\\' {
			continue
		}
		if char == '"' {
			quoted = !quoted
		}
		if quoted {
			continue
		}
		if strings.ContainsRune("([{", char) {
			open = append(open, index)
		} else if strings.ContainsRune(")]}", char) {
			if len(open) == 0 {
				unmatched = append(unmatched, index)
			} else {
				open = open[:len(open)-1]
			}
		}
	}
	marks := append(unmatched, open...)
	slices.Sort(marks)
	for index := len(marks) - 1; index >= 0; index-- {
		at := marks[index]
		runes = append(runes[:at], append([]rune{'\\'}, runes[at:]...)...)
	}
	return string(runes)
}
