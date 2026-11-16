package formulatex

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/xmltree"
)

// unsupported is a construct the serializer does not write; its text names the construct, so the
// failures group into classes.
type unsupported string

// Write is a formula's TeX from its MathML, or an error naming the first construct it cannot
// write. The TeX is only a candidate: Check proves it by the round trip.
func Write(mathml string) (formula Formula, err error) {
	root, parseErr := xmltree.Parse(mathml)
	if parseErr != nil {
		return Formula{}, fmt.Errorf("unparsed MathML: %w", parseErr)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			formula, err = Formula{}, unsupportedError(recovered)
		}
	}()
	formula.Display = root.Attrs["display"] == "block"
	if formula.Display {
		if environment, text, ok := alignment(root); ok {
			formula.Environment, formula.Text = environment, text
			return formula, nil
		}
	}
	formula.Text = rowOf(root.Elements())
	return formula, nil
}

// unsupportedError is the error a fail panic carries; any other panic goes on.
func unsupportedError(recovered any) error {
	construct, ok := recovered.(unsupported)
	if !ok {
		panic(recovered)
	}
	return errors.New(string(construct))
}

func fail(format string, arguments ...any) { panic(unsupported(fmt.Sprintf(format, arguments...))) }

// join concatenates TeX pieces, with a space where a command name would run into a letter.
func join(pieces []string) string {
	var builder strings.Builder
	for _, piece := range pieces {
		if piece == "" {
			continue
		}
		if endsInCommandName(builder.String()) && startsWithLetter(piece) {
			builder.WriteByte(' ')
		}
		builder.WriteString(piece)
	}
	return builder.String()
}

func endsInCommandName(text string) bool {
	index := len(text) - 1
	for index >= 0 && isASCIILetter(text[index]) {
		index--
	}
	return index >= 0 && index < len(text)-1 && text[index] == '\\' && !escapedBackslash(text, index)
}

// escapedBackslash: the backslash at index is the second of "\\".
func escapedBackslash(text string, index int) bool {
	count := 0
	for at := index - 1; at >= 0 && text[at] == '\\'; at-- {
		count++
	}
	return count%2 == 1
}

func startsWithLetter(text string) bool {
	character, _ := utf8.DecodeRuneInString(text)
	return character < utf8.RuneSelf && isASCIILetter(byte(character))
}

func isASCIILetter(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

// group is an argument: a single character or command stays as it is, anything else is braced.
func group(text string) string {
	if utf8.RuneCountInString(text) == 1 || isCommand(text) || oneGroup(text) {
		return text
	}
	return "{" + text + "}"
}

// oneGroup: the text is one braced group, "{...}" whose first brace closes at its end.
func oneGroup(text string) bool {
	if len(text) < 2 || text[0] != '{' {
		return false
	}
	depth := 0
	for index := 0; index < len(text); index++ {
		switch {
		case text[index] == '\\':
			index++
		case text[index] == '{':
			depth++
		case text[index] == '}':
			depth--
			if depth == 0 {
				return index == len(text)-1
			}
		}
	}
	return false
}

func isCommand(text string) bool {
	if len(text) < 2 || text[0] != '\\' {
		return false
	}
	for index := 1; index < len(text); index++ {
		if !isASCIILetter(text[index]) {
			return false
		}
	}
	return true
}

// braced is an argument that is always braced (\frac{a}{b}, \text{...}).
func braced(text string) string {
	if oneGroup(text) {
		return text
	}
	return "{" + text + "}"
}

// element writes one MathML element.
func element(node *xmltree.Node) string {
	switch node.Name {
	case "mi", "mo", "mn":
		return token(node)
	case "mrow":
		return row(node)
	case "mtext":
		return "\\text" + braced(textEscape(node.Content()))
	case "mspace":
		return space(node)
	case "msub", "msup", "msubsup":
		return scripts(node)
	case "munder", "mover", "munderover":
		return limits(node)
	case "mfrac":
		return fraction(node)
	case "msqrt":
		return "\\sqrt" + braced(sequence(node.Elements()))
	case "mstyle":
		return style(node)
	case "mpadded":
		return padded(node)
	case "mphantom":
		return "\\phantom" + braced(sequence(node.Elements()))
	case "mtable":
		return arrayTable(node)
	}
	fail("element %s", node.Name)
	return ""
}

// children is an element's element children, of which there must be count.
func children(node *xmltree.Node, count int) []*xmltree.Node {
	elements := node.Elements()
	if len(elements) != count {
		fail("%s with %d children", node.Name, len(elements))
	}
	return elements
}

var textEscapes = strings.NewReplacer(`\`, `\textbackslash{}`, "{", `\{`, "}", `\}`, "$", `\$`, "%", `\%`, "#", `\#`,
	"&", `\&`, "_", `\_`, "^", `\^{}`, "~", `\~{}`)

func textEscape(text string) string { return textEscapes.Replace(text) }
