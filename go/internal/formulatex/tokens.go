package formulatex

import (
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/mathtokens"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// variantCommands are the alphabet commands of amsmath, amssymb and bm (\boldsymbol is bm's \bm once
// bm is loaded), by MathML variant.
var variantCommands = map[string]string{"normal": "mathrm", "bold": "mathbf", "bold-italic": "boldsymbol",
	"script": "mathcal", "double-struck": "mathbb", "fraktur": "mathfrak", "sans-serif": "mathsf",
	"monospace": "mathtt", "italic": "mathit"}

// specials are ASCII characters TeX math writes escaped.
var specials = map[string]string{"{": `\{`, "}": `\}`, "%": `\%`, "#": `\#`, "&": `\&`, "$": `\$`, "_": `\_`, `\`: `\backslash`}

// token writes an mi, mo or mn: the symbol table first, then by its characters.
func token(node *xmltree.Node) string {
	text := node.Content()
	if text == "" {
		return "{}"
	}
	if tex, ok := symbols[symbolKey{node.Name, text, node.Attrs["mathvariant"]}]; ok {
		return tex
	}
	if node.Name == "mo" {
		return classed(node, operator(text))
	}
	return alphanumeric(text, node.Attrs["mathvariant"])
}

// naturalClasses are the ASCII characters TeX sets as binary operators or relations by their
// math codes; any other character spaced as one was made one by the author (an e-mail address's
// @ as \mathbin).
var naturalClasses = map[string]bool{"+": true, "-": true, "*": true, "=": true, "<": true, ">": true, ":": true}

// classed is an mo's TeX in the class its spacing shows, when its character has another class.
func classed(node *xmltree.Node, tex string) string {
	text := node.Content()
	if len(text) != 1 || naturalClasses[text] || node.Attrs["lspace"] != node.Attrs["rspace"] {
		return tex
	}
	switch node.Attrs["lspace"] {
	case mediumWidth:
		return "\\mathbin" + braced(tex)
	case thickWidth:
		return "\\mathrel" + braced(tex)
	}
	return tex
}

func operator(text string) string {
	if utf8.RuneCountInString(text) == 1 && text[0] < utf8.RuneSelf {
		if tex, ok := specials[text]; ok {
			return tex
		}
		return text
	}
	pieces := []string{}
	for _, character := range text {
		tex, ok := symbols[symbolKey{"mo", string(character), ""}]
		if !ok {
			fail("operator %q", string(character))
		}
		pieces = append(pieces, tex)
	}
	return join(pieces)
}

// alphanumeric writes an identifier or number by runs of one alphabet: a run of italic Latin
// letters is plain TeX (several letters as \mathit), any other variant gets its command.
func alphanumeric(text, attribute string) string {
	pieces := []string{}
	for _, run := range alphabetRuns(text, attribute) {
		pieces = append(pieces, alphabetRun(run.text, run.variant))
	}
	return join(pieces)
}

type alphabetText struct{ text, variant string }

func alphabetRuns(text, attribute string) []alphabetText {
	runs := []alphabetText{}
	for _, character := range text {
		base, variant, ok := mathtokens.DecodeAlphabet(character)
		if !ok {
			base, variant = string(character), attribute
		}
		if count := len(runs); count > 0 && runs[count-1].variant == variant {
			runs[count-1].text += base
			continue
		}
		runs = append(runs, alphabetText{base, variant})
	}
	return runs
}

func alphabetRun(text, variant string) string {
	inner := []string{}
	for _, character := range text {
		inner = append(inner, baseCharacter(string(character)))
	}
	body := join(inner)
	plainLetters := isPlain(text)
	switch {
	case variant == "" && plainLetters, variant == "italic" && plainLetters && utf8.RuneCountInString(text) == 1:
		return body
	case variant == "" || variant == "normal" && !plainLetters:
		return body
	}
	command, ok := variantCommands[variant]
	if !ok {
		fail("alphabet %s", variant)
	}
	return "\\" + command + braced(body)
}

// isPlain: only ASCII letters and digits.
func isPlain(text string) bool {
	for index := 0; index < len(text); index++ {
		character := text[index]
		if !isASCIILetter(character) && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

// baseCharacter is the TeX of an unstyled character: itself, or the command the symbol table
// knows for it (a Greek letter as math italic or upright).
func baseCharacter(character string) string {
	if isPlain(character) {
		return character
	}
	if tex, ok := specials[character]; ok {
		return tex
	}
	for _, element := range []string{"mi", "mo"} {
		for _, variant := range []string{"", "normal"} {
			if tex, ok := symbols[symbolKey{element, character, variant}]; ok {
				return tex
			}
		}
	}
	if italic, ok := mathtokens.EncodeAlphabet(character, "italic"); ok {
		if tex, known := symbols[symbolKey{"mi", italic, ""}]; known {
			return tex
		}
	}
	if utf8.RuneCountInString(character) == 1 && character[0] < utf8.RuneSelf {
		return character
	}
	fail("character %q", character)
	return ""
}

// operatorNames are amsmath's named operators.
var operatorNames = map[string]bool{"arccos": true, "arcsin": true, "arctan": true, "arg": true, "cos": true,
	"cosh": true, "cot": true, "coth": true, "csc": true, "deg": true, "det": true, "dim": true, "exp": true, "gcd": true,
	"hom": true, "inf": true, "ker": true, "lg": true, "lim": true, "liminf": true, "limsup": true, "ln": true, "log": true,
	"max": true, "min": true, "Pr": true, "sec": true, "sin": true, "sinh": true, "sup": true, "tan": true, "tanh": true}

// uprightWord is the text of an mrow of upright letters (an operator name or \mathrm word).
func uprightWord(node *xmltree.Node) (string, bool) {
	if node.Name != "mrow" {
		return "", false
	}
	elements := node.Elements()
	if len(elements) < 2 {
		return "", false
	}
	var builder strings.Builder
	for index, child := range elements {
		text := child.Content()
		// \operatorname sets a hyphen as itself (\operatorname{non-qd}), not as a minus.
		if child.Name == "mo" && text == "-" && index > 0 && index < len(elements)-1 && len(child.Elements()) == 0 {
			builder.WriteString(text)
			continue
		}
		if child.Name != "mi" || child.Attrs["mathvariant"] != "normal" || len(child.Elements()) > 0 || !isPlain(text) {
			return "", false
		}
		builder.WriteString(text)
	}
	return builder.String(), true
}

// operatorName is the TeX of a word set as an operator: amsmath's own command, else \operatorname.
func operatorName(word string) string {
	if operatorNames[word] {
		return "\\" + word
	}
	return "\\operatorname" + braced(word)
}
