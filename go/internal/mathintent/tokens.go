package mathintent

import (
	"slices"
	"strings"
	"unicode"
)

// token is one TeX token: 'c' a control sequence (text its name, "|" for \|), 'x' a character,
// '{' '}' '^' '_' themselves, '#' a parameter (text its digit), 'o' and 'z' the opening and
// closing of a recognised bar macro's argument (concept set), 't' a macro-written transpose, 'n' a named operator's sign (concept
// set: expectation, variance).
type token struct {
	kind    byte
	text    string
	concept Concept
}

func (t token) is(kind byte, text string) bool { return t.kind == kind && t.text == text }

// tokenize reads TeX into tokens: white space and comments are dropped.
func tokenize(text string) []token {
	runes := []rune(text)
	tokens := []token{}
	for index := 0; index < len(runes); index++ {
		character := runes[index]
		switch {
		case character == '%':
			for index < len(runes) && runes[index] != '\n' {
				index++
			}
		case unicode.IsSpace(character):
		case character == '\\' && index+1 < len(runes):
			name, next := controlName(runes, index+1)
			tokens = append(tokens, token{kind: 'c', text: name})
			index = next - 1
		case character == '#' && index+1 < len(runes):
			tokens = append(tokens, token{kind: '#', text: string(runes[index+1])})
			index++
		case strings.ContainsRune("{}^_", character):
			tokens = append(tokens, token{kind: byte(character), text: string(character)})
		default:
			tokens = append(tokens, token{kind: 'x', text: string(character)})
		}
	}
	return tokens
}

// controlName is the name of the control sequence whose backslash precedes from, and the index
// after it: a run of letters, or one other character.
func controlName(runes []rune, from int) (string, int) {
	stop := from
	for stop < len(runes) && isLetter(runes[stop]) {
		stop++
	}
	if stop == from {
		stop++
	}
	return string(runes[from:stop]), stop
}

func isLetter(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '@'
}

// maxDepth bounds macro expansion: a kept macro never recurses, but a body may use another.
const maxDepth = 8

// expand replaces the uses of the paper's macros by their bodies; a recognised bar macro becomes
// its argument between an opening and a closing mark, a recognised probability macro one mark.
func expand(tokens []token, macros Macros, depth int) []token {
	result := []token{}
	for index := 0; index < len(tokens); index++ {
		item := tokens[index]
		macro, known := macros[item.text]
		if item.kind != 'c' || !known || depth >= maxDepth {
			result = append(result, item)
			continue
		}
		start := index + 1
		if macro.Paired {
			start = pairedVariant(tokens, start)
		}
		arguments, next := readArguments(tokens, start, macro.Parameters)
		index = next - 1
		concept := recognised(item.text, macro)
		if concept == "" && macro.Paired {
			concept = pairedInner(item.text, macro, arguments)
		}
		if slices.Contains([]Concept{ExpectedValue, Variance, Probability}, concept) &&
			ambiguousSign(concept, macro, tokenize(macro.Body)) && !operatorFollows(tokens, next) {
			concept = ""
		}
		if script, italic := transposeMacro(macro); script && concept == "" {
			result = append(result, token{kind: 't', concept: Transpose, text: italicText(italic)})
			continue
		}
		switch {
		case concept == Probability:
			result = append(result, token{kind: 'p', concept: concept})
		case concept == ExpectedValue || concept == Variance || concept == InnerProduct:
			// The operator's mark, then its body as written (its argument may hold more concepts).
			result = append(result, token{kind: 'n', concept: concept})
			result = append(result, expand(substitute(tokenize(macro.Body), arguments), macros, depth+1)...)
		case concept == Ket || concept == Bra:
			// The bracket's marks with the state between them (it may hold more concepts): a ket is
			// | state ⟩ and a bra ⟨ state |. The marks claim the bracket's bar.
			result = append(result, token{kind: 'o', concept: concept})
			result = append(result, expand(arguments[0], macros, depth+1)...)
			result = append(result, token{kind: 'z', concept: concept})
		case concept != "":
			result = append(result, token{kind: 'o', concept: concept})
			result = append(result, expand(arguments[0], macros, depth+1)...)
			result = append(result, token{kind: 'z', concept: concept})
		default:
			result = append(result, expand(substitute(tokenize(macro.Body), arguments), macros, depth+1)...)
		}
	}
	return result
}

// pairedVariant skips a paired delimiter's star or its optional size (\abs*{x}, \abs[\big]{x}):
// both change only the delimiters' size, never what the pair means. It returns the index of the
// first argument.
func pairedVariant(tokens []token, index int) int {
	if index >= len(tokens) || tokens[index].kind != 'x' {
		return index
	}
	if tokens[index].text == "*" {
		return index + 1
	}
	if tokens[index].text != "[" {
		return index
	}
	depth := 0
	for at := index + 1; at < len(tokens); at++ {
		switch {
		case tokens[at].kind == '{':
			depth++
		case tokens[at].kind == '}':
			depth--
		case depth == 0 && tokens[at].is('x', "]"):
			return at + 1
		}
	}
	return index
}

// pairedInner: a paired angle bracket of one parameter that names an inner product (\inner
// declared with \langle and \rangle), used around two sides joined by one comma (\inner{u, v}),
// is an inner product. Angle brackets alone are not: ⟨a, b⟩ is as often a generated group or a
// pair, so the name decides, as for a bar (decision 114, amendment 2026-11-13).
func pairedInner(name string, macro Macro, arguments [][]token) Concept {
	if macro.Parameters != 1 || !slices.Contains(innerNames, name) || !anglesAround(tokenize(macro.Body)) {
		return ""
	}
	commas, depth := 0, 0
	for _, item := range arguments[0] {
		switch {
		case item.kind == '{':
			depth++
		case item.kind == '}':
			depth--
		case depth == 0 && item.is('x', ","):
			commas++
		}
	}
	if commas != 1 {
		return ""
	}
	return InnerProduct
}

// anglesAround: the body is ⟨#1⟩ (sizers aside).
func anglesAround(body []token) bool {
	kept := []token{}
	for _, item := range withoutBraces(body) {
		if !(item.kind == 'c' && slices.Contains(sizers, item.text)) {
			kept = append(kept, item)
		}
	}
	return len(kept) == 3 && kept[0].is('c', "langle") && kept[1].is('#', "1") && kept[2].is('c', "rangle")
}

// readArguments reads count undelimited arguments from index: a braced group's tokens, or one
// token. A missing argument is empty.
func readArguments(tokens []token, index, count int) ([][]token, int) {
	arguments := make([][]token, count)
	for argument := range count {
		arguments[argument], index = readArgument(tokens, index)
	}
	return arguments, index
}

func readArgument(tokens []token, index int) ([]token, int) {
	if index >= len(tokens) {
		return nil, index
	}
	if tokens[index].kind != '{' {
		return tokens[index : index+1], index + 1
	}
	depth := 0
	for stop := index; stop < len(tokens); stop++ {
		switch tokens[stop].kind {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return tokens[index+1 : stop], stop + 1
			}
		}
	}
	return tokens[index+1:], len(tokens)
}

// substitute puts the arguments in place of #1...#9, each braced as TeX passes it.
func substitute(body []token, arguments [][]token) []token {
	result := []token{}
	for _, item := range body {
		if item.kind != '#' || parameter(item) >= len(arguments) {
			result = append(result, item)
			continue
		}
		result = append(result, token{kind: '{', text: "{"})
		result = append(result, arguments[parameter(item)]...)
		result = append(result, token{kind: '}', text: "}"})
	}
	return result
}

// parameter is a parameter token's argument index; past any argument list for #0 or a non-digit.
func parameter(item token) int {
	if item.text == "" || item.text[0] < '1' || item.text[0] > '9' {
		return 1 << 30
	}
	return int(item.text[0] - '1')
}
