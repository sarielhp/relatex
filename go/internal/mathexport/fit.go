package mathexport

import (
	"regexp"
	"strconv"
)

// pageRootPoints is the text size of the page's root font (16px) in CSS points: body text is set
// at 1rem whatever the TeX size of the document was, so a 10pt document's text is drawn 1.2 times
// larger than the TeX geometry measured on its own 10pt text.
const pageRootPoints = 12

var pointLength = regexp.MustCompile(`\A(-?(?:\d+(?:\.\d*)?|\.\d+))pt\z`)

// lengthAttributes are the dimension attributes of the TeX boxes the MathML pins text and rows with.
var lengthAttributes = map[string][]string{
	"mpadded": {"width", "height", "depth", "lspace", "voffset"},
	"mspace":  {"width", "height", "depth"},
}

// FitToPage makes a formula's absolute point geometry agree with its text on the page: it reaches
// out for rows that lean left of their box (a stack of aligned display rows, whose box width
// counts only what stands right of the alignment point), then scales the point lengths by the
// ratio of the page's text size (the root font) to the document's body text size in scaled points.
// A 12pt document keeps its lengths.
func FitToPage(root *Node, base int64) {
	for _, node := range root.Descendants() {
		reachLeft(node)
	}
	if base <= 0 || base == pageRootPoints*65_536 {
		return
	}
	factor := float64(pageRootPoints*65_536) / float64(base)
	for _, node := range root.Descendants() {
		for _, name := range lengthAttributes[node.Name] {
			scalePoints(node, name, factor)
		}
	}
}

// pointValue is an attribute holding a plain length in points.
func pointValue(node *Node, name string) (float64, bool) {
	source, present := node.Attribute(name)
	if !present {
		return 0, false
	}
	match := pointLength.FindStringSubmatch(source)
	if match == nil {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	return value, err == nil
}

func setPoints(node *Node, name string, value float64) {
	node.SetAttribute(name, strconv.FormatFloat(value, 'f', 6, 64)+"pt")
}

func scalePoints(node *Node, name string, factor float64) {
	if value, ok := pointValue(node, name); ok {
		setPoints(node, name, value*factor)
	}
}

// reachLeft widens an mpadded whose row of zero-width pieces leans left of it (each piece pinned
// by a negative lspace): the box starts that far left, so its cell holds the rows instead of
// letting them overflow over the cells before it (the equation number set over the row).
func reachLeft(node *Node) {
	width, ok := pointValue(node, "width")
	if node.Name != "mpadded" || !ok {
		return
	}
	children := node.Elements()
	if len(children) != 1 || children[0].Name != "mrow" {
		return
	}
	lean := 0.0
	for _, piece := range children[0].Elements() {
		pieceWidth, sized := pointValue(piece, "width")
		offset, shifted := pointValue(piece, "lspace")
		if piece.Name != "mpadded" || !sized || pieceWidth != 0 || !shifted {
			return
		}
		lean = min(lean, offset)
	}
	if lean >= 0 {
		return
	}
	own, _ := pointValue(node, "lspace")
	setPoints(node, "lspace", own-lean)
	setPoints(node, "width", width-lean)
}
