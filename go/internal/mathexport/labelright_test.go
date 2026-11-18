package mathexport

import (
	"strings"
	"testing"
)

const labelledTable = `<math><mtable class="%s"><mtr><mtd intent=":equation-label"><mtext>(1)</mtext></mtd>` +
	`<mtd><mi>x</mi></mtd></mtr></mtable></math>`

func TestLabelsRightMovesNumberLast(t *testing.T) {
	for _, class := range []string{"eqnarray", "align", "gather*"} {
		root := ParseRaw(strings.Replace(labelledTable, "%s", class, 1))
		labelsRight(root)
		row := root.Elements()[0].Elements()[0]
		cells := row.Elements()
		if len(cells) != 3 || cells[2].Get("intent") != ":equation-label" || cells[1].Name != "mtd" {
			t.Fatalf("%s: the number is not the last cell: %s", class, root.String())
		}
		if !strings.Contains(cells[2].Get("style"), "text-align:-webkit-right") ||
			!strings.Contains(root.Elements()[0].Get("style"), "width:100%") {
			t.Errorf("%s: number not right-aligned or table not full width: %s", class, root.String())
		}
	}
}

func TestLabelsRightLeavesMultlineAndLoneLabels(t *testing.T) {
	for _, source := range []string{
		strings.Replace(labelledTable, "%s", "multline", 1),
		`<math><mtable><mtr><mtd intent=":equation-label"><mtext>(1)</mtext></mtd></mtr></mtable></math>`,
	} {
		root := ParseRaw(source)
		labelsRight(root)
		if root.String() != ParseRaw(source).String() {
			t.Errorf("changed: %s", root.String())
		}
	}
}
