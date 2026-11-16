package mathexport

import "slices"

// Two places where the collector's MathML follows TeX's math list closer than the formula's reading
// (decision 114, amendment 2026-11-15, found by the 100-formula MathCAT sample): a number TeX sets
// as digits, a period and digits, and a script TeX sets on an empty box after its base ({}^{-1},
// A{}'). Both are regrouped for speech; no glyph, space or stretch changes.

// scriptable are the elements a script written on an empty box belongs to.
var scriptable = []string{"mi", "mn", "mrow", "msub", "msup", "msubsup", "mfrac", "msqrt", "mroot"}

// emptyBaseScript: an msub, msup or msubsup whose base is an empty row, TeX's {} before a script.
func emptyBaseScript(node *Node) bool {
	if !slices.Contains([]string{"msub", "msup", "msubsup"}, node.Name) || len(node.Elements()) < 2 {
		return false
	}
	base := node.Elements()[0]
	return base.Name == "mrow" && len(base.Elements()) == 0 && len(textsOf(base)) == 0
}

// attachScripts makes a script on an empty box the script of the element before it: MathCAT reads
// "Σ{}^{-1}" as "sigma, pre super negative 1" (the prescript of what follows) but "sigma to the
// negative 1 power" when the script has its base. A script with nothing before it (a prescript:
// {}^{14}C) or after an operator, a space or text stays as it is.
func attachScripts(row *Node) {
	children := row.Elements()
	result := make([]*Node, 0, len(children))
	for _, child := range children {
		if len(result) > 0 && emptyBaseScript(child) && attachable(result[len(result)-1]) {
			base := result[len(result)-1]
			child.setElements(append([]*Node{base}, child.Elements()[1:]...))
			result[len(result)-1] = child
			continue
		}
		result = append(result, child)
	}
	row.setElements(result)
}

// attachable: an element that can be a script's base, and not an accent box (the accent groups with
// the base after it).
func attachable(node *Node) bool {
	if !slices.Contains(scriptable, node.Name) {
		return false
	}
	_, accent := accentBox(node)
	return !accent
}

// digits: an mn of decimal digits only.
func digits(node *Node) bool {
	text := tokenString(node)
	if node.Name != "mn" || text == "" {
		return false
	}
	for _, character := range text {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// decimalPoint: the period TeX sets between the digits of a number, with no space around it.
func decimalPoint(node *Node) bool {
	if node.Name != "mo" || tokenString(node) != "." {
		return false
	}
	for _, attribute := range node.Attrs {
		if attribute.Prefix != "" || attribute.Name != "lspace" && attribute.Name != "rspace" || attribute.Value() != "0" {
			return false
		}
	}
	return true
}

// mergeDecimals writes digits, a decimal point and digits as one mn: MathCAT reads 0.2 inside a
// list ([0.2, 0.4]) as "0 dot 2". The number keeps the first mn's attributes and shows the same
// characters in the same font, and the point had no space around it.
func mergeDecimals(row *Node) {
	children := row.Elements()
	result := make([]*Node, 0, len(children))
	for index := 0; index < len(children); index++ {
		child := children[index]
		if digits(child) && index+2 < len(children) && decimalPoint(children[index+1]) && digits(children[index+2]) {
			child.SetText(tokenString(child) + "." + tokenString(children[index+2]))
			index += 2
		}
		result = append(result, child)
	}
	row.setElements(result)
}
