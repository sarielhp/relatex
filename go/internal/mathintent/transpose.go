package mathintent

import (
	"slices"
	"strings"
)

// Transpose by a macro's body (decision 187): a macro without parameters whose body is a transpose
// script (^\top, ^T, ^{\mathsf T}, ^\intercal, with spacing and an ensuremath around it) says
// transpose wherever it is used, after a closing parenthesis or a letter alike. The body is the
// evidence, never the macro's name. An italic T (^T) is a power as often as a transpose in running
// text, but in a macro that does nothing else it is the author's transpose sign.

// kernCommands take a dimension; the dimension is no part of the script.
var kernCommands = []string{"mkern", "kern", "mskip", "hskip", "hspace"}

// ignoredCommands change nothing about what the body means.
var ignoredCommands = []string{"ensuremath", "relax", "protect", "mathord", "nobreak"}

// transposeBody reports whether the body is nothing but a transpose script, and whether its sign is
// an italic T (the MathML must then be searched for an italic T too).
func transposeBody(body []token) (script, italic bool) {
	flat := withoutBraces(withoutSpacing(body))
	if len(flat) < 2 || flat[0].kind != '^' {
		return false, false
	}
	sign := flat[1:]
	switch {
	case len(sign) == 1 && (sign[0].is('c', "top") || sign[0].is('c', "intercal")):
		return true, false
	case len(sign) == 1 && sign[0].is('x', "T"):
		return true, true
	case len(sign) == 2 && sign[0].kind == 'c' && slices.Contains(fonts, sign[0].text) && sign[1].is('x', "T"):
		return true, false
	}
	return false, false
}

// withoutSpacing drops the spacing commands, their dimensions and the commands that change nothing.
func withoutSpacing(tokens []token) []token {
	kept := []token{}
	for index := 0; index < len(tokens); index++ {
		item := tokens[index]
		switch {
		case item.kind != 'c':
			kept = append(kept, item)
		case slices.Contains(kernCommands, item.text):
			index = skipDimension(tokens, index+1) - 1
		case spacing(item) || slices.Contains(ignoredCommands, item.text):
		default:
			kept = append(kept, item)
		}
	}
	return kept
}

// skipDimension is the index after a TeX dimension at index: a sign, digits and a point, a two-letter
// unit; a braced \hspace argument is one group.
func skipDimension(tokens []token, index int) int {
	if index < len(tokens) && tokens[index].kind == '{' {
		_, next := readArgument(tokens, index)
		return next
	}
	for index < len(tokens) && tokens[index].kind == 'x' && strings.Contains("+-.,0123456789", tokens[index].text) {
		index++
	}
	for range 2 {
		if index < len(tokens) && tokens[index].kind == 'x' && isLetter([]rune(tokens[index].text)[0]) {
			index++
		}
	}
	return index
}

// transposeMacro: the macro is a transpose script (a parameterless body that is not a paired delimiter).
func transposeMacro(macro Macro) (script, italic bool) {
	if macro.Parameters != 0 || macro.Paired {
		return false, false
	}
	return transposeBody(tokenize(macro.Body))
}

// italicText marks a transpose token whose sign is an italic T.
func italicText(italic bool) string {
	if italic {
		return "italic"
	}
	return ""
}
