package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// An eqnarray set as several displays: a label names the display printing its number, not the first.
func TestEquationLabelNamesTheDisplayPrintingItsNumber(t *testing.T) {
	first := textBlock("d1", "display_math", int64(9))
	second := textBlock("d2", "display_math", int64(9), "(1)")
	third := textBlock("d3", "display_math", int64(9))
	fourth := textBlock("d4", "display_math", int64(9), "(2)")
	blocks := []rec.Map{first, second, third, fourth}
	e := &Evidence{records: []rec.Map{{"code": "equation_context", "source": "latex-equation-hooks",
		"entity": int64(9), "seq": int64(1)}}}

	label := func(number string) rec.Map {
		return rec.Map{"name": "eq", "counter": "equation", "number": number, "target_entity": int64(9), "seq": int64(5)}
	}
	if got := blockIDs(e.equationBlocks(label("2"), blocks)); len(got) != 1 || got[0] != "d4" {
		t.Fatalf("label 2 targets %v, want d4", got)
	}
	if got := blockIDs(e.equationBlocks(label("1"), blocks)); len(got) != 1 || got[0] != "d2" {
		t.Fatalf("label 1 targets %v, want d2", got)
	}
	if got := blockIDs(e.equationBlocks(label("7"), blocks)); len(got) != 1 || got[0] != "d1" {
		t.Fatalf("label 7 (no display prints it) targets %v, want the first display d1", got)
	}
}

// An eqnarray whose rows are displays printing their numbers in their formulas' row label cells:
// a \label at a row's end names the display whose formula prints its number (fourth cohort 1038).
func TestEquationLabelNamesTheDisplayWhoseFormulaPrintsItsNumber(t *testing.T) {
	display := func(id string, formula int64) rec.Map {
		return rec.Map{"id": id, "type": "display_math", "entity": int64(9),
			"content": []any{rec.Map{"type": "math", "formula": formula}}}
	}
	row := func(formula int64, tag string) rec.Map {
		return rec.Map{"kind": "math", "formula": formula,
			"mathml": `<math><mtable><mlabeledtr><mtd intent=":equation-label"><mtext>` + tag + `</mtext></mtd></mlabeledtr></mtable></math>`}
	}
	e := &Evidence{records: []rec.Map{{"kind": "diagnostic", "code": "equation_context", "source": "latex-equation-hooks",
		"entity": int64(9), "seq": int64(1)}, row(1, "(2.5)"), row(2, "(2.6)")}}
	label := rec.Map{"name": "v_dot", "counter": "equation", "number": "2.6", "target_entity": int64(9), "seq": int64(5)}
	if got := blockIDs(e.equationBlocks(label, []rec.Map{display("d1", 1), display("d2", 2)})); len(got) != 1 || got[0] != "d2" {
		t.Fatalf("label 2.6 targets %v, want d2", got)
	}
}

// iopart sets \equation as text-mode math in a line-wide box (third cohort 629): one paragraph
// holds the formula and its printed tag, and the label names it.
func TestEquationLabelNamesAParagraphOfFormulaAndTag(t *testing.T) {
	paragraph := rec.Map{"id": "p", "type": "paragraph", "entity": int64(9), "content": []any{
		rec.Map{"type": "text", "text": " "}, rec.Map{"type": "math", "formula": int64(3)}, rec.Map{"type": "text", "text": "(6)"}}}
	e := &Evidence{records: []rec.Map{{"code": "equation_context", "source": "latex-equation-hooks",
		"entity": int64(9), "seq": int64(1)}}}
	label := rec.Map{"name": "eq", "counter": "equation", "number": "6", "target_entity": int64(9), "seq": int64(5)}
	if got := blockIDs(e.equationBlocks(label, []rec.Map{paragraph})); len(got) != 1 || got[0] != "p" {
		t.Fatalf("label targets %v, want the paragraph p", got)
	}
	// A paragraph printing the tag without a formula is not the equation.
	plain := textBlock("q", "paragraph", int64(9), "Text", "(6)")
	if got := e.equationBlocks(label, []rec.Map{plain}); len(got) != 0 {
		t.Fatalf("label targets %v, want none", blockIDs(got))
	}
}

// An eqnarray set as row displays under a coloured \theequation (papers3/0552): the label of
// the last row, recorded as {\color{blue}8}, names the display printing (8).
func TestColouredEquationLabelNamesTheDisplayPrintingIt(t *testing.T) {
	blocks := []rec.Map{textBlock("d1", "display_math", int64(8), "(1)"), textBlock("d8", "display_math", int64(8), "(8)")}
	e := &Evidence{records: []rec.Map{{"code": "equation_context", "source": "latex-equation-hooks",
		"entity": int64(8), "seq": int64(1)}}}
	label := rec.Map{"name": "e8", "counter": "equation", "number": `{\color  {blue}8}`, "target_entity": int64(8), "seq": int64(5)}
	if got := blockIDs(e.equationBlocks(label, blocks)); len(got) != 1 || got[0] != "d8" {
		t.Fatalf("coloured label targets %v, want d8", got)
	}
}
