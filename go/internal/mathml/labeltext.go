package mathml

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// TagFormulaClass marks a formula a display's tag typesets, kept as MathML in the row's label cell
// (\tag{by computation for $\rec{\Sn^1}$}, books/0007; decided 2026-11-09): the label is its text
// runs (mtext) and these formulas (tex/relatex-external-equations.lua).
const TagFormulaClass = "tag-formula"

// LabelText is the printed text of an equation-label cell: its mtext children and the glyphs of its
// tag formulas, in order, stripped. A no-break space at a run's edge (kept so MathML does not trim
// it beside a formula) reads as a space.
func LabelText(cell *Node) string {
	var builder strings.Builder
	for _, child := range cell.Elements() {
		if child.Name == "mtext" || (child.Name == "mrow" && child.Attr("class") == TagFormulaClass) {
			child.writeText(&builder)
		}
	}
	return rec.Strip(strings.ReplaceAll(builder.String(), "\u00a0", " "))
}

func (n *Node) writeText(builder *strings.Builder) {
	if n.text {
		builder.WriteString(n.Text)
		return
	}
	for _, child := range n.Children {
		child.writeText(builder)
	}
}
