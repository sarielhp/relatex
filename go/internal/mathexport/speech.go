package mathexport

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// The MathML a page writes for speech (decision 100). Speech engines (MathCAT, Speech Rule
// Engine, VoiceOver) read presentation MathML; the collector's MathML follows TeX's math list,
// which speaks badly in a few places: an operator name (\log, \operatorname{rank}) is one
// upright mi per letter ("normal l normal o normal g"), mathvariant="normal" stands on symbols
// MathML Core never italicizes ("normal empty set"), a named function has no function
// application, and fences are not grouped with what they enclose. cleanSpeech reorganizes the
// browser projection on those points only: it never changes a glyph, a space or a stretch, so
// the page renders as before. The acceptance reads the same rules on its own
// (lib/relatex/math_speech.rb).

var (
	speechRows     = []string{"math", "mrow", "mstyle", "mpadded", "mphantom", "msqrt", "mtd"}
	speechScripts  = []string{"msub", "msup", "msubsup", "munder", "mover", "munderover"}
	speechWrappers = []string{"mrow", "mstyle", "mpadded"}
	// speechFunctions are LaTeX's predefined operator names that take an argument, and the
	// common \operatorname ones (div, rank, tr, dist, diam...).
	speechFunctions = []string{"arccos", "arcsin", "arctan", "arg", "cos", "cosh", "cot", "coth", "csc", "deg", "det",
		"dim", "dist", "exp", "gcd", "hom", "Id", "id", "inf", "ker", "lcm", "lg", "lim", "liminf", "limsup", "ln", "log",
		"max", "min", "Pr", "sec", "sin", "sinh", "sup", "tan", "tanh", "Cov", "curl", "diag", "diam", "div", "erf", "grad",
		"rank", "rot", "sgn", "sign", "supp", "tr", "Tr", "Var", "vol", "Stab", "rk", "Aut", "dom", "argmax",
		"argmin", "im", "conv", "Lip", "coker", "codim", "Hess", "proj", "cl"}
	speechPairs = map[string]string{"(": ")", "[": "]", "{": "}", "⟨": "⟩", "⌊": "⌋", "⌈": "⌉"}
)

const (
	// functionApplication is U+2061 FUNCTION APPLICATION.
	functionApplication = "\u2061"
	// speechOpening are the operators an argument may open with.
	speechOpening = "([{⟨⌊⌈|‖"
	// speechStretchy are the characters MathML Core may stretch along a row: inside a new group
	// they would stretch to the group instead of the row.
	speechStretchy = "()[]{}⟨⟩⌊⌋⌈⌉|‖∣∥/\\↑↓⇑⇓↕⇕⟮⟯⦅⦆⎰⎱∫∬∭∮"
)

// autoItalic reports whether MathML Core's math-auto transform italicizes the character: only
// on these does a single character's mathvariant="normal" change what shows.
func autoItalic(character rune) bool {
	switch {
	case character >= 'A' && character <= 'Z', character >= 'a' && character <= 'z':
		return true
	case character >= 0x391 && character <= 0x3A9 && character != 0x3A2, character >= 0x3B1 && character <= 0x3C9:
		return true
	}
	return slices.Contains([]rune{0x131, 0x237, 0x3F4, 0x2207, 0x2202, 0x3F5, 0x3D1, 0x3F0, 0x3D5, 0x3F1, 0x3D6}, character)
}

func cleanSpeech(node *Node) {
	cleanSpeechRows(node, &accentNames{})
}

func cleanSpeechRows(node *Node, accents *accentNames) {
	for _, child := range node.Elements() {
		cleanSpeechRows(child, accents)
	}
	if node.Name == "mi" {
		normalVariant(node)
	}
	if node.Name == "mi" || node.Name == "mo" || node.Name == "mn" {
		speechToken(node)
	}
	if node.Name == "mtd" {
		dropEmptyLabel(node)
	}
	if !slices.Contains(speechRows, node.Name) {
		return
	}
	joinTextRuns(node)
	mergeLetters(node)
	mergeStyledNames(node)
	mergeDecimals(node)
	attachScripts(node)
	separateIdentifiers(node)
	applyFunctions(node)
	applyAccents(node, accents)
	groupFences(node)
}

func tokenString(node *Node) string { return strings.Join(node.Texts(), "") }

// normalVariant drops mathvariant="normal" where MathML Core would not italicize anyway: more
// than one character, or one outside the math-auto letters.
func normalVariant(node *Node) {
	if node.Get("mathvariant") != "normal" {
		return
	}
	text := tokenString(node)
	character, _ := utf8.DecodeRuneInString(text)
	if utf8.RuneCountInString(text) != 1 || !autoItalic(character) {
		node.RemoveAttribute("mathvariant")
	}
}

func uprightLetter(node *Node) bool {
	if node.Name != "mi" || len(node.Attrs) != 1 || node.Get("mathvariant") != "normal" {
		return false
	}
	text := tokenString(node)
	return len(text) == 1 && (text[0] >= 'A' && text[0] <= 'Z' || text[0] >= 'a' && text[0] <= 'z')
}

// mergeLetters makes each run of upright letters one identifier.
func mergeLetters(row *Node) {
	kept := []*Node{}
	var run *Node
	for _, child := range row.Elements() {
		switch {
		case !uprightLetter(child):
			kept, run = append(kept, child), nil
		case run == nil:
			kept, run = append(kept, child), child
		default:
			run.SetText(tokenString(run) + tokenString(child))
			run.RemoveAttribute("mathvariant")
		}
	}
	row.setElements(kept)
}

// styledVariants are the mathematical alphabets whose letters spell a word as themselves: sans-serif,
// bold sans-serif and typewriter. Italic ones mark variables and indices instead.
var styledVariants = []string{"sans-serif", "bold-sans-serif", "monospace"}

// styledLetter: an mi of one ASCII letter of one of styledVariants, with no attribute (the projection
// encodes the alphabet in the character).
func styledLetter(node *Node) (string, string, bool) {
	if node.Name != "mi" || len(node.Attrs) != 0 {
		return "", "", false
	}
	text := tokenString(node)
	if utf8.RuneCountInString(text) != 1 {
		return "", "", false
	}
	base, variant, ok := decode([]rune(text)[0], "", false)
	if !ok || len(base) != 1 || !asciiLetter(base[0]) || !slices.Contains(styledVariants, variant) {
		return "", "", false
	}
	return base, variant, true
}

// mergeStyledNames writes a run of letters of one styled alphabet that spells a known function name
// as one mi: MathCAT reads the three sans-serif mi of \mathsf{log} "l o g" (sample 84). Any other run
// stays: a single letter or a word that is no function name marks an index or a name (T, refl), and a
// space, a script or another atom between the letters breaks the run.
func mergeStyledNames(row *Node) {
	children := row.Elements()
	result := make([]*Node, 0, len(children))
	for index := 0; index < len(children); {
		child := children[index]
		base, variant, ok := styledLetter(child)
		if !ok {
			result = append(result, child)
			index++
			continue
		}
		run := []*Node{child}
		word := base
		for index+len(run) < len(children) {
			nextBase, nextVariant, nextOK := styledLetter(children[index+len(run)])
			if !nextOK || nextVariant != variant {
				break
			}
			run = append(run, children[index+len(run)])
			word += nextBase
		}
		if len(run) > 1 && slices.Contains(speechFunctions, word) {
			text := ""
			for _, letter := range run {
				text += tokenString(letter)
			}
			run[0].SetText(text)
			result = append(result, run[0])
		} else {
			result = append(result, run...)
		}
		index += len(run)
	}
	row.setElements(result)
}

// applyFunctions puts U+2061 after a named function that an argument follows (and spells its name).
func applyFunctions(row *Node) {
	children := row.Elements()
	result := []*Node{}
	for index, child := range children {
		result = append(result, child)
		if functionName(child) && appliesTo(nextNonSpace(children, index)) {
			result = append(result, newToken(row, "mo", functionApplication))
			spokenName(child)
		}
	}
	row.setElements(result)
}

func nextNonSpace(children []*Node, index int) *Node {
	for _, sibling := range children[index+1:] {
		if sibling.Name != "mspace" {
			return sibling
		}
	}
	return nil
}

// baseWrapper: a script element (its base) or a wrapper of one element leads to the base.
func baseWrapper(node *Node) bool {
	count := len(node.Elements())
	return slices.Contains(speechScripts, node.Name) && count > 0 || slices.Contains(speechWrappers, node.Name) && count == 1
}

// functionWord: the name a function is known by: a styled word's base letters (𝗅𝗈𝗀 is log), else
// the node's own text.
func functionWord(node *Node) string {
	if word, ok := styledWord(node, tokenString(node)); ok {
		return word
	}
	return tokenString(node)
}

// functionName: a named function, alone or as the base of its scripts (\log_2, \max_x, \lim_n).
// A name of several words (\limsup: lim, a space, sup) is one function too.
func functionName(node *Node) bool {
	for baseWrapper(node) {
		node = node.Elements()[0]
	}
	if node.Name == "mi" || operatorText(node) {
		return slices.Contains(speechFunctions, functionWord(node))
	}
	words := 0
	for _, child := range node.Elements() {
		if child.Name == "mspace" {
			continue
		}
		if child.Name != "mi" || !slices.Contains(speechFunctions, functionWord(child)) {
			return false
		}
		words++
	}
	return node.Name == "mrow" && words > 0
}

// operatorText: an mtext inside an mpadded, how the collector writes \operatorname{supp}: the name's letters
// as text.
func operatorText(node *Node) bool {
	return node.Name == "mtext" && node.Parent != nil && node.Parent.Name == "mpadded"
}

// appliesTo: an argument follows, anything but an operator, or an opening fence.
func appliesTo(node *Node) bool {
	if node == nil || functionName(node) {
		return false
	}
	if node.Name != "mo" {
		return true
	}
	text := tokenString(node)
	return utf8.RuneCountInString(text) == 1 && strings.Contains(speechOpening, text)
}

func newToken(parent *Node, name, text string) *Node {
	token := &Node{Name: name, Parent: parent}
	token.SetText(text)
	return token
}
