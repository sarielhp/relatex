package mathexport

import (
	"slices"

	"github.com/sarielhp/relatex/internal/mathintent"
)

// The constructs Dirac notation typesets (decision 114, amendment 2026-11-10). A ket is a bar, its
// state, then the closing angle; a bra the opening angle, its state, then the bar; a braket both
// around a middle bar. The bars and angles are the page's ordinary fences (an mo, or an mi when
// TeX sets the bar as an ordinary symbol), so each rule takes its marks from one row, left to
// right, and gives the whole construct the intent.

// diracBar is the bar a Dirac construct is written with.
const diracBar = "|"

// diracSpan is one bracket of a row: its opening mark and the mark that closes it.
type diracSpan struct{ open, shut *Node }

// kets: each | … ⟩ of one row taken left to right, the state before the angle its argument; the
// bars a bra already took are not the ket's, and none when a bar of the formula is left over.
func kets(root *Node) []placement {
	claimed := map[*Node]bool{}
	spans := braSpans(root)
	for _, span := range spans {
		claimed[span.shut] = true
	}
	found, placed := []placement{}, 0
	for _, row := range delimiterRows(root) {
		var open *Node
		for _, child := range row.Elements() {
			switch {
			case claimed[child]:
				continue
			case open == nil && barText(child) == diracBar:
				open = child
			case open != nil && barText(child) == diracBar:
				// The bar nearest the angle is the one that opens the ket.
				open = child
			case open != nil && angleText(child) == "⟩":
				found = append(found, diracPair(row, open, child, mathintent.Ket))
				open = nil
				placed++
			}
		}
		if open != nil {
			return nil
		}
	}
	if placed != countOperators(root, diracBar)-len(spans) {
		return nil
	}
	return found
}

// bras: each ⟨ … | of one row taken left to right, the state between them its argument; none when a
// bra's angle or bar of the formula is left over.
func bras(root *Node) []placement {
	spans := braSpans(root)
	if len(spans) != countOperators(root, "⟨") {
		return nil
	}
	found := make([]placement, len(spans))
	for index, span := range spans {
		found[index] = diracPair(span.open.Parent, span.open, span.shut, mathintent.Bra)
	}
	return found
}

// braSpans: each ⟨ … | of one row taken left to right; an angle with no bar after it stays alone.
func braSpans(root *Node) []diracSpan {
	spans := []diracSpan{}
	for _, row := range delimiterRows(root) {
		var open *Node
		for _, child := range row.Elements() {
			switch {
			case open == nil && angleText(child) == "⟨":
				open = child
			case open != nil && barText(child) == diracBar:
				spans = append(spans, diracSpan{open, child})
				open = nil
			}
		}
	}
	return spans
}

// brakets: each ⟨ … | … ⟩ of one row with one bar between and neither end nested; the two sides of
// the bar are its arguments, its intent inner-product (mathintent reads a braket as one).
func brakets(root *Node) []placement {
	return pairedInnerProducts(root, angleText, barBetween)
}

// barBetween is the index of the one bar between open and shut, -1 when there is not exactly one or
// a side is empty.
func barBetween(children []*Node, open, shut int) int {
	if open < 0 {
		return -1
	}
	bar := -1
	for index := open + 1; index < shut; index++ {
		if barText(children[index]) != diracBar {
			continue
		}
		if bar >= 0 {
			return -1
		}
		bar = index
	}
	if bar <= open+1 || bar >= shut-1 {
		return -1
	}
	return bar
}

// diracPair is the placement of a ket or a bra: a new mrow around the two marks carries the intent,
// the state between them is its argument.
func diracPair(row, open, shut *Node, concept mathintent.Concept) placement {
	return func(names *argNames) {
		children := row.Elements()
		start, stop := slices.Index(children, open), slices.Index(children, shut)
		if start < 0 || stop <= start+1 {
			return
		}
		argument := names.argument(slices.Clone(children[start+1 : stop]))
		setIntent(wrap(row, open, shut), call(concept, argument))
	}
}

// innerProductShapes: an inner product is written either ⟨ x , y ⟩ (a comma between, decision 114)
// or ⟨ x | y ⟩ (Dirac's braket). The two never mix in one formula: whichever shape accounts for
// every angle of the page is the one, and the other's would leave a mark over.
func innerProductShapes(root *Node) []placement {
	for _, find := range []func(*Node) []placement{innerProducts, brakets} {
		if found := find(root); len(found) > 0 {
			return found
		}
	}
	return nil
}

// angleText: the angle a node shows, or the text of an mi that holds one.
func angleText(node *Node) string {
	if node.Name == "mi" {
		return tokenString(node)
	}
	return fenceText(node)
}
