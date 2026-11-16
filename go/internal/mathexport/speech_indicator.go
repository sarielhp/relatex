package mathexport

// The indicator function (decision 114 amendment 2026-11-10). MathCAT reads the double-struck one
// of 𝟙_A as "double struck 1 sub ..."; the literal intent makes it say "indicator sub ...".

const (
	// indicator is U+1D7D9, the double-struck digit one.
	indicator = "\U0001D7D9"
	// indicatorIntent is the literal a speech engine reads as "indicator".
	indicatorIntent = "indicator"
)

// speechIndicator gives a double-struck one the literal intent "indicator" when it is the base of
// a subscript: 𝟙_A(x) is the indicator function in every use of the notation. A bare 𝟙, or one
// with a superscript only, keeps no intent: it is the number in some other role.
func speechIndicator(node *Node) {
	if node.Name != "mn" && node.Name != "mi" || tokenString(node) != indicator || node.Has("intent") {
		return
	}
	base := node
	for base.Parent != nil && base.Parent.Name == "mrow" && len(base.Parent.Elements()) == 1 {
		base = base.Parent
	}
	parent := base.Parent
	if parent == nil || parent.Name != "msub" && parent.Name != "msubsup" || len(parent.Elements()) == 0 ||
		parent.Elements()[0] != base {
		return
	}
	setIntent(node, indicatorIntent)
}
