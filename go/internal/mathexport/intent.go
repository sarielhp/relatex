package mathexport

import (
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/mathintent"
	"github.com/sarielhp/relatex/internal/rec"
)

// MathML 4 intent from the author's TeX (decision 109). The source names a concept
// (mathintent.Read counts \binom, \lfloor, \abs defined as a bar pair, \frac{dy}{dx}, A^\top...);
// the page's MathML is searched for the constructs that concept typesets; only when the two counts
// agree does each construct get its intent, so a construct the source does not name never gets
// one. A construct spanning several siblings is wrapped in a new mrow carrying the intent, and an
// argument of several siblings in a new mrow carrying its arg: the acceptance unwraps exactly
// those (lib/relatex/math_intent.rb), so the rest of the formula is checked as before.

// ProjectFormula is ProjectAnnotated for a math node of the document: its AnnotationTeX as the
// annotation and the intent the author's own TeX states, read with the paper's macros. The author's speech
// hints (decision 122) are given either in the document's math_speech, read by MacrosOf, or by
// speech.
func ProjectFormula(root *Node, node rec.Map, macros mathintent.Macros, speech ...mathintent.Speech) string {
	return projectFormula(root, node, macros, speech).String()
}

// ProjectPage is ProjectFormula as the page draws it: fences of ordinary height are not stretchy (a
// browser draws a stretchy fence padded; see softFences), and the numbers of alignments at the right
// margin where the PDF prints them (labelsRight).
func ProjectPage(root *Node, node rec.Map, macros mathintent.Macros) string {
	projected := projectFormula(root, node, macros, nil)
	softFences(projected)
	labelsRight(projected)
	return projected.String()
}

func projectFormula(root *Node, node rec.Map, macros mathintent.Macros, speech []mathintent.Speech) *Node {
	tex := AuthorTeX(node)
	copied := root.Clone()
	applyVariant(copied, "", false)
	cleanSpeech(copied)
	if annotation := AnnotationTeX(node); annotation != "" {
		annotate(copied, annotation)
	}
	// Intent reads the author's own TeX only: the formula TeX is the MathML again and states
	// no meaning the MathML does not. After the annotation, so new rows go inside its
	// presentation and never become it.
	if tex != "" {
		ApplyIntent(copied, mathintent.Read(tex, macros, speech...))
	}
	return copied
}

// intentRule finds the constructs of one concept; each placement gives one its intent.
type intentRule struct {
	concept mathintent.Concept
	find    func(root *Node) []placement
}

type placement func(names *argNames)

var intentRules = []intentRule{
	{mathintent.Derivative, func(root *Node) []placement { return derivatives(root, "d") }},
	{mathintent.PartialDerivative, func(root *Node) []placement { return derivatives(root, "∂") }},
	{mathintent.Binomial, binomials},
	{mathintent.Floor, func(root *Node) []placement { return delimited(root, "⌊", "⌋", mathintent.Floor) }},
	{mathintent.Ceiling, func(root *Node) []placement { return delimited(root, "⌈", "⌉", mathintent.Ceiling) }},
	{mathintent.AbsoluteValue, func(root *Node) []placement { return bars(root, "|", mathintent.AbsoluteValue) }},
	{mathintent.Cardinality, func(root *Node) []placement { return bars(root, "|", mathintent.Cardinality) }},
	{mathintent.Norm, func(root *Node) []placement { return bars(root, "‖", mathintent.Norm) }},
	{mathintent.Transpose, func(root *Node) []placement { return transposes(root, false) }},
	{mathintent.Inverse, inverses},
	{mathintent.Factorial, factorials},
	{mathintent.Probability, func(root *Node) []placement { return namedTokens(root, "Pr") }},
	{mathintent.ExpectedValue, func(root *Node) []placement { return operatorTokens(root, mathintent.ExpectedValue, expectationSign) }},
	{mathintent.Variance, func(root *Node) []placement { return operatorTokens(root, mathintent.Variance, varianceSign) }},
	{mathintent.InnerProduct, innerProductShapes},
	{mathintent.Ket, kets},
	{mathintent.Bra, bras},
}

// ApplyIntent gives the formula's constructs the intent its source reading states.
func ApplyIntent(math *Node, reading mathintent.Reading) {
	names := &argNames{}
	for _, rule := range intentRules {
		count := reading.Count(rule.concept)
		if count == 0 {
			continue
		}
		find := rule.find
		if rule.concept == mathintent.Transpose {
			find = func(root *Node) []placement { return transposes(root, reading.ItalicTranspose > 0) }
		}
		if found := find(math); len(found) == count {
			for _, place := range found {
				place(names)
			}
		}
	}
	if count := reading.BlackboardProbability; count > 0 {
		if found := namedTokens(math, "ℙ"); len(found) == count {
			for _, place := range found {
				place(names)
			}
		}
	}
	applySpeechHints(math, reading.Speech, names)
}

// applySpeechHints gives a macro the author named (decision 122) the intent of an operator applied
// to its argument, so MathCAT says "image of f" where the source reads \Im f.
func applySpeechHints(math *Node, speech mathintent.Speech, names *argNames) {
	for key, name := range speech {
		for _, operator := range speechOperators(math, key) {
			speechIntent(operator, name, names)
		}
	}
}

// speechOperators are the identifiers the hinted macro shows as, each with an argument after it.
func speechOperators(root *Node, key string) []*Node {
	found := []*Node{}
	for _, node := range root.Descendants() {
		if node.Name == "mi" && tokenString(node) == key && speechArgument(operatorUnit(node)) != nil {
			found = append(found, node)
		}
	}
	return found
}

// operatorUnit is the node the operator shows as: the identifier, or the row or script wrapping
// only it (the collector writes \mathrm{Im} as an mrow of one mi).
func operatorUnit(node *Node) *Node {
	for node.Parent != nil && baseWrapper(node.Parent) {
		node = node.Parent
	}
	return node
}

// annotation: a metadata element, not rendered, so never an operator's argument.
func annotation(node *Node) bool {
	return node.Name == "annotation" || node.Name == "annotation-xml"
}

// speechArgument is the element a hinted operator applies to: the next non-space sibling, an
// invisible operator or an annotation aside, when an argument may follow.
func speechArgument(operator *Node) *Node {
	children := operator.Parent.Elements()
	index := slices.Index(children, operator)
	if index < 0 {
		return nil
	}
	for _, sibling := range children[index+1:] {
		if sibling.Name == "mspace" || invisibleOperator(sibling) || annotation(sibling) {
			continue
		}
		if appliesTo(sibling) {
			return sibling
		}
		return nil
	}
	return nil
}

// speechIntent wraps the operator and its argument in a new row carrying name($a1), the argument
// named. The operator's own literal intent, if the speech pass gave one, gives way to the author's.
func speechIntent(operator *Node, name string, names *argNames) {
	unit := operatorUnit(operator)
	argument := speechArgument(unit)
	concept, ok := mathintent.IntentName(name)
	if argument == nil || !ok {
		return
	}
	operator.RemoveAttribute("intent")
	row := wrap(unit.Parent, unit, argument)
	row.SetAttribute("intent", concept+"($"+names.of(argument)+")")
}

// argNames hands out the formula's argument names: a1, a2...
type argNames struct{ next int }

// of is the node's argument name, given it now when it has none. A row of the formula gets a new
// mrow around it to carry the name: only new rows carry intent or arg, so the acceptance can tell
// them from the formula's own.
func (a *argNames) of(node *Node) string {
	if node.Name == "mrow" && !node.Has("intent") && !node.Has("arg") {
		node = wrap(node.Parent, node, node)
	}
	if name, ok := node.Attribute("arg"); ok {
		return name
	}
	return a.assign(node)
}

// assign gives the node the next argument name.
func (a *argNames) assign(node *Node) string {
	a.next++
	name := "a" + strconv.Itoa(a.next)
	node.SetAttribute("arg", name)
	return name
}

// argument is the argument a run of siblings forms: the one element, or a new mrow around them.
func (a *argNames) argument(nodes []*Node) string {
	if len(nodes) == 1 {
		return "$" + a.of(nodes[0])
	}
	parent := nodes[0].Parent
	wrapper := wrap(parent, nodes[0], nodes[len(nodes)-1])
	return "$" + a.assign(wrapper)
}

// wrap puts the siblings first...last in a new mrow in their place.
func wrap(parent, first, last *Node) *Node {
	children := parent.Elements()
	start, stop := slices.Index(children, first), slices.Index(children, last)
	wrapper := &Node{Name: "mrow", Parent: parent}
	wrapper.setElements(children[start : stop+1])
	replaced := append(slices.Clone(children[:start]), wrapper)
	parent.setElements(append(replaced, children[stop+1:]...))
	return wrapper
}

// fenceCore is the operator a node shows as a delimiter: an mo, or a wrapper whose only element
// apart from spaces leads to one (a sized fence is an mo in mpadded, with struts).
func fenceCore(node *Node) *Node {
	for node != nil {
		if node.Name == "mo" {
			return node
		}
		if !slices.Contains([]string{"mrow", "mpadded", "mstyle"}, node.Name) {
			return nil
		}
		var only *Node
		for _, child := range node.Elements() {
			if child.Name == "mspace" {
				continue
			}
			if only != nil {
				return nil
			}
			only = child
		}
		node = only
	}
	return nil
}

func fenceText(node *Node) string {
	if core := fenceCore(node); core != nil {
		return tokenString(core)
	}
	return ""
}

// elementsOf are the elements a row holds, spaces and invisible operators left out; a non-row is
// its own one element.
func elementsOf(node *Node) []*Node {
	if node.Name != "mrow" {
		return []*Node{node}
	}
	kept := []*Node{}
	for _, child := range node.Elements() {
		if child.Name != "mspace" && !invisibleOperator(child) {
			kept = append(kept, child)
		}
	}
	return kept
}

// leaf descends single-element rows to the token they hold, nil when there is none.
func leaf(node *Node) *Node {
	for node != nil && slices.Contains([]string{"mrow", "mstyle", "mpadded"}, node.Name) {
		elements := node.Elements()
		if len(elements) != 1 {
			return nil
		}
		node = elements[0]
	}
	return node
}

func leafText(node *Node, names ...string) (string, bool) {
	token := leaf(node)
	if token == nil || !slices.Contains(names, token.Name) {
		return "", false
	}
	return tokenString(token), true
}

// setIntent gives a node its intent unless it has one (an equation label's).
func setIntent(node *Node, intent string) {
	if !node.Has("intent") {
		node.SetAttribute("intent", intent)
	}
}

func call(concept mathintent.Concept, arguments ...string) string {
	return string(concept) + "(" + strings.Join(arguments, ",") + ")"
}
