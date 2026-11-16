package formulatex

import (
	"math"
	"strings"

	"github.com/sarielhp/relatex/internal/xmltree"
)

// padded writes the mpadded constructs TeX's boxes make: \text (with math inside), a \big
// delimiter and the struts its box keeps.
func padded(node *xmltree.Node) string {
	core := unwrapPadded(node)
	if text, ok := textBox(core); ok {
		return text
	}
	if len(core) >= 2 && core[0].Name == "mo" && allSpaces(core[1:]) {
		return bigDelimiter(core[0], core[1])
	}
	if anchor, ok := raisedAnchor(node, core); ok {
		return anchor
	}
	if tex, ok := composites[signature([]*xmltree.Node{node})]; ok {
		return tex
	}
	fail("mpadded around %s", names(core))
	return ""
}

// raisedAnchor is an empty box raised out of the line, as hyperref sets a link target in a
// formula: \smash{\raisebox{...}{}}.
func raisedAnchor(node *xmltree.Node, core []*xmltree.Node) (string, bool) {
	if len(core) != 1 || core[0].Name != "mspace" || len(core[0].Elements()) > 0 {
		return "", false
	}
	for _, size := range []string{"width", "height", "depth"} {
		if length(core[0].Attrs[size]) != 0 {
			return "", false
		}
	}
	for current := node; current.Name == "mpadded" || current.Name == "mrow"; {
		if raise := length(current.Attrs["voffset"]); raise > 0 {
			return "\\smash{\\raisebox{" + bigPoints(raise) + "}{}}", true
		}
		elements := current.Elements()
		if len(elements) != 1 {
			break
		}
		current = elements[0]
	}
	return "", false
}

// paddedKind is an mpadded's atom class: \bigl and \bigr make opening and closing atoms, a plain
// \big and \text ordinary ones.
func paddedKind(node *xmltree.Node) string {
	core := unwrapPadded(node)
	if len(core) < 2 || core[0].Name != "mo" || !allSpaces(core[1:]) {
		return "ord"
	}
	switch text := core[0].Content(); {
	case openings[text]:
		return "open"
	case closings[text]:
		return "close"
	}
	return "ord"
}

// unwrapPadded is what a chain of mpadded and mrow elements holds.
// Empty groups (an author's \mathopen{} before a \left inside \big) are left out: they typeset
// nothing.
func unwrapPadded(node *xmltree.Node) []*xmltree.Node {
	elements := withoutEmptyRows(node.Elements())
	for len(elements) == 1 && (elements[0].Name == "mpadded" || elements[0].Name == "mrow") {
		elements = withoutEmptyRows(elements[0].Elements())
	}
	return elements
}

func withoutEmptyRows(elements []*xmltree.Node) []*xmltree.Node {
	kept := make([]*xmltree.Node, 0, len(elements))
	for _, element := range elements {
		if element.Name != "mrow" || len(element.Elements()) > 0 || element.Content() != "" {
			kept = append(kept, element)
		}
	}
	return kept
}

// textBox is \text: a box of text pieces, each in its own mpadded, and math between them. luamml
// starts a new piece of text at a kern (the one between "v" and "e"), not at a space, so two
// pieces of text join without one; a piece moved right (lspace) starts with a space.
func textBox(core []*xmltree.Node) (string, bool) {
	var builder strings.Builder
	previous, texts := "", 0
	for _, piece := range core {
		inner := []*xmltree.Node{piece}
		if piece.Name == "mpadded" {
			inner = unwrapPadded(piece)
		}
		switch {
		case len(inner) == 1 && inner[0].Name == "mtext":
			if previous == "" && piece.Name == "mpadded" && leadingSpace(piece) {
				builder.WriteByte(' ')
			}
			builder.WriteString(textEscape(inner[0].Content()))
			previous, texts = "text", texts+1
		case piece.Name == "mpadded" && len(inner) > 0 && allSpaces(inner):
			// The space after math in text is a piece of its own.
			builder.WriteByte(' ')
			previous = "space"
		case len(inner) > 0 && piece.Name == "mpadded":
			// Math in text takes the style around it: luamml writes that style as an mstyle.
			if len(inner) == 1 && inner[0].Name == "mstyle" {
				inner = inner[0].Elements()
			}
			if previous == "text" {
				builder.WriteByte(' ')
			}
			builder.WriteString("$" + sequence(inner) + "$")
			previous = "math"
		default:
			return "", false
		}
	}
	if texts == 0 {
		return "", false
	}
	return "\\text" + braced(builder.String()), true
}

// leadingSpace: the first piece of a text box stands right of the box's start.
func leadingSpace(piece *xmltree.Node) bool {
	for current := piece; current.Name == "mpadded" || current.Name == "mrow"; {
		if current.Name == "mpadded" && length(current.Attrs["lspace"]) > 0 {
			return true
		}
		elements := current.Elements()
		if len(elements) != 1 {
			break
		}
		current = elements[0]
	}
	return false
}

func allSpaces(elements []*xmltree.Node) bool {
	for _, element := range elements {
		if element.Name != "mspace" {
			return false
		}
	}
	return true
}

func names(elements []*xmltree.Node) string {
	list := make([]string, len(elements))
	for index, element := range elements {
		list[index] = element.Name
	}
	return strings.Join(list, ",")
}

var (
	openings = map[string]bool{"(": true, "[": true, "{": true, "⟨": true, "⌊": true, "⌈": true}
	closings = map[string]bool{")": true, "]": true, "}": true, "⟩": true, "⌋": true, "⌉": true}
)

// bigSizes are \big, \Big, \bigg and \Bigg, by their height in \big's (1.2 times a 10 pt strut,
// 11.955 bp).
var bigSizes = []struct {
	command string
	ratio   float64
}{{"big", 1}, {"Big", 1.5}, {"bigg", 2}, {"Bigg", 2.5}}

// bigDelimiter is \big( and its sizes, read from the strut that sizes it; the side (\bigl,
// \bigr) is the delimiter's.
func bigDelimiter(fence, strut *xmltree.Node) string {
	ratio := length(strut.Attrs["height"]) / 11.955
	command := bigSizes[0].command
	best := math.Inf(1)
	for _, size := range bigSizes {
		if distance := math.Abs(ratio - size.ratio); distance < best {
			command, best = size.command, distance
		}
	}
	text := fence.Content()
	switch {
	case openings[text]:
		command += "l"
	case closings[text]:
		command += "r"
	}
	return join([]string{"\\" + command, delimiter(fence)})
}
