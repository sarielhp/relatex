package mathexport

import "strings"

// labelsRight moves the equation numbers of a page's alignments to the right margin, where the
// PDF prints them. The collector writes a row's number as its first cell (reading order and the
// acceptance read it there); only the page's copy is reordered: a spacer cell as wide as the
// number's column keeps the formula centered, and the table takes the whole line so the number
// sits at its end. Multline keeps its one-column lines, aligned by their cells.
func labelsRight(node *Node) {
	if node.Name == "mtable" && rightLabelled(node) {
		moveLabels(node)
	}
	for _, child := range node.Elements() {
		labelsRight(child)
	}
}

func rightLabelled(table *Node) bool {
	if strings.TrimSuffix(table.Get("class"), "*") == "multline" {
		return false
	}
	rows := table.Elements()
	for _, row := range rows {
		cells := row.Elements()
		if row.Name != "mtr" || len(cells) < 2 || cells[0].Get("intent") != ":equation-label" {
			return false
		}
	}
	return len(rows) > 0
}

func moveLabels(table *Node) {
	table.SetAttribute("style", strings.TrimPrefix(table.Get("style")+";width:100%", ";"))
	for _, row := range table.Elements() {
		cells := row.Elements()
		label := cells[0]
		label.SetAttribute("style", label.Get("style")+";text-align:right;text-align:-webkit-right;width:50%")
		spacer := &Node{Name: "mtd", Parent: row}
		spacer.SetAttribute("style", "width:50%")
		spacer.Children = []*Node{{Name: "mtext", Parent: spacer}}
		row.setElements(append(append([]*Node{spacer}, cells[1:]...), label))
	}
}
