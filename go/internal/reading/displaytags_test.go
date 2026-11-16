package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func formulaDisplay(id string, entity, formula any, texts ...string) rec.Map {
	block := textBlock(id, "display_math", entity, texts...)
	block["content"] = append([]any{rec.Map{"type": "math", "formula": formula}}, block["content"].([]any)...)
	return block
}

// A multline set as two formulas (fourth cohort 961): the label recorded on the first formula
// names the part whose row label cell prints its number.
func TestFormulaLabelNamesTheContinuedDisplayPrintingItsNumber(t *testing.T) {
	first := formulaDisplay("d1", int64(48), int64(249))
	second := formulaDisplay("d2", int64(48), int64(250))
	e := &Evidence{
		blocks: []rec.Map{first, second},
		records: []rec.Map{
			{"kind": "math", "formula": int64(249), "mathml": `<math display="block"><mtable><mtr><mtd intent=":equation-label"><mtext></mtext></mtd></mtr></mtable></math>`},
			{"kind": "math", "formula": int64(250), "mathml": `<math display="block"><mtable><mtr><mtd intent=":equation-label"><mtext>(3.3)</mtext></mtd></mtr></mtable></math>`},
		},
	}
	label := rec.Map{"name": "eq", "counter": "equation", "number": "3.3", "formula": int64(249), "target_entity": int64(48),
		"math_environment": "equation", "seq": int64(5)}
	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "d2" {
		t.Fatalf("label targets %v, want d2", got)
	}
	// A number no part prints keeps the formula's own display.
	label["number"] = "3.4"
	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "d1" {
		t.Fatalf("label 3.4 targets %v, want d1", got)
	}
}

// A stray \\ ends an equation with an empty row that prints the label's number (fourth cohort
// 1065: the formula prints (3), the empty line (4)): the label names that line.
func TestEquationLabelNamesTheLineOfItsUnprintedNumber(t *testing.T) {
	display := textBlock("d", "display_math", int64(23), "x", "(3)")
	tag := textBlock("n", "paragraph", int64(23), "(4)")
	e := &Evidence{records: []rec.Map{{"code": "equation_context", "source": "latex-equation-hooks",
		"entity": int64(23), "seq": int64(1)}}}
	label := func(number string) rec.Map {
		return rec.Map{"name": "eq", "counter": "equation", "number": number, "target_entity": int64(23), "seq": int64(5)}
	}
	if got := blockIDs(e.equationBlocks(label("4"), []rec.Map{display, tag})); len(got) != 1 || got[0] != "n" {
		t.Fatalf("label 4 targets %v, want n", got)
	}
	if got := blockIDs(e.equationBlocks(label("3"), []rec.Map{display, tag})); len(got) != 1 || got[0] != "d" {
		t.Fatalf("label 3 targets %v, want d", got)
	}
}

// An align row label names a row of its own formula (second cohort 268): even when that row
// prints no number and another display of the equation prints the label's, the label keeps
// its formula's display, whose row the row matching then accepts or reports unresolved.
func TestRowLabelKeepsItsFormulaDisplay(t *testing.T) {
	printing := formulaDisplay("d1", int64(6), int64(82))
	own := formulaDisplay("d2", int64(6), int64(83))
	e := &Evidence{
		blocks: []rec.Map{printing, own},
		records: []rec.Map{
			{"kind": "math", "formula": int64(82), "mathml": `<math display="block"><mtable class="align"><mtr><mtd intent=":equation-label"><mtext>(1b)</mtext></mtd></mtr></mtable></math>`},
			{"kind": "math", "formula": int64(83), "mathml": `<math display="block"><mtable class="align"><mtr><mtd intent=":equation-label"><mtext></mtext></mtd></mtr></mtable></math>`},
		},
	}
	label := rec.Map{"name": "evLambda2", "counter": "equation", "number": "1b", "formula": int64(83), "target_entity": int64(6),
		"math_environment": "align", "math_row": int64(1), "seq": int64(5)}
	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "d2" {
		t.Fatalf("row label targets %v, want d2", got)
	}
}
