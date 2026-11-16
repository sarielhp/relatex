// Package mathintent reads a formula's TeX for the meanings its macros fix (decision 109):
// which MathML 4 intent concepts the source states, and how often. It reads source only, the
// author's verified TeX and the paper's kept math macros; where each concept lands in the MathML
// is mathexport's to find, and a concept is given only where both counts agree.
package mathintent

import (
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Concept is a concept of the MathML 4 intent Core list.
type Concept string

// The concepts the source can fix.
const (
	AbsoluteValue     Concept = "absolute-value"
	Cardinality       Concept = "cardinality"
	Norm              Concept = "norm"
	Floor             Concept = "floor"
	Ceiling           Concept = "ceiling"
	Binomial          Concept = "binomial-coefficient"
	Transpose         Concept = "transpose"
	Inverse           Concept = "inverse"
	Factorial         Concept = "factorial"
	Probability       Concept = "probability"
	Derivative        Concept = "derivative"
	PartialDerivative Concept = "partial-derivative"
	// ExpectedValue and Variance are Open concepts MathCAT reads ("expected value of X", "variance of
	// X"), named by the paper's own macros (decision 114).
	ExpectedValue Concept = "expected-value"
	Variance      Concept = "variance"
	// InnerProduct is a Core concept (MathCAT: "u inner product v").
	InnerProduct Concept = "inner-product"
	// Ket, Bra and InnerProduct as Dirac notation writes them (MathCAT: "ket of psi", "bra of phi",
	// "phi inner product psi"), named by the author's macros (decision 114, amendment 2026-11-10).
	Ket Concept = "ket"
	Bra Concept = "bra"
)

// Macro is an author macro as document.json keeps it (math_macros, decision 064).
type Macro struct {
	Body       string
	Parameters int
	// Paired: a mathtools paired delimiter (decision 114), whose uses may be starred or sized
	// (\abs*{x}, \abs[\big]{x}) and whose delimiter pair names its concept.
	Paired bool
}

// Macros are the paper's kept math macros by name.
type Macros map[string]Macro

// Speech are the author's speech hints (decision 122): a macro name without its backslash and the
// words a reader says for it. A hint sets the macro's MathML intent; without one nothing changes.
type Speech map[string]string

// speechEntry marks a hint carried in Macros, where a TeX control sequence cannot be named: MacrosOf
// reads document.json's math_speech into it, so every caller of ProjectFormula gets the hints without
// another parameter.
const speechEntry = "\x00speech:"

// Speech returns the speech hints MacrosOf read from document.json.
func (m Macros) Speech() Speech {
	speech := Speech{}
	for name, macro := range m {
		if key, ok := strings.CutPrefix(name, speechEntry); ok {
			speech[key] = macro.Body
		}
	}
	return speech
}

// conceptName is a MathML intent concept name (an NCName, decision 109).
var conceptName = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_.-]*$`)

// IntentName is the intent concept a speech hint's words become: words join with hyphens, which
// readers speak as spaces; ok is false when the words cannot name a concept.
func IntentName(words string) (string, bool) {
	name := strings.Join(strings.Fields(words), "-")
	return name, conceptName.MatchString(name)
}

// MacrosOf reads document.json's math_macros and its author hints; a document without them has none.
func MacrosOf(document rec.Map) Macros {
	macros := Macros{}
	kept, _ := document["math_macros"].(rec.Map)
	for name, value := range kept {
		entry, _ := value.(rec.Map)
		body, _ := entry["body"].(string)
		parameters, _ := entry["parameters"].(int64)
		_, paired := entry["paired"].(rec.Map)
		macros[name] = Macro{Body: body, Parameters: int(parameters), Paired: paired}
	}
	speech, _ := document["math_speech"].(rec.Map)
	for name, value := range speech {
		if text, ok := value.(string); ok {
			macros[speechEntry+name] = Macro{Body: text}
		}
	}
	return macros
}

// Reading is what a formula's source states: how many times each concept occurs. A bar concept
// (absolute value, cardinality, norm) counts only when every bar of its kind in the formula comes
// from a macro that names it, none nested in another: otherwise its pairs could not be told apart.
type Reading struct {
	Counts map[Concept]int
	// BlackboardProbability counts the probability macros whose body is \mathbb{P}.
	BlackboardProbability int
	// ItalicTranspose counts the transposes written by a macro whose body is ^T (an italic sign, decision 187).
	ItalicTranspose int
	// Speech are the author's speech hints (decision 122), macro name -> spoken name.
	Speech Speech
}

// Count is the number of occurrences of the concept.
func (r Reading) Count(concept Concept) int { return r.Counts[concept] }

// Read reads a formula's TeX with the paper's macros. A bar no macro names gets no concept: one
// meaning per document is wrong where a paper uses bars for several things (decision 114, item 5).
func Read(tex string, macros Macros, speech ...Speech) Reading {
	hints := macros.Speech()
	for _, extra := range speech {
		for key, name := range extra {
			hints[key] = name
		}
	}
	expanded := expand(tokenize(tex), macros, 0)
	s := &scanner{tokens: expanded, counts: map[Concept]int{}, singleClean: true, doubleClean: true, ketClean: true}
	s.run()
	reading := s.reading()
	if len(hints) > 0 {
		reading.Speech = hints
	}
	return reading
}

// barFamily is the bar a concept is written with: "|" or "‖".
func barFamily(concept Concept) string {
	if concept == Norm {
		return "‖"
	}
	return "|"
}

var (
	absoluteNames    = []string{"abs", "Abs", "absval", "Absval", "absv"}
	cardinalityNames = []string{"card", "Card", "cardinality"}
	probabilityNames = []string{"P", "Pr", "PP", "Prob", "prob", "Prb", "Probability", "probability", "Proba", "proba"}
	sizers           = []string{"left", "right", "bigl", "bigr", "Bigl", "Bigr", "biggl", "biggr", "Biggl", "Biggr", "mathopen",
		"mathclose", "big", "Big", "bigg", "Bigg"}
)

// recognised is the concept a macro's name and definition fix together: double bars around one
// argument are always a norm by the pair; single bars need a name that says absolute value or
// cardinality; or a probability name defined as \mathbb{P}. Anything else is expanded as written.
func recognised(name string, macro Macro) Concept {
	body := tokenize(macro.Body)
	if concept := operatorConcept(name, body); concept != "" {
		return concept
	}
	if concept := diracConcept(name, macro); concept != "" {
		return concept
	}
	if macro.Parameters == 0 {
		if slices.Contains(probabilityNames, name) && blackboardP(body) {
			return Probability
		}
		return ""
	}
	if macro.Parameters != 1 {
		return ""
	}
	switch bar := pairAround(body); {
	case bar == "‖":
		// Double bars around one argument are always a norm by the pair, whether \DeclarePairedDelimiter or \newcommand.
		return Norm
	case bar == "|" && slices.Contains(absoluteNames, name):
		return AbsoluteValue
	case bar == "|" && slices.Contains(cardinalityNames, name):
		return Cardinality
	}
	return ""
}

func blackboardP(body []token) bool {
	flat := withoutBraces(body)
	return len(flat) == 2 && flat[0].is('c', "mathbb") && flat[1].is('x', "P")
}

// pairAround is the bar a body sets on both sides of #1 (\left|#1\right|, \lvert#1\rvert,
// \|#1\|...), "" otherwise.
func pairAround(body []token) string {
	kept := []token{}
	for _, item := range withoutBraces(body) {
		if !(item.kind == 'c' && slices.Contains(sizers, item.text)) {
			kept = append(kept, item)
		}
	}
	if len(kept) != 3 || !kept[1].is('#', "1") {
		return ""
	}
	left, right := barOf(kept[0]), barOf(kept[2])
	if left == "" || left != right {
		return ""
	}
	return left
}

// barOf is the bar a token typesets, "" for anything else.
func barOf(item token) string {
	switch {
	case item.is('x', "|"), item.kind == 'c' && slices.Contains([]string{"vert", "lvert", "rvert"}, item.text):
		return "|"
	case item.kind == 'c' && slices.Contains([]string{"|", "Vert", "lVert", "rVert"}, item.text):
		return "‖"
	}
	return ""
}

func withoutBraces(tokens []token) []token {
	kept := []token{}
	for _, item := range tokens {
		if item.kind != '{' && item.kind != '}' {
			kept = append(kept, item)
		}
	}
	return kept
}

// trigonometric are the functions whose ^{-1} is their inverse function (\sin^{-1} is arcsin).
var trigonometric = []string{"sin", "cos", "tan", "sec", "csc", "cot", "sinh", "cosh", "tanh", "coth"}

// fonts are the upright and sans-serif font commands a transpose T is set in.
var fonts = []string{"mathsf", "mathrm", "textsf", "textrm", "mathsfup", "mathup"}

// transposeMark: a superscript that is the transpose sign (\top, \intercal, an upright or
// sans-serif T); an italic T is a power as often as a transpose, so it is none.
func transposeMark(argument []token) bool {
	flat := withoutBraces(argument)
	switch len(flat) {
	case 1:
		return flat[0].is('c', "top") || flat[0].is('c', "intercal")
	case 2:
		font := flat[0].kind == 'c' && slices.Contains(fonts, flat[0].text)
		return font && flat[1].is('x', "T")
	}
	return false
}

// minusOne: a superscript -1.
func minusOne(argument []token) bool {
	flat := withoutBraces(argument)
	return len(flat) == 2 && flat[0].is('x', "-") && flat[1].is('x', "1")
}

func joined(tokens []token) string {
	var builder strings.Builder
	for _, item := range tokens {
		builder.WriteString(item.text)
	}
	return builder.String()
}
