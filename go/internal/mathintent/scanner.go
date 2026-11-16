package mathintent

import (
	"slices"
	"strings"
)

// scanner counts the concepts of an expanded token list.
type scanner struct {
	tokens []token
	counts map[Concept]int
	// singleClean and doubleClean: every | (‖) of the formula comes from a macro or delimiter that
	// names its concept, and none such pair nests in another. The bars of a ket and a bra are the
	// concept's own, so they set neither.
	singleClean, doubleClean bool
	singleDepth, doubleDepth int
	singleConcepts           []Concept
	blackboard               int
	// italicTranspose counts the macro-written transposes whose sign is an italic T.
	italicTranspose int
	// ketDepth, ketClean and braOpen: a ket nests in no other, a bra's marks pair out (both claim
	// their bracket's bar).
	ketDepth, braOpen int
	ketClean          bool
}

var (
	binomials = []string{"binom", "tbinom", "dbinom", "choose"}
	fractions = []string{"frac", "dfrac", "tfrac"}
	// openers and closers are the sized delimiters that make a ‖ a norm's side.
	openers = []string{"left", "bigl", "Bigl", "biggl", "Biggl"}
	closers = []string{"right", "bigr", "Bigr", "biggr", "Biggr"}
)

func (s *scanner) run() {
	for index, item := range s.tokens {
		switch item.kind {
		case 'o', 'z':
			s.mark(item)
		case 'p':
			s.blackboard++
		case 'n':
			s.counts[item.concept]++
		case 't':
			s.counts[Transpose]++
			if item.text == "italic" {
				s.italicTranspose++
			}
		case 'x':
			s.character(item)
		case 'c':
			s.command(index, item)
		case '^':
			s.superscript(index)
		}
	}
}

func (s *scanner) reading() Reading {
	counts := map[Concept]int{}
	for concept, count := range s.counts {
		counts[concept] = count
	}
	if !s.singleClean || len(s.singleConcepts) > 1 {
		delete(counts, AbsoluteValue)
		delete(counts, Cardinality)
	}
	if !s.singleClean || s.braOpen > 0 {
		delete(counts, Bra)
	}
	if !s.singleClean || !s.ketClean || s.ketDepth != 0 {
		delete(counts, Ket)
	}
	if !s.doubleClean {
		delete(counts, Norm)
	}
	return Reading{Counts: counts, BlackboardProbability: s.blackboard, ItalicTranspose: s.italicTranspose}
}

// mark opens or closes a recognised bracket macro's argument. A bar macro and a ket carry their
// bar first; a bra carries its bar last, so its marks pair out instead: both claim one bracket.
func (s *scanner) mark(item token) {
	depth := &s.singleDepth
	clean := &s.singleClean
	if item.concept == Bra {
		if closedBy(&s.braOpen) == 0 {
			s.counts[Bra]++
		}
		return
	}
	if item.concept == Ket {
		depth, clean = &s.ketDepth, &s.ketClean
	}
	if barFamily(item.concept) == "‖" {
		depth, clean = &s.doubleDepth, &s.doubleClean
	}
	if item.kind == 'z' {
		*depth--
		return
	}
	if *depth > 0 {
		*clean = false
	}
	*depth++
	s.counts[item.concept]++
	if item.concept != Norm && item.concept != Ket && !slices.Contains(s.singleConcepts, item.concept) {
		s.singleConcepts = append(s.singleConcepts, item.concept)
	}
}

// closedBy: a mark closes the innermost open bracket of its kind (1) or opens one (0); a bracket
// inside another is never told apart from it, so the bars are not clean.
func closedBy(open *int) int {
	if *open > 0 {
		*open--
		return 1
	}
	*open++
	return 0
}

func (s *scanner) character(item token) {
	switch item.text {
	case "!":
		s.counts[Factorial]++
	case "|":
		// A bar no macro names: its meaning is unknown, so no bar of the formula gets one.
		s.singleClean = false
	}
}

func (s *scanner) command(index int, item token) {
	switch {
	case slices.Contains(binomials, item.text):
		s.counts[Binomial]++
	case item.text == "lfloor":
		s.counts[Floor]++
	case item.text == "lceil":
		s.counts[Ceiling]++
	case item.text == "Pr":
		s.counts[Probability]++
	case item.text == "operatorname":
		if argument, _ := readArgument(s.tokens, index+1); joined(withoutBraces(argument)) == "Pr" {
			s.counts[Probability]++
		}
	case slices.Contains(fractions, item.text):
		s.fraction(index)
	case barOf(item) == "|":
		s.singleClean = false
	case barOf(item) == "‖":
		s.doubleBar(index, item)
	}
}

// doubleBar: \lVert opens a norm and \rVert closes one, so do \left\| and \right\| (or sized \bigl,
// \bigr); a bare \| or \Vert could be either side, or a parallel sign.
func (s *scanner) doubleBar(index int, item token) {
	previous := token{}
	if index > 0 {
		previous = s.tokens[index-1]
	}
	switch {
	case item.text == "lVert", previous.kind == 'c' && slices.Contains(openers, previous.text):
		if s.doubleDepth > 0 {
			s.doubleClean = false
		}
		s.doubleDepth++
		s.counts[Norm]++
	case item.text == "rVert", previous.kind == 'c' && slices.Contains(closers, previous.text):
		s.doubleDepth--
	default:
		s.doubleClean = false
	}
}

// superscript counts a transpose sign, and -1 on a trigonometric function.
func (s *scanner) superscript(index int) {
	argument, next := readArgument(s.tokens, index+1)
	if len(argument) == 1 && argument[0].kind == 'c' && slices.Contains(fonts, argument[0].text) {
		// ^\mathsf{T}: the font command takes its own argument.
		font, _ := readArgument(s.tokens, next)
		argument = append(slices.Clone(argument), font...)
	}
	previous := token{}
	if index > 0 {
		previous = s.tokens[index-1]
	}
	switch {
	case transposeMark(argument):
		s.counts[Transpose]++
	case minusOne(argument) && previous.kind == 'c' && slices.Contains(trigonometric, previous.text):
		s.counts[Inverse]++
	}
}

// fraction counts a Leibniz derivative written as a fraction with its function in the numerator:
// \frac{dy}{dx}, \frac{d^2 y}{dx^2}, \frac{\partial f}{\partial x}, \frac{\partial^2 f}{\partial x
// \partial y}. The operator form \frac{d}{dx} has no function to name and counts as none.
func (s *scanner) fraction(index int) {
	arguments, _ := readArguments(s.tokens, index+1, 2)
	if concept := leibniz(withoutBraces(arguments[0]), withoutBraces(arguments[1])); concept != "" {
		s.counts[concept]++
	}
}

func leibniz(numerator, denominator []token) Concept {
	kind, at := differential(numerator, 0)
	if kind == "" {
		return ""
	}
	order, at := exponent(numerator, at)
	if order == 0 || at >= len(numerator) || numerator[at].kind == 'x' && strings.Contains("+-=<>,/|", numerator[at].text) {
		return ""
	}
	total, groups := 0, 0
	for at = 0; at < len(denominator); {
		if spacing(denominator[at]) {
			at++
			continue
		}
		mark, next := differential(denominator, at)
		if mark != kind || next >= len(denominator) || !variable(denominator[next]) {
			return ""
		}
		power, after := exponent(denominator, next+1)
		if power == 0 {
			return ""
		}
		total, groups, at = total+power, groups+1, after
	}
	switch {
	case groups == 0 || total != order:
		return ""
	case kind == "d" && groups == 1:
		return Derivative
	case kind == "partial":
		return PartialDerivative
	}
	return ""
}

// differential reads a d (italic, \mathrm{d}, \operatorname{d}) or a \partial at index: its kind
// and the index after it.
func differential(tokens []token, index int) (string, int) {
	if index >= len(tokens) {
		return "", index
	}
	item := tokens[index]
	switch {
	case item.is('x', "d"):
		return "d", index + 1
	case item.is('c', "partial"):
		return "partial", index + 1
	case item.kind == 'c' && slices.Contains([]string{"mathrm", "textrm", "operatorname", "mathup"}, item.text) &&
		index+1 < len(tokens) && tokens[index+1].is('x', "d"):
		return "d", index + 2
	}
	return "", index
}

// exponent reads an optional ^n (a number) at index: n (1 without one) and the index after it; 0
// for a superscript that is not a number.
func exponent(tokens []token, index int) (int, int) {
	if index >= len(tokens) || tokens[index].kind != '^' {
		return 1, index
	}
	value, at := 0, index+1
	for at < len(tokens) && tokens[at].kind == 'x' && len(tokens[at].text) == 1 && tokens[at].text[0] >= '0' && tokens[at].text[0] <= '9' {
		value = value*10 + int(tokens[at].text[0]-'0')
		at++
	}
	return value, at
}

// variable: a letter or a Greek letter's command.
func variable(item token) bool {
	if item.kind == 'x' {
		return len(item.text) == 1 && isLetter(rune(item.text[0])) && item.text != "@"
	}
	return item.kind == 'c' && slices.Contains(greek, item.text)
}

var greek = []string{"alpha", "beta", "gamma", "delta", "epsilon", "varepsilon", "zeta", "eta", "theta", "vartheta", "iota",
	"kappa", "lambda", "mu", "nu", "xi", "pi", "rho", "sigma", "tau", "upsilon", "phi", "varphi", "chi", "psi", "omega",
	"Gamma", "Delta", "Theta", "Lambda", "Xi", "Pi", "Sigma", "Upsilon", "Phi", "Psi", "Omega"}

func spacing(item token) bool {
	return item.kind == 'c' && slices.Contains([]string{",", ";", ":", "!", " ", "thinspace", "quad", "medspace"}, item.text)
}
