package mathexport

import (
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/mathintent"
)

// The constructs each concept typesets, as the page's MathML shows them (decision 109).

// withSelf is the node and its descendant elements in document order.
func withSelf(root *Node) []*Node { return append([]*Node{root}, root.Descendants()...) }

// delimiterRows are the elements whose children a delimiter pair is sought in: every element but a
// sized fence's own wrappers (\bigl| is an mo in an mrow with struts, in two mpadded), whose lone
// delimiter belongs to the row around them (decision 114, amendment 2026-11-13).
func delimiterRows(root *Node) []*Node {
	rows := []*Node{}
	for _, node := range withSelf(root) {
		if node.Name == "mo" || fenceCore(node) == nil {
			rows = append(rows, node)
		}
	}
	return rows
}

// binomials: a fraction without a rule between ( and ), as \binom sets it; the intent goes on a
// new mrow around the three, the arguments are the fraction's two parts.
func binomials(root *Node) []placement {
	found := []placement{}
	for _, row := range withSelf(root) {
		children := row.Elements()
		for index, child := range children {
			if child.Name != "mfrac" || !ruleless(child) || len(child.Elements()) != 2 {
				continue
			}
			open, shut := neighbour(children, index, -1), neighbour(children, index, 1)
			if open == nil || shut == nil || fenceText(open) != "(" || fenceText(shut) != ")" {
				continue
			}
			fraction := child
			found = append(found, func(names *argNames) {
				parts := fraction.Elements()
				intent := call(mathintent.Binomial, "$"+names.of(parts[0]), "$"+names.of(parts[1]))
				setIntent(wrap(row, open, shut), intent)
			})
		}
	}
	return found
}

// ruleless: a linethickness of zero, in any unit.
func ruleless(fraction *Node) bool {
	thickness, ok := fraction.Attribute("linethickness")
	if !ok {
		return false
	}
	number := strings.TrimRight(thickness, "abcdefghijklmnopqrstuvwxyz%")
	value, err := strconv.ParseFloat(number, 64)
	return err == nil && value == 0
}

// neighbour is the nearest sibling in the direction (-1, 1) that is not a space.
func neighbour(children []*Node, index, direction int) *Node {
	for at := index + direction; at >= 0 && at < len(children); at += direction {
		if children[at].Name != "mspace" {
			return children[at]
		}
	}
	return nil
}

// delimited: each pair of the opening and closing delimiter in one row, nested pairs matched; none
// when a delimiter of the formula pairs with nothing.
func delimited(root *Node, opening, closing string, concept mathintent.Concept) []placement {
	found, placed := []placement{}, 0
	for _, row := range delimiterRows(root) {
		open := []*Node{}
		for _, child := range row.Elements() {
			switch fenceText(child) {
			case opening:
				open = append(open, child)
			case closing:
				if len(open) == 0 {
					return nil
				}
				found = append(found, enclosed(row, open[len(open)-1], child, concept))
				open = open[:len(open)-1]
				placed++
			}
		}
	}
	if placed != countOperators(root, opening) || placed != countOperators(root, closing) {
		return nil
	}
	return found
}

// innerProducts: each ⟨ … , … ⟩ of one row with one comma between, none nested; the intent goes on a
// new mrow around the pair, the two sides of the comma are its arguments (decision 114).
func innerProducts(root *Node) []placement {
	return pairedInnerProducts(root, fenceText, commaBetween)
}

// pairedInnerProducts finds the ⟨ … ⟩ pairs whose text is read by text, each holding one separator
// that between finds; the one body of innerProducts (a comma) and brakets (a bar).
func pairedInnerProducts(root *Node, text func(*Node) string, between func([]*Node, int, int) int) []placement {
	found, placed := []placement{}, 0
	for _, row := range delimiterRows(root) {
		children := row.Elements()
		open := -1
		for index, child := range children {
			switch text(child) {
			case "⟨":
				if open >= 0 {
					return nil
				}
				open = index
			case "⟩":
				separator := between(children, open, index)
				if separator < 0 {
					return nil
				}
				found = append(found, innerProduct(row, children[open], children[separator], child))
				open, placed = -1, placed+1
			}
		}
	}
	if placed != countOperators(root, "⟨") || placed != countOperators(root, "⟩") {
		return nil
	}
	return found
}

// commaBetween is the index of the one comma between open and shut, -1 when there is not exactly
// one or a side is empty.
func commaBetween(children []*Node, open, shut int) int {
	comma := -1
	for index := open + 1; open >= 0 && index < shut; index++ {
		if children[index].Name == "mo" && tokenString(children[index]) == "," {
			if comma >= 0 {
				return -1
			}
			comma = index
		}
	}
	if comma == open+1 || comma == shut-1 {
		return -1
	}
	return comma
}

// innerProduct is the placement of one pair: each side its argument (a side of several elements
// gets a new row, so the indices are read again after the first).
func innerProduct(row, open, comma, shut *Node) placement {
	side := func(first, last *Node) []*Node {
		children := row.Elements()
		return slices.Clone(children[slices.Index(children, first)+1 : slices.Index(children, last)])
	}
	return func(names *argNames) {
		left := names.argument(side(open, comma))
		right := names.argument(side(comma, shut))
		setIntent(wrap(row, open, shut), call(mathintent.InnerProduct, left, right))
	}
}

// bars: the bars of one row taken two by two, left to right, as the source (which nests none of
// them, mathintent) wrote them; none when a bar of the formula is left over or sits elsewhere
// (a script's base).
func bars(root *Node, bar string, concept mathintent.Concept) []placement {
	found, placed := []placement{}, 0
	scripts := map[*Node]bool{}
	for _, row := range delimiterRows(root) {
		if scripts[row] {
			continue
		}
		var open *Node
		for _, child := range row.Elements() {
			switch {
			case open != nil && scriptedBarIntent() && scriptedBar(child, bar):
				scripts[child] = true
				found = append(found, enclosedScripted(row, open, child, concept))
				open = nil
				placed += 2
			case barText(child) != bar:
			case open == nil:
				open = child
			default:
				found = append(found, enclosed(row, open, child, concept))
				open = nil
				placed += 2
			}
		}
		if open != nil {
			return nil
		}
	}
	if placed != countOperators(root, bar) {
		return nil
	}
	return found
}

// scriptedBarIntent is the experimental setting RELATEX_SCRIPTED_BAR=1 (plans/status/2026-11-16-a11y-night,
// decision 142 fork): a script on the closing bar of a pair, as TeX sets \norm{x}^2 for \norm = \|#1\|,
// moves the pair into the script's base so the pair gets its intent. Off by default.
func scriptedBarIntent() bool { return os.Getenv("RELATEX_SCRIPTED_BAR") == "1" }

// scriptedBar: a script whose base is the bar.
func scriptedBar(node *Node, bar string) bool {
	if !slices.Contains([]string{"msup", "msub", "msubsup"}, node.Name) {
		return false
	}
	elements := node.Elements()
	return len(elements) > 0 && barText(elements[0]) == bar
}

// enclosedScripted is enclosed for a closing bar that is the base of a script: the opening bar, the
// argument and that base become the script's new base, the row the intent is on.
func enclosedScripted(row, open, script *Node, concept mathintent.Concept) placement {
	return func(names *argNames) {
		children := row.Elements()
		start, stop := slices.Index(children, open), slices.Index(children, script)
		parts := script.Elements()
		if start < 0 || stop <= start+1 || len(parts) < 2 {
			return
		}
		argument := names.argument(slices.Clone(children[start+1 : stop]))
		// A multi-element argument is now one new row of the row: read the children again.
		children = row.Elements()
		start, stop = slices.Index(children, open), slices.Index(children, script)
		base := &Node{Name: "mrow", Parent: script}
		base.setElements(append(slices.Clone(children[start:stop]), parts[0]))
		setIntent(base, call(concept, argument))
		script.setElements(append([]*Node{base}, parts[1:]...))
		row.setElements(append(slices.Clone(children[:start]), append([]*Node{script}, children[stop+1:]...)...))
	}
}

// barText: the delimiter a node shows, or the text of an mi (TeX sets \| and a bare | as ordinary
// symbols, decision 114).
func barText(node *Node) string {
	if node.Name == "mi" {
		return tokenString(node)
	}
	return fenceText(node)
}

// enclosed is the placement of a function of what two delimiters enclose: a new mrow around them
// carries the intent, what lies between is its argument.
func enclosed(row, open, shut *Node, concept mathintent.Concept) placement {
	return func(names *argNames) {
		children := row.Elements()
		start, stop := slices.Index(children, open), slices.Index(children, shut)
		if start < 0 || stop <= start+1 {
			return
		}
		inner := children[start+1 : stop]
		argument := names.argument(slices.Clone(inner))
		setIntent(wrap(row, open, shut), call(concept, argument))
	}
}

// countOperators: the tokens (operators, and identifiers such as an ordinary bar) with the text.
func countOperators(root *Node, text string) int {
	count := 0
	for _, node := range root.Descendants() {
		if (node.Name == "mo" || node.Name == "mi") && tokenString(node) == text {
			count++
		}
	}
	return count
}

// transposeSigns are the superscripts read as a transpose: ⊤ (\top), ⊺ (\intercal), a sans-serif
// 𝖳; an upright T (\mathrm{T}) is an mi with mathvariant="normal"; an italic T only where a macro
// whose body is ^T wrote it (italic).
var transposeSigns = []string{"⊤", "⊺", "𝖳"}

// transposes: a superscript transpose sign on a base that is not an operator. The sign on a closing
// parenthesis or bracket is the transpose of the group it closes (decision 187): TeX sets (AB)^T
// flat, so a new row from the opening fence to the script carries the intent and the text between
// the fences is its argument.
func transposes(root *Node, italic bool) []placement {
	found := []placement{}
	for _, node := range withSelf(root) {
		if node.Name != "msup" || len(node.Elements()) != 2 {
			continue
		}
		base, script := node.Elements()[0], scriptSign(node.Elements()[1])
		if script == nil || !transposeSign(script, italic) {
			continue
		}
		msup := node
		if fenceCore(base) == nil {
			found = append(found, func(names *argNames) {
				setIntent(msup, call(mathintent.Transpose, "$"+names.of(base)))
			})
			continue
		}
		if open := groupOpening(msup); open != nil {
			found = append(found, enclosed(msup.Parent, open, msup, mathintent.Transpose))
		}
	}
	return found
}

// scriptSign is the token a script shows, spaces left out (\mkern before the T of a macro).
func scriptSign(script *Node) *Node {
	parts := elementsOf(script)
	if len(parts) != 1 {
		return nil
	}
	return leaf(parts[0])
}

// groupOpening is the opening fence that the closing fence at the base of script closes, among the
// script's siblings: a parenthesis with a parenthesis, a bracket with a bracket; nil when the row
// does not hold the pair.
func groupOpening(script *Node) *Node {
	closer := fenceText(script.Elements()[0])
	opener, ok := map[string]string{")": "(", "]": "["}[closer]
	if !ok || script.Parent == nil {
		return nil
	}
	children := script.Parent.Elements()
	depth := 1
	for index := slices.Index(children, script) - 1; index >= 0; index-- {
		text := fenceText(scriptBase(children[index]))
		switch {
		case text == ")" || text == "]":
			depth++
		case text == "(" || text == "[":
			depth--
		}
		if depth == 0 {
			if text != opener {
				return nil
			}
			return children[index]
		}
	}
	return nil
}

// scriptBase is a script's base, a node itself otherwise.
func scriptBase(node *Node) *Node {
	if slices.Contains([]string{"msup", "msub", "msubsup"}, node.Name) && len(node.Elements()) > 0 {
		return node.Elements()[0]
	}
	return node
}

func transposeSign(script *Node, italic bool) bool {
	text := tokenString(script)
	if slices.Contains(transposeSigns, text) {
		return true
	}
	if script.Name != "mi" {
		return false
	}
	return text == "T" && script.Get("mathvariant") == "normal" || italic && (text == "T" || text == "𝑇")
}

// inverseFunctions are the trigonometric and hyperbolic names whose ^{-1} is their inverse.
var inverseFunctions = []string{"sin", "cos", "tan", "sec", "csc", "cot", "sinh", "cosh", "tanh", "coth"}

// inverses: a trigonometric function's name with the superscript -1.
func inverses(root *Node) []placement {
	found := []placement{}
	for _, node := range withSelf(root) {
		if node.Name != "msup" || len(node.Elements()) != 2 {
			continue
		}
		base, script := node.Elements()[0], node.Elements()[1]
		name, named := leafText(base, "mi")
		if !named || !slices.Contains(inverseFunctions, name) || !minusOne(script) {
			continue
		}
		msup := node
		found = append(found, func(names *argNames) {
			setIntent(msup, call(mathintent.Inverse, "$"+names.of(base)))
		})
	}
	return found
}

func minusOne(script *Node) bool {
	parts := elementsOf(script)
	if len(parts) != 2 {
		return false
	}
	sign, signed := leafText(parts[0], "mo")
	one, numbered := leafText(parts[1], "mn")
	return signed && numbered && (sign == "−" || sign == "-") && one == "1"
}

// factorials: an exclamation mark after an operand (not an operator other than a closing
// fence), with no second mark after it (n!! is the double factorial).
func factorials(root *Node) []placement {
	found := []placement{}
	for _, row := range withSelf(root) {
		children := row.Elements()
		for index, child := range children {
			if child.Name != "mo" || tokenString(child) != "!" || !factorialOperand(children, index) {
				continue
			}
			if next := neighbour(children, index, 1); next != nil && next.Name == "mo" && tokenString(next) == "!" {
				continue
			}
			mark := child
			found = append(found, func(*argNames) { setIntent(mark, string(mathintent.Factorial)) })
		}
	}
	return found
}

func factorialOperand(children []*Node, index int) bool {
	previous := neighbour(children, index, -1)
	if previous == nil {
		return false
	}
	if previous.Name != "mo" {
		return true
	}
	return slices.Contains([]string{")", "]", "}"}, tokenString(previous))
}

// namedTokens: the identifiers with the text, for a concept that names them (\Pr: probability).
func namedTokens(root *Node, text string) []placement {
	return operatorTokens(root, mathintent.Probability, func(node *Node) bool { return tokenString(node) == text })
}

// operatorTokens: the identifiers a concept's sign shows as, each given the concept.
func operatorTokens(root *Node, concept mathintent.Concept, sign func(*Node) bool) []placement {
	found := []placement{}
	for _, node := range root.Descendants() {
		if node.Name == "mi" && sign(node) {
			name := node
			found = append(found, func(*argNames) { setIntent(name, string(concept)) })
		}
	}
	return found
}

// expectationSign: an E an expectation macro sets (decision 114): blackboard, sans-serif, bold, or
// upright (an italic E is a variable).
func expectationSign(node *Node) bool {
	text := tokenString(node)
	return slices.Contains([]string{"𝔼", "𝖤", "𝐄", "𝗘", "𝙴"}, text) || text == "E" && node.Get("mathvariant") == "normal"
}

// varianceSign: Var (one identifier since decision 100), or a blackboard, sans-serif, bold or
// upright V.
func varianceSign(node *Node) bool {
	text := tokenString(node)
	return slices.Contains([]string{"Var", "var", "VAR", "𝕍", "𝖵", "𝐕"}, text) || text == "V" && node.Get("mathvariant") == "normal"
}

// derivatives: a Leibniz fraction, d (or ∂) with its order and the function over d x (∂ x) with
// theirs, the orders adding up. The intent goes on the fraction: derivative($f,$x,order), or
// partial-derivative($f,$x,order,$y,order...).
//
// Off by default (a11y night run B5a): MathCAT 0.7.7 has no reading of the Core derivative, so it
// speaks the shape as a prefix and its arguments run on, "partial f of X X 1" for
// \frac{\partial f(X)}{\partial X} (papers2/0310), "derivative of f of x comma x comma 1" for
// dy/dx, where the page without intent says "the fraction with numerator ... and denominator ...".
// RELATEX_LEIBNIZ_INTENT=1 writes the shape for a reader that implements it.
func derivatives(root *Node, mark string) []placement {
	if os.Getenv("RELATEX_LEIBNIZ_INTENT") != "1" {
		return nil
	}
	found := []placement{}
	for _, node := range withSelf(root) {
		if node.Name != "mfrac" || len(node.Elements()) != 2 {
			continue
		}
		numerator, denominator := elementsOf(node.Elements()[0]), elementsOf(node.Elements()[1])
		order, ok := differentialOrder(numerator, mark)
		groups, matched := variables(denominator, mark)
		if !ok || !matched || len(numerator) < 2 || sumOrders(groups) != order || mark == "d" && len(groups) != 1 {
			continue
		}
		fraction, function := node, numerator[1:]
		found = append(found, func(names *argNames) {
			arguments := []string{names.argument(function)}
			for _, group := range groups {
				arguments = append(arguments, "$"+names.of(group.variable), strconv.Itoa(group.order))
			}
			concept := mathintent.PartialDerivative
			if mark == "d" {
				concept = mathintent.Derivative
			}
			setIntent(fraction, call(concept, arguments...))
		})
	}
	return found
}

type differentialGroup struct {
	variable *Node
	order    int
}

func sumOrders(groups []differentialGroup) int {
	total := 0
	for _, group := range groups {
		total += group.order
	}
	return total
}

// differentialOrder reads the numerator's leading d (or ∂), alone or with a numeric power.
func differentialOrder(numerator []*Node, mark string) (int, bool) {
	if len(numerator) == 0 {
		return 0, false
	}
	head := numerator[0]
	if differentialMark(head, mark) {
		return 1, true
	}
	if head.Name != "msup" || len(head.Elements()) != 2 || !differentialMark(head.Elements()[0], mark) {
		return 0, false
	}
	return numericPower(head.Elements()[1])
}

// variables reads the denominator as d x, d x^2, or ∂ x ∂ y^2... groups.
func variables(denominator []*Node, mark string) ([]differentialGroup, bool) {
	groups := []differentialGroup{}
	for index := 0; index < len(denominator); index += 2 {
		if index+1 >= len(denominator) || !differentialMark(denominator[index], mark) {
			return nil, false
		}
		group, ok := differentialVariable(denominator[index+1])
		if !ok {
			return nil, false
		}
		groups = append(groups, group)
	}
	return groups, len(groups) > 0
}

func differentialVariable(node *Node) (differentialGroup, bool) {
	if node.Name == "mi" {
		return differentialGroup{node, 1}, true
	}
	if node.Name != "msup" || len(node.Elements()) != 2 || node.Elements()[0].Name != "mi" {
		return differentialGroup{}, false
	}
	order, ok := numericPower(node.Elements()[1])
	return differentialGroup{node.Elements()[0], order}, ok
}

func numericPower(script *Node) (int, bool) {
	text, ok := leafText(script, "mn")
	if !ok {
		return 0, false
	}
	value, err := strconv.Atoi(text)
	return value, err == nil && value > 0
}

// differentialMark: an italic or upright d, or ∂.
func differentialMark(node *Node, mark string) bool {
	token := leaf(node)
	if token == nil || token.Name != "mi" && token.Name != "mo" {
		return false
	}
	text := tokenString(token)
	if mark == "∂" {
		return text == "∂"
	}
	return token.Name == "mi" && (text == "𝑑" || text == "d" && token.Get("mathvariant") == "normal")
}
