package authormath

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// equationNumber is MathStructure::NUMBER: an equation number's text, not compared.
var equationNumber = regexp.MustCompile(`\A\([^()\t\n\v\f\r ]{1,12}\)\z`)

// projected is MathStructure's projection of an element: its name, its meaning-bearing
// attributes and either its text (a leaf) or its projected children.
type projected struct {
	name       string
	attributes []string
	text       string
	children   []*projected
	leaf       bool
}

// difference is MathStructure.difference: "" when the two MathML trees have the same
// structure, else where they first differ. The structure folds what differs between the
// paper's fonts and the standard preamble: an mrow of one element is that element, empty mrow
// and mtext elements (anchors) go, adjacent identifiers of one variant are one (\rm RMS), and
// an equation number's text is not compared.
func difference(original, typeset string) string {
	return compare(projection(original), projection(typeset), "")
}

func projection(mathml string) *projected {
	root, err := xmltree.Parse(mathml)
	if err != nil {
		// Nokogiri recovers what it can; a document without a root projects to nothing.
		return &projected{}
	}
	return project(root)
}

func project(element *xmltree.Node) *projected {
	children := []*projected{}
	for _, child := range element.Elements() {
		if projectedChild := project(child); !empty(projectedChild) {
			children = append(children, projectedChild)
		}
	}
	children = mergeIdentifiers(children)
	if element.Name == "mrow" && len(children) == 1 {
		return children[0]
	}
	attributes := []string{}
	if value, ok := element.Attrs["mathvariant"]; ok {
		attributes = append(attributes, "mathvariant="+value)
	}
	if len(children) > 0 {
		return &projected{name: element.Name, attributes: attributes, children: children}
	}
	text := strings.TrimFunc(element.Content(), isRubySpace)
	if element.Name == "mtext" && equationNumber.MatchString(text) {
		text = "(N)"
	}
	return &projected{name: element.Name, attributes: attributes, text: text, leaf: true}
}

// isRubySpace is what Ruby's String#strip removes.
func isRubySpace(character rune) bool {
	return character == ' ' || (character >= '\t' && character <= '\r') || character == 0
}

func empty(node *projected) bool {
	return (node.name == "mrow" || node.name == "mtext") && node.leaf && node.text == ""
}

func sameAttributes(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func mergeIdentifiers(children []*projected) []*projected {
	merged := []*projected{}
	for _, child := range children {
		if len(merged) > 0 {
			previous := merged[len(merged)-1]
			if previous.name == "mi" && child.name == "mi" && sameAttributes(previous.attributes, child.attributes) && child.leaf {
				merged[len(merged)-1] = &projected{name: "mi", attributes: previous.attributes, text: previous.text + child.text, leaf: true}
				continue
			}
		}
		merged = append(merged, child)
	}
	return merged
}

func compare(left, right *projected, path string) string {
	if left.name != right.name || !sameAttributes(left.attributes, right.attributes) {
		return fmt.Sprintf("%s: %s vs %s", path, left.name, right.name)
	}
	here := path + "/" + left.name
	if left.leaf || right.leaf {
		return leafDifference(left, right, here)
	}
	if len(left.children) != len(right.children) {
		return fmt.Sprintf("%s: %d vs %d children", here, len(left.children), len(right.children))
	}
	for index := range left.children {
		if found := compare(left.children[index], right.children[index], fmt.Sprintf("%s[%d]", here, index+1)); found != "" {
			return found
		}
	}
	return ""
}

func leafDifference(left, right *projected, path string) string {
	if left.leaf && right.leaf && left.text == right.text {
		return ""
	}
	return fmt.Sprintf("%s: %s vs %s", path, prefix(inspect(left), 40), prefix(inspect(right), 40))
}

// inspect is Ruby's #inspect of a projection's third element: a string, or the array of
// projected children.
func inspect(node *projected) string {
	if node.leaf {
		return rec.InspectString(node.text)
	}
	parts := make([]string, len(node.children))
	for index, child := range node.children {
		parts[index] = inspectNode(child)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func inspectNode(node *projected) string {
	attributes := make([]string, len(node.attributes))
	for index, attribute := range node.attributes {
		attributes[index] = rec.InspectString(attribute)
	}
	return "[" + rec.InspectString(node.name) + ", [" + strings.Join(attributes, ", ") + "], " + inspect(node) + "]"
}

// prefix is text[0, count] by characters.
func prefix(text string, count int) string {
	if utf8.RuneCountInString(text) <= count {
		return text
	}
	return string([]rune(text)[:count])
}
