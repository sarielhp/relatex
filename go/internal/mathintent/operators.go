package mathintent

import "slices"

// The operators the paper's macros name (decision 114): measured on the stores (534 items), a macro
// for the expectation is defined in 73 papers and used 4,232 times, one for the variance in 38
// papers, 407 uses. A macro counts when its name says the concept and its body begins with the
// operator's sign in an operator's font: \E defined as \mathbb{E}, \operatorname{E} or
// \mathop{\mathbb{E}}\left[#1\right]; \Var as \operatorname{Var}, \mathrm{Var} or \mathbb{V}. An
// italic E is a variable, and a bare E or Var in a formula says nothing.

var (
	expectationNames = []string{"E", "EE", "Ex", "EX", "Expect", "expect", "Expectation", "expectation", "Esp", "Eb", "bE", "expec"}
	varianceNames    = []string{"Var", "var", "VAR", "Variance", "variance"}
	// operatorWrappers may stand before the sign: \mathop{...}, \operatorname*{...}.
	operatorWrappers = []string{"mathop", "operatornamewithlimits", "DOTSB"}
	// signFonts are the fonts an operator's sign is set in.
	signFonts = []string{"operatorname", "mathbb", "mathbbm", "mathds", "mathrm", "mathup", "mathsf", "mathbf", "textrm", "textsf",
		"textup", "text", "mathsfup", "bm", "boldsymbol"}
)

// innerNames are the names of inner product macros (17 papers of the stores, 269 uses).
var innerNames = []string{"inner", "innerprod", "innerproduct", "inprod", "iprod", "ip", "Inner", "ang", "scal", "scalar"}

// operatorConcept: the expectation, variance or inner product a macro's name and body state
// together, "" else.
func operatorConcept(name string, body []token) Concept {
	switch {
	case slices.Contains(innerNames, name) && anglePair(body):
		return InnerProduct
	case slices.Contains(expectationNames, name) && signedBy(body, "E"):
		return ExpectedValue
	case slices.Contains(varianceNames, name) && (signedBy(body, "Var") || signedBy(body, "var") || signedBy(body, "V")):
		return Variance
	}
	return ""
}

// signedBy: the body begins (behind any wrapper) with a sign font and the word, no letter after it.
func signedBy(body []token, word string) bool {
	flat := withoutBraces(body)
	at := 0
	for at < len(flat) && (flat[at].kind == 'c' && slices.Contains(operatorWrappers, flat[at].text) || flat[at].is('x', "*")) {
		at++
	}
	if at >= len(flat) || flat[at].kind != 'c' || !slices.Contains(signFonts, flat[at].text) {
		return false
	}
	at++
	if at < len(flat) && flat[at].is('x', "*") {
		at++
	}
	for _, character := range word {
		if at >= len(flat) || !flat[at].is('x', string(character)) {
			return false
		}
		at++
	}
	return at >= len(flat) || !(flat[at].kind == 'x' && len(flat[at].text) == 1 && isLetter(rune(flat[at].text[0])))
}

// anglePair: the body is ⟨#1, #2⟩ (\langle, or < and > after \left and \right; sizers and thin
// spaces aside).
func anglePair(body []token) bool {
	kept := []token{}
	for _, item := range withoutBraces(body) {
		if !(item.kind == 'c' && (slices.Contains(sizers, item.text) || spacing(item))) {
			kept = append(kept, item)
		}
	}
	if len(kept) != 5 || !kept[1].is('#', "1") || !kept[2].is('x', ",") || !kept[3].is('#', "2") {
		return false
	}
	return (kept[0].is('c', "langle") || kept[0].is('x', "<")) && (kept[4].is('c', "rangle") || kept[4].is('x', ">"))
}

// ambiguousSign: the macro's body shows its sign in a font a plain symbol also uses (a bold, roman
// or sans-serif single letter: \def\bE{{\mathbf E}} names a triple of sets in papers4/1042), and no
// parameter or operator wrapper says it takes an argument. Blackboard E and V, the word Var and
// anything under \operatorname or \mathop stay operators. A bare blackboard P is as ambiguous as a
// bold E.
func ambiguousSign(concept Concept, macro Macro, body []token) bool {
	flat := withoutBraces(body)
	switch {
	case macro.Parameters > 0, len(flat) > 0 && flat[0].kind == 'c' && slices.Contains(operatorWrappers, flat[0].text):
		return false
	case concept == Probability:
		return true
	case len(flat) == 0 || flat[0].kind != 'c':
		return false
	case flat[0].text == "operatorname" || slices.Contains([]string{"mathbb", "mathbbm", "mathds"}, flat[0].text):
		return false
	}
	return !signedByWord(flat[1:], "Var") && !signedByWord(flat[1:], "var")
}

// signedByWord: the tokens after the font are the word (behind a star).
func signedByWord(rest []token, word string) bool {
	for len(rest) > 0 && rest[0].is('x', "*") {
		rest = rest[1:]
	}
	for _, character := range word {
		if len(rest) == 0 || !rest[0].is('x', string(character)) {
			return false
		}
		rest = rest[1:]
	}
	return true
}

// operatorFollows: an operand bracket comes next after the macro, behind sizers, spaces and
// scripts: \bE[X], \bE\left[X\right], \bE_{\mu}[X]. A name followed by anything else (an equals
// sign, a comma, a letter) is a symbol.
func operatorFollows(tokens []token, index int) bool {
	for index < len(tokens) {
		item := tokens[index]
		switch {
		case item.kind == 'c' && (slices.Contains(sizers, item.text) || spacing(item)):
			index++
		case item.kind == '^' || item.kind == '_':
			_, index = readArgument(tokens, index+1)
		default:
			return item.is('x', "(") || item.is('x', "[") || item.is('c', "lbrack") || item.is('c', "{") || item.is('c', "lbrace")
		}
	}
	return false
}
