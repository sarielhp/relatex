package formulatex

import (
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/xmltree"
)

// closingDots: the thin space amsmath's \dots and \cdots end a formula with.
func closingDots(elements []*xmltree.Node, index int) bool {
	if index != len(elements)-1 || index == 0 {
		return false
	}
	_, ok := dotsCommand(elements[index-1])
	return ok
}

// lowDots: three dots \ldots sets, at index, when they are not wrapped in an mrow (a formula of
// \ldots alone).
func lowDots(elements []*xmltree.Node, index int) bool {
	if index+2 >= len(elements) {
		return false
	}
	for offset, lspace := range []string{"0", "0.167em", "0.167em"} {
		node := elements[index+offset]
		if node.Name != "mo" || node.Content() != "." || node.Attrs["lspace"] != lspace {
			return false
		}
	}
	return true
}

// wordTeX is a word of upright letters: an operator name, or \mathrm text.
func wordTeX(word string, operator bool) string {
	if operator {
		return operatorName(word)
	}
	// Only \operatorname keeps a hyphen; braced, it is an ordinary atom.
	if strings.Contains(word, "-") {
		return "{" + operatorName(word) + "}"
	}
	return "\\mathrm" + braced(word)
}

// atomKind is the TeX atom class an element stands for, as far as the thin spaces go.
func atomKind(node *xmltree.Node) string {
	switch {
	case node.Name == "mspace" && isGlue(node):
		return "space"
	case node.Name == "mspace":
		// A strut (\rule) is a box, an ordinary atom.
		return "ord"
	case node.Name == "mo":
		// An mo carries its spacing itself, whatever its class.
		return "mo"
	case node.Name == "mpadded":
		return paddedKind(node)
	case node.Name == "mrow" && isWord(node):
		return "word"
	case isOperator(node):
		return "op"
	case isExtensibleArrow(node):
		return "rel"
	case stackedSymbol(node):
		return "symbol"
	case scriptedOperator(node):
		// The operator symbol carries the spacing, as an mo does.
		return "mo"
	case node.Name == "mrow" && isFenced(node):
		return "inner"
	case node.Name == "msub" || node.Name == "msup" || node.Name == "msubsup":
		// Scripts keep their base's class: \left(...\right)^2 is an inner atom.
		if base := node.Elements(); len(base) > 0 && base[0].Name == "mrow" && isFenced(base[0]) {
			return "inner"
		}
		if isOperator(node) {
			return "op"
		}
		return "ord"
	case node.Name == "mrow":
		if _, ok := dotsCommand(node); ok {
			return "inner"
		}
	}
	return "ord"
}

func isWord(node *xmltree.Node) bool {
	_, ok := uprightWord(node)
	return ok
}

func isFenced(node *xmltree.Node) bool {
	elements := node.Elements()
	return len(elements) >= 2 && delimiting(elements[0], elements[len(elements)-1])
}

// isOperator: an operator name, alone or with its limits or scripts.
func isOperator(node *xmltree.Node) bool {
	switch node.Name {
	case "munder", "mover", "munderover", "msub", "msup", "msubsup":
		elements := node.Elements()
		// An accent over a word (\overline{\mathrm{dist}}) is an ordinary atom.
		if node.Name == "mover" && len(elements) == 2 && accents[accentText(elements[1])] != "" {
			return false
		}
		return len(elements) > 0 && isOperator(elements[0])
	}
	_, ok := uprightWord(node)
	return ok
}

// row writes an mrow: a word of upright letters, a \left...\right pair, or its elements.
func row(node *xmltree.Node) string {
	if word, ok := uprightWord(node); ok {
		return wordTeX(word, operatorNames[word])
	}
	if dots, ok := dotsCommand(node); ok {
		return dots
	}
	elements := node.Elements()
	if len(elements) == 0 {
		return "{}"
	}
	environment, width := fencedMatrix(elements, 0)
	if width == 0 && len(elements) >= 2 && delimiting(elements[0], elements[len(elements)-1]) {
		return rowOf(elements)
	}
	// A fenced matrix is its own \left...\right row: braces would make it an ordinary atom.
	if width == len(elements) {
		return environment
	}
	if binomial, ok := binom(elements); ok {
		return binomial
	}
	// A group of several atoms is the author's braces: TeX makes them an mrow again.
	if len(elements) >= 2 {
		return braced(sequence(elements))
	}
	return sequence(elements)
}

// rowOf writes a row's elements: \binom, a \left...\right pair (with \middle fences inside), or
// the sequence.
func rowOf(elements []*xmltree.Node) string {
	if binomial, ok := binom(elements); ok {
		return binomial
	}
	last := len(elements) - 1
	if _, width := fencedMatrix(elements, 0); width > 0 || last < 1 || !delimiting(elements[0], elements[last]) {
		return sequence(elements)
	}
	pieces := []string{"\\left", delimiter(elements[0])}
	start := 1
	for index := 1; index < last; index++ {
		if isFence(elements[index]) {
			pieces = append(pieces, sequence(elements[start:index]), "\\middle", delimiter(elements[index]))
			start = index + 1
		}
	}
	pieces = append(pieces, sequence(elements[start:last]), "\\right", delimiter(elements[last]))
	return join(pieces)
}

// dotsCommand is \ldots or \cdots: an mrow of three dots TeX sets as one inner atom.
func dotsCommand(node *xmltree.Node) (string, bool) {
	elements := node.Elements()
	if node.Name != "mrow" || len(elements) != 3 {
		return "", false
	}
	text := elements[0].Content()
	for _, child := range elements {
		if child.Name != "mo" || child.Content() != text {
			return "", false
		}
	}
	switch text {
	case ".":
		return "\\ldots", true
	case "⋅":
		return "\\cdots", true
	}
	return "", false
}

func isFence(node *xmltree.Node) bool { return node.Name == "mo" && node.Attrs["fence"] == "true" }

// delimiter is a fence's TeX as \left and \right take it; an empty fence is ".".
func delimiter(node *xmltree.Node) string {
	if node.Content() == "" || isNullDelimiter(node) {
		return "."
	}
	return token(node)
}

// scripts writes msub, msup and msubsup; a prime superscript is written as '.
func scripts(node *xmltree.Node) string {
	count := map[string]int{"msub": 2, "msup": 2, "msubsup": 3}[node.Name]
	elements := children(node, count)
	base := scriptBase(elements[0])
	switch node.Name {
	case "msub":
		return base + "_" + argument(elements[1])
	case "msup":
		return base + superscript(elements[1])
	}
	return base + "_" + argument(elements[1]) + superscript(elements[2])
}

func scriptBase(node *xmltree.Node) string {
	if word, ok := uprightWord(node); ok {
		return operatorName(word)
	}
	text := element(node)
	if node.Name == "mrow" && len(node.Elements()) == 0 {
		return "{}"
	}
	if utf8Single(text) || isCommand(text) || node.Name == "mrow" && strings.HasPrefix(text, "\\left") ||
		node.Name == "mrow" && isOperator(node) {
		return text
	}
	return "{" + text + "}"
}

func utf8Single(text string) bool { return len([]rune(text)) == 1 }

// superscript is ^{...}, or primes as '.
func superscript(node *xmltree.Node) string {
	if primes, ok := primeCount(node); ok {
		return strings.Repeat("'", primes)
	}
	return "^" + argument(node)
}

// argument is a script or limit: an operator alone is braced, which keeps it an operator (TeX
// makes an unbraced ^* an ordinary atom).
func argument(node *xmltree.Node) string {
	if node.Name == "mo" {
		return braced(element(node))
	}
	return group(element(node))
}

func primeCount(node *xmltree.Node) (int, bool) {
	if node.Name == "mo" && node.Content() == "′" {
		return 1, true
	}
	if node.Name != "mrow" {
		return 0, false
	}
	elements := node.Elements()
	for _, child := range elements {
		if child.Name != "mo" || child.Content() != "′" {
			return 0, false
		}
	}
	return len(elements), len(elements) > 0
}

// accents are mover's accent characters and their commands.
var accents = map[string]string{"^": "hat", "ˆ": "hat", "¯": "bar", "‾": "overline", "˜": "tilde", "~": "widetilde",
	"→": "vec", "˙": "dot", "¨": "ddot", "ˇ": "check", "˘": "breve", "´": "acute", "`": "grave", "˚": "mathring"}

// limits writes munder, mover and munderover: accents, an operator's limits, else \underset and
// \overset.
func limits(node *xmltree.Node) string {
	count := map[string]int{"munder": 2, "mover": 2, "munderover": 3}[node.Name]
	elements := children(node, count)
	if node.Name == "mover" {
		if command, ok := accents[accentText(elements[1])]; ok {
			return "\\" + command + braced(element(elements[0]))
		}
	}
	if braces, ok := brace(node, elements); ok {
		return braces
	}
	if arrow, ok := extensibleArrow(node, elements); ok {
		return arrow
	}
	if isOperator(elements[0]) || elements[0].Attrs["movablelimits"] != "" {
		return operatorLimits(node.Name, elements)
	}
	return stacked(node.Name, elements)
}

// largeOperators are the operators TeX sets large in a display (\sum, \int, ...); any other mo
// with limits is an atom the author made an operator (\mathop{\to}\limits, \stackrel).
var largeOperators = map[string]bool{"∑": true, "∏": true, "∐": true, "∫": true, "∬": true, "∭": true, "∮": true,
	"∯": true, "∰": true, "⋀": true, "⋁": true, "⋂": true, "⋃": true, "⨀": true, "⨁": true, "⨂": true, "⨄": true,
	"⨆": true, "⨌": true}

// operatorLimits writes an operator with its limits. A character that is not a large operator is
// made one with \mathop, inside \mathrel or \mathbin when its spacing shows that class.
func operatorLimits(name string, elements []*xmltree.Node) string {
	operator := elements[0]
	base := element(operator)
	if operator.Attrs["movablelimits"] == "false" {
		base += "\\limits"
	}
	if word, ok := uprightWord(operator); ok && !operatorNames[word] {
		base = "\\operatorname*" + braced(word)
	}
	wrapper := ""
	if operator.Name == "mo" && !largeOperators[operator.Content()] {
		base = "\\mathop" + braced(element(operator))
		if operator.Attrs["movablelimits"] == "false" {
			base += "\\limits"
		}
		wrapper = map[string]string{"0.278em": "mathrel", "0.222em": "mathbin"}[operator.Attrs["lspace"]]
	}
	switch name {
	case "munder":
		base += "_" + argument(elements[1])
	case "mover":
		base += "^" + argument(elements[1])
	default:
		base += "_" + argument(elements[1]) + "^" + argument(elements[2])
	}
	if wrapper == "" {
		return base
	}
	return "\\" + wrapper + braced(base)
}

// stacked is \underset and \overset: around an operator name, an ordinary atom.
func stacked(name string, elements []*xmltree.Node) string {
	switch name {
	case "munder":
		return "\\underset" + braced(element(elements[1])) + braced(element(elements[0]))
	case "mover":
		return "\\overset" + braced(element(elements[1])) + braced(element(elements[0]))
	}
	return "\\overset" + braced(element(elements[2])) + braced("\\underset"+braced(element(elements[1]))+braced(element(elements[0])))
}

// brace is \overbrace{...}^{...} or \underbrace{...}_{...}: amsmath sets the brace and its body as
// a table of two rows, the brace's row alone.
func brace(node *xmltree.Node, elements []*xmltree.Node) (string, bool) {
	if node.Name == "munderover" || elements[0].Name != "mtable" {
		return "", false
	}
	rows := elements[0].Elements()
	if len(rows) != 2 {
		return "", false
	}
	command, body, mark := "overbrace", rows[1], "^"
	if node.Name == "munder" {
		command, body, mark = "underbrace", rows[0], "_"
	}
	braceRow := rows[0]
	if node.Name == "munder" {
		braceRow = rows[1]
	}
	if text := braceRow.Content(); text != "⏞" && text != "⏟" {
		return "", false
	}
	return "\\" + command + braced(alignedCell(firstCell(body))) + mark + argument(elements[1]), true
}

func firstCell(tableRow *xmltree.Node) *xmltree.Node {
	cells := tableRow.Elements()
	if len(cells) != 1 {
		fail("brace row of %d cells", len(cells))
	}
	return cells[0]
}

func accentText(node *xmltree.Node) string {
	if node.Name != "mo" || len(node.Elements()) > 0 {
		return ""
	}
	return node.Content()
}

// fraction writes \frac, or \binom for a fraction without a rule.
func fraction(node *xmltree.Node) string {
	elements := children(node, 2)
	numerator, denominator := braced(element(elements[0])), braced(element(elements[1]))
	thickness, ruled := node.Attrs["linethickness"]
	if ruled && zeroLength(thickness) {
		return "\\genfrac{}{}{0pt}{}" + numerator + denominator
	}
	return "\\frac" + numerator + denominator
}

// binom is \binom: a fraction without a rule between parentheses, as amsmath's \genfrac sets it.
func binom(elements []*xmltree.Node) (string, bool) {
	if len(elements) != 3 || elements[0].Name != "mo" || elements[0].Content() != "(" || elements[2].Name != "mo" ||
		elements[2].Content() != ")" || elements[1].Name != "mfrac" || !zeroLength(elements[1].Attrs["linethickness"]) {
		return "", false
	}
	parts := children(elements[1], 2)
	return "\\binom" + braced(element(parts[0])) + braced(element(parts[1])), true
}

func zeroLength(text string) bool {
	value, err := strconv.ParseFloat(strings.TrimRight(text, "ptem"), 64)
	return err == nil && value == 0
}

// styles are mstyle's (displaystyle, scriptlevel) and their commands.
var styles = map[[2]string]string{{"true", "0"}: "displaystyle", {"false", "0"}: "textstyle",
	{"false", "1"}: "scriptstyle", {"false", "2"}: "scriptscriptstyle"}

func style(node *xmltree.Node) string {
	command, ok := styles[[2]string{node.Attrs["displaystyle"], node.Attrs["scriptlevel"]}]
	if !ok || len(node.Attrs) != 2 {
		fail("mstyle %v", node.Attrs)
	}
	return braced("\\" + command + " " + sequence(node.Elements()))
}
