package formulatex

import "github.com/sarielhp/relatex/internal/xmltree"

// TeX puts a thin space between an operator or an inner atom (\left...\right, \ldots) and an
// ordinary, operator or inner atom beside it. luamml writes it as an mspace when neither neighbor
// is an mo (an mo carries its spacing itself). The TeX written for a row must make exactly the
// mspaces the row has: such a thin space is left to TeX, a word of upright letters is an operator
// only where its neighbors show TeX's spaces, and an inner atom that lacks them is braced (an
// ordinary atom). TeX puts that space after any glue between the two atoms (\!, \hspace), right
// before the second atom: it is the last mspace before an atom, never the first after one.

// sequence writes a row of elements.
func sequence(elements []*xmltree.Node) string { return sequenceAfter(elements, false) }

// leadingAtom stands for the empty group amsmath sets before a left-aligned cell (the {} that lets
// "&=" space its relation): TeX spaces the cell's first atom against it.
var leadingAtom = &xmltree.Node{Name: "mrow"}

// sequenceAfter writes a row of elements, after an ordinary atom when leadingOrd.
func sequenceAfter(elements []*xmltree.Node, leadingOrd bool) string {
	if leadingOrd {
		elements = append([]*xmltree.Node{leadingAtom}, elements...)
	}
	kinds := atomKinds(elements)
	automatic := automaticSpaces(elements, kinds)
	pieces := []string{}
	for index := 0; index < len(elements); index++ {
		node := elements[index]
		if node == leadingAtom {
			continue
		}
		if tex, width := compositeAt(elements, index); width > 0 {
			pieces = append(pieces, tex)
			index += width - 1
			continue
		}
		if automatic[index] || node.Name == "mspace" && thinSpace(node) && closingDots(elements, index) {
			continue
		}
		if environment, width := fencedMatrix(elements, index); width > 0 {
			pieces = append(pieces, environment)
			index += width - 1
			continue
		}
		if bareMatrix(elements, index) {
			pieces = append(pieces, matrix(elements[index+1], "matrix"))
			index += 2
			continue
		}
		if dots, ok := dotsCommand(node); ok && dots == "\\ldots" && closingDots(elements, index+1) {
			pieces = append(pieces, "\\dots")
			continue
		}
		if lowDots(elements, index) {
			pieces = append(pieces, "\\ldots")
			index += 2
			continue
		}
		pieces = append(pieces, atomTeX(node, kinds[index]))
	}
	return join(pieces)
}

// atomTeX is an element in the class the row gave it.
func atomTeX(node *xmltree.Node, kind string) string {
	if word, ok := uprightWord(node); ok {
		return wordTeX(word, kind == "op")
	}
	switch kind {
	case "braced":
		return braced(element(node))
	case "stacked":
		count := map[string]int{"munder": 2, "mover": 2, "munderover": 3}[node.Name]
		return stacked(node.Name, children(node, count))
	}
	return element(node)
}

func thinSpace(node *xmltree.Node) bool {
	return node.Name == "mspace" && node.Attrs["width"] == "0.167em" && len(node.Attrs) == 1
}

// TeX's spaces between atoms, as luamml writes them.
const (
	thinWidth   = "0.167em"
	mediumWidth = "0.222em"
	thickWidth  = "0.278em"
)

// atomSpaces are the spaces TeX puts between atoms (The TeXbook, chapter 18), among the atoms
// luamml writes as elements other than an mo (an mo carries its spacing itself): an ordinary
// atom (also a braced group or a word not yet settled), an operator, a binary operator and a
// relation built of several elements (\xrightarrow, \underset{x}{=}), the opening and closing
// atoms of \bigl and \bigr, and an inner atom.
var atomSpaces = map[string]map[string]string{
	"ord":   {"op": thinWidth, "inner": thinWidth, "bin": mediumWidth, "rel": thickWidth},
	"op":    {"ord": thinWidth, "op": thinWidth, "inner": thinWidth, "rel": thickWidth},
	"bin":   {"ord": mediumWidth, "op": mediumWidth, "open": mediumWidth, "inner": mediumWidth},
	"rel":   {"ord": thickWidth, "op": thickWidth, "open": thickWidth, "inner": thickWidth},
	"close": {"op": thinWidth, "inner": thinWidth, "bin": mediumWidth, "rel": thickWidth},
	"inner": {"ord": thinWidth, "op": thinWidth, "open": thinWidth, "inner": thinWidth, "bin": mediumWidth, "rel": thickWidth},
}

// spaceBetween is the width of the space TeX puts between atoms of these kinds; "" for none.
func spaceBetween(left, right string) string { return atomSpaces[pairKind(left)][pairKind(right)] }

func pairKind(kind string) string {
	if kind == "braced" || kind == "word" || kind == "stacked" {
		return "ord"
	}
	return kind
}

// namedWidth is a space's width when it is a space alone (no height or other attribute).
func namedWidth(node *xmltree.Node) string {
	if node.Name != "mspace" || len(node.Attrs) != 1 {
		return ""
	}
	return node.Attrs["width"]
}

// stackable: munder, mover or munderover around an operator name.
func stackable(node *xmltree.Node) bool {
	switch node.Name {
	case "munder", "mover", "munderover":
		elements := node.Elements()
		if len(elements) == 0 {
			return false
		}
		_, word := uprightWord(elements[0])
		return word
	}
	return false
}

// atomKinds are the elements' atom classes, words and inner atoms settled by their neighbors.
func atomKinds(elements []*xmltree.Node) []string {
	kinds := make([]string, len(elements))
	for index, node := range elements {
		kinds[index] = atomKind(node)
		if kinds[index] == "symbol" {
			kinds[index] = symbolKind(elements, index)
		}
	}
	for index, node := range elements {
		switch kinds[index] {
		case "word":
			word, _ := uprightWord(node)
			kinds[index] = "ord"
			before, after := spacedSides(elements, kinds, index, "op")
			if before.consistent() && after.consistent() && (before.spaced || after.spaced || operatorNames[word]) {
				kinds[index] = "op"
			}
		case "inner":
			if before, after := spacedSides(elements, kinds, index, "inner"); !before.consistent() || !after.consistent() {
				kinds[index] = "braced"
			}
		case "op":
			// An operator name with limits but without an operator's spaces is the author's
			// \underset{...}{\lim}, an ordinary atom.
			if !stackable(node) {
				continue
			}
			if before, after := spacedSides(elements, kinds, index, "op"); !before.consistent() || !after.consistent() {
				kinds[index] = "stacked"
			}
		}
	}
	return kinds
}

// side is what stands on one side of an atom: whether thin spaces separate it from its
// neighbor, and whether that neighbor is one TeX would space against.
type side struct{ spaced, wanted bool }

// consistent: the side has the thin space TeX would put there (it may have one anyway, written).
func (s side) consistent() bool { return s.spaced || !s.wanted }

// spacedSides are the sides of the atom at index, were it of the kind given. TeX's space before
// it would be the mspace right before it; the space after it, the last mspace before the next
// atom.
func spacedSides(elements []*xmltree.Node, kinds []string, index int, kind string) (side, side) {
	at := index - 1
	for at >= 0 && isGlue(elements[at]) {
		at--
	}
	before := side{}
	if at >= 0 {
		want := spaceBetween(kinds[at], kind)
		before = side{spaced: spacedBy(elements[index-1], want), wanted: want != ""}
	}
	at = index + 1
	for at < len(elements) && isGlue(elements[at]) {
		at++
	}
	after := side{}
	if at > index+1 {
		after.spaced = spacedBy(elements[at-1], "")
	}
	if at < len(elements) {
		want := spaceBetween(kind, kinds[at])
		after = side{spaced: at > index+1 && spacedBy(elements[at-1], want), wanted: want != ""}
	}
	return before, after
}

// spacedBy: the element is a thin space, or the space wanted.
func spacedBy(node *xmltree.Node, want string) bool {
	width := namedWidth(node)
	return width == thinWidth || width != "" && width == want
}

// automaticSpaces marks the spaces TeX puts there itself: the last mspace before an atom, when
// it is the space TeX puts between the atom before the glue and this one.
func automaticSpaces(elements []*xmltree.Node, kinds []string) []bool {
	automatic := make([]bool, len(elements))
	for end := 2; end < len(elements); end++ {
		if isGlue(elements[end]) || namedWidth(elements[end-1]) == "" {
			continue
		}
		start := end - 1
		for start > 0 && isGlue(elements[start-1]) {
			start--
		}
		if start > 0 && namedWidth(elements[end-1]) == spaceBetween(kinds[start-1], kinds[end]) {
			automatic[end-1] = true
		}
	}
	return automatic
}

// isGlue: an mspace of width alone (a space or kern; a strut's mspace is a box, an atom).
func isGlue(node *xmltree.Node) bool {
	return node.Name == "mspace" && length(node.Attrs["height"]) == 0 && length(node.Attrs["depth"]) == 0
}
