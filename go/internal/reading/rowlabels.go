package reading

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// printedRowLabel: the collector binds an align row's label to the row with the label's \row@
// index among the rows it reads, and amsmath's \row@ counts rows the collector never reads (an
// empty "& \notag" row, second cohort 268), or the label stands in an unnumbered row whose number
// the next row prints (\label before the first row, papers5/1373). When the recorded row does not
// print the label's number and exactly one other row of the same alignment prints "(number)",
// the label names that row, as the printed number and amsmath's anchor do.
func (e *Evidence) printedRowLabel(label rec.Map) rec.Map {
	number := evidence.LiteralNumber(rec.FetchString(label, "number"))
	formula := label["formula"]
	if !truthy(label["math_row"]) || !truthy(formula) || number == nil || *number == "" {
		return label
	}
	tag := "(" + *number + ")"
	if e.formulaPrintsTag(formula, tag) {
		return label
	}
	printing := []any{}
	for _, record := range e.records {
		if record["code"] == "external_alignment_row" && record["source"] == "native-display-alignment" &&
			rec.Equal(record["entity"], label["entity"]) && e.formulaPrintsTag(record["formula"], tag) {
			printing = append(printing, record["formula"])
		}
	}
	if len(printing) != 1 {
		return label
	}
	return rec.Merge(label, rec.Map{"formula": printing[0], "math_row": int64(1)})
}

// formulaPrintsTag: one of the formula's row label cells prints tag.
func (e *Evidence) formulaPrintsTag(formula any, tag string) bool {
	return slices.Contains(labelCellTexts(e.mathMLOf(formula)), tag)
}

// labelCellTexts are the texts of a formula's row label cells (labelCellText), in order.
func labelCellTexts(source string) []string {
	if !strings.Contains(source, `intent=":equation-label"`) {
		return nil
	}
	root, err := xmltree.Parse(source)
	if err != nil {
		return nil
	}
	texts := []string{}
	var walk func(node *xmltree.Node)
	walk = func(node *xmltree.Node) {
		for _, child := range node.Elements() {
			if child.Name == "mtd" && child.Attrs["intent"] == ":equation-label" {
				texts = append(texts, labelCellText(child))
				continue
			}
			walk(child)
		}
	}
	walk(root)
	return texts
}
