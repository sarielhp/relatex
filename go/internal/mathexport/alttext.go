package mathexport

import (
	"slices"
	"strings"
	"unicode"
)

// AltText is a formula's text alternative (decision 088): a linear form of its MathML for
// software without MathML, in the notation of LaTeX and UnicodeMath (x_i^2, (a)/(b), √(x)).
// The collector reads formulas from what TeX typeset, so no TeX source is at hand; the linear
// form is read from the MathML alone, and the acceptance reads it again on its own
// (lib/relatex/math_alttext.rb). Math italic letters read as plain letters; other alphabets
// (bold, script, double-struck) keep their characters, which carry meaning.
func AltText(root *Node) string {
	text := strings.Map(plainLetter, linear(root))
	return strings.Join(strings.FieldsFunc(text, unicode.IsSpace), " ")
}

var (
	altTokens  = []string{"mi", "mn", "mo", "mtext", "ms"}
	altSilent  = []string{"mphantom", "none", "mprescripts", "annotation", "annotation-xml"}
	altWrapper = []string{"mrow", "mstyle", "mpadded"}
)

// altScripts are the script elements: the marks before each child after the base.
var altScripts = map[string][]string{"msub": {"_"}, "msup": {"^"}, "msubsup": {"_", "^"},
	"munder": {"_"}, "mover": {"^"}, "munderover": {"_", "^"}}

func linear(node *Node) string {
	if node.Text {
		return node.Value()
	}
	if slices.Contains(altTokens, node.Name) || slices.Contains(altSilent, node.Name) {
		return tokenText(node)
	}
	children := node.Elements()
	if marks, ok := altScripts[node.Name]; ok && len(children) == len(marks)+1 {
		result := linear(children[0])
		for index, mark := range marks {
			result += mark + group(children[index+1])
		}
		return result
	}
	return structureText(node, children)
}

// tokenText is a token's characters, and nothing for an element that shows nothing.
func tokenText(node *Node) string {
	if slices.Contains(altSilent, node.Name) {
		return ""
	}
	var builder strings.Builder
	for _, child := range node.Children {
		builder.WriteString(linear(child))
	}
	return builder.String()
}

// structureText reads fractions, roots, tables and space; any other element is its children in
// order (semantics by its first child, the presentation).
func structureText(node *Node, children []*Node) string {
	switch {
	case node.Name == "mfrac" && len(children) == 2:
		return group(children[0]) + "/" + group(children[1])
	case node.Name == "msqrt":
		return "√(" + linearJoined(children, "") + ")"
	case node.Name == "mroot" && len(children) == 2:
		return "√[" + linear(children[1]) + "](" + linear(children[0]) + ")"
	case node.Name == "mspace":
		return " "
	case node.Name == "mtable":
		return linearJoined(children, "; ")
	case node.Name == "mtr" || node.Name == "mlabeledtr":
		return linearJoined(children, " ")
	case node.Name == "semantics" && len(children) > 0:
		return linear(children[0])
	}
	var builder strings.Builder
	for _, child := range node.Children {
		builder.WriteString(linear(child))
	}
	return builder.String()
}

func linearJoined(children []*Node, separator string) string {
	parts := make([]string, len(children))
	for index, child := range children {
		parts[index] = linear(child)
	}
	return strings.Join(parts, separator)
}

// group is an operand of a fraction or a script: bare when it is one token or one character,
// in parentheses otherwise.
func group(node *Node) string {
	for slices.Contains(altWrapper, node.Name) && len(node.Elements()) == 1 {
		node = node.Elements()[0]
	}
	text := strings.Join(strings.FieldsFunc(linear(node), unicode.IsSpace), " ")
	if slices.Contains(altTokens, node.Name) || len([]rune(text)) <= 1 {
		return text
	}
	return "(" + text + ")"
}

// plainLetter reads a math italic letter (U+1D434–U+1D467, U+210E, U+1D6E2–U+1D71B) as its
// plain letter, and drops the invisible operators (U+2061–U+2064) and the zero-width space.
func plainLetter(character rune) rune {
	switch {
	case character == 0x200B || character >= 0x2061 && character <= 0x2064:
		return -1
	case character == 0x210E:
		return 'h'
	case character >= 0x1D434 && character <= 0x1D467:
		offset := character - 0x1D434
		if offset < 26 {
			return 'A' + offset
		}
		return 'a' + offset - 26
	case character >= 0x1D6E2 && character <= 0x1D71B:
		return italicGreek[character-0x1D6E2]
	}
	return character
}

// italicGreek are the plain letters of U+1D6E2–U+1D71B, in order.
var italicGreek = []rune("ΑΒΓΔΕΖΗΘΙΚΛΜΝΞΟΠΡϴΣΤΥΦΧΨΩ∇αβγδεζηθικλμνξοπρςστυφχψω∂ϵϑϰϕϱϖ")
