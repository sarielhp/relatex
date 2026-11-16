package formulatex

import (
	"strings"

	"github.com/sarielhp/relatex/internal/xmltree"
)

// arrowFills are the operators of amsmath's extensible arrows, in the order their fill sets them
// (the bar \relbar repeats as one leader), by command.
var arrowFills = map[string]string{"−→": "xrightarrow", "←−": "xleftarrow"}

// extensibleArrow is \xrightarrow[below]{above} and \xleftarrow: an arrow fill under or over its
// labels, each label padded on the arrow's open side by a thin space the command adds itself.
func extensibleArrow(node *xmltree.Node, elements []*xmltree.Node) (string, bool) {
	command, ok := arrowCommand(node, elements)
	if !ok {
		return "", false
	}
	labels := elements[1:]
	if node.Name == "munderover" {
		labels = []*xmltree.Node{elements[2], elements[1]}
	}
	texts := make([]string, len(labels))
	for index, label := range labels {
		text, ok := arrowLabel(label, command == "xleftarrow")
		if !ok {
			return "", false
		}
		texts[index] = text
	}
	below := ""
	if len(texts) == 2 {
		below = "[" + texts[1] + "]"
	}
	return "\\" + command + below + braced(texts[0]), true
}

// arrowLabel is a label without the thin space after it (before it, for a left arrow).
func arrowLabel(label *xmltree.Node, left bool) (string, bool) {
	inner := label.Elements()
	if label.Name != "mrow" || len(inner) == 0 {
		return "", false
	}
	if left {
		return sequence(inner[1:]), thinSpace(inner[0])
	}
	return sequence(inner[:len(inner)-1]), thinSpace(inner[len(inner)-1])
}

// arrowCommand is the extensible arrow an mover or munderover sets: its base an mpadded holding
// an arrow fill.
func arrowCommand(node *xmltree.Node, elements []*xmltree.Node) (string, bool) {
	if node.Name == "munder" || elements[0].Name != "mpadded" {
		return "", false
	}
	var fill strings.Builder
	collectOperators(elements[0], &fill)
	command, ok := arrowFills[fill.String()]
	return command, ok
}

func collectOperators(node *xmltree.Node, fill *strings.Builder) {
	if node.Name == "mo" {
		fill.WriteString(node.Content())
	}
	for _, child := range node.Elements() {
		collectOperators(child, fill)
	}
}

// stackedSymbol: \underset or \overset around a symbol (an mo that is not an operator with
// limits nor an accent). amsmath gives it the symbol's class: a relation stays a relation.
func stackedSymbol(node *xmltree.Node) bool {
	switch node.Name {
	case "munder", "mover", "munderover":
	default:
		return false
	}
	elements := node.Elements()
	if len(elements) < 2 || elements[0].Name != "mo" || elements[0].Attrs["movablelimits"] != "" {
		return false
	}
	return node.Name != "mover" || accents[accentText(elements[1])] == ""
}

// symbolKind is a stacked symbol's class, read from the spaces TeX put beside it: a relation's
// thick space, a binary operator's medium one, else an ordinary atom.
func symbolKind(elements []*xmltree.Node, index int) string {
	for _, at := range []int{index + 1, index - 1} {
		if at < 0 || at >= len(elements) {
			continue
		}
		switch namedWidth(elements[at]) {
		case thickWidth:
			return "rel"
		case mediumWidth:
			return "bin"
		}
	}
	return "ord"
}

// scriptedOperator: an operator symbol (\sum, \mathop{\to}) with limits or scripts; luamml
// writes the operator's spaces on the symbol's mo.
func scriptedOperator(node *xmltree.Node) bool {
	switch node.Name {
	case "munder", "mover", "munderover", "msub", "msup", "msubsup":
	default:
		return false
	}
	elements := node.Elements()
	if len(elements) < 2 || elements[0].Name != "mo" {
		return false
	}
	return elements[0].Attrs["movablelimits"] != "" || largeOperators[elements[0].Content()]
}

// isExtensibleArrow: the element is \xrightarrow or \xleftarrow, a relation.
func isExtensibleArrow(node *xmltree.Node) bool {
	switch node.Name {
	case "mover", "munderover":
		if len(node.Elements()) < 2 {
			return false
		}
		_, ok := arrowCommand(node, node.Elements())
		return ok
	}
	return false
}
