package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

const eqnarrayRows = `<math xmlns="http://www.w3.org/1998/Math/MathML" display="block"><mtable class="eqnarray">` +
	`<mtr><mtd intent=":equation-label"><mtext>(6)</mtext></mtd><mtd><mi>x</mi></mtd></mtr>` +
	`<mtr><mtd intent=":equation-label"><mtext>(7)</mtext></mtd><mtd><mi>y</mi></mtd></mtr></mtable></math>`

// An eqnarray is read row by row, as an align is: a label names the row printing its number.
func TestEqnarrayLabelsNameTheRowPrintingTheirNumber(t *testing.T) {
	records := []rec.Map{{"kind": "math", "formula": int64(5), "mathml": eqnarrayRows}}
	rows := newMathRows(records, []any{rec.Map{"formula": int64(5), "id": "m1"}})
	if len(rows.rows) != 2 {
		t.Fatalf("rows %v, want two", rows.rows)
	}
	block := rec.Map{"content": []any{rec.Map{"type": "math", "formula": int64(5), "occurrence": "m1"}}}
	label := rec.Map{"math_environment": "eqnarray", "math_row": int64(2), "formula": int64(5), "number": "7"}
	row, ok := rows.target(label, block)
	if !ok || row["row"] != "m1.r2" {
		t.Fatalf("label 7 targets %v (%v), want row m1.r2", row, ok)
	}
	label["number"] = "6"
	if _, ok := rows.target(label, block); ok {
		t.Fatal("label 6 names row 2, which prints (7)")
	}
}

// Math in a tag keeps its minus signs in the row's tag, as its label cell prints them and as
// the document validation reads them (second cohort 511); a label still matches it.
func TestRowTagKeepsTheMinusSignsOfMathInTheTag(t *testing.T) {
	mathml := `<math xmlns="http://www.w3.org/1998/Math/MathML" display="block"><mtable class="align">` +
		`<mtr><mtd intent=":equation-label"><mtext>(use c1−s)</mtext></mtd><mtd><mi>x</mi></mtd></mtr></mtable></math>`
	rows := newMathRows([]rec.Map{{"kind": "math", "formula": int64(5), "mathml": mathml}}, []any{rec.Map{"formula": int64(5), "id": "m1"}})
	if len(rows.rows) != 1 || rows.rows[0]["tag"] != "(use c1−s)" {
		t.Fatalf("rows %v, want the tag (use c1−s)", rows.rows)
	}
	block := rec.Map{"content": []any{rec.Map{"type": "math", "formula": int64(5), "occurrence": "m1"}}}
	label := rec.Map{"math_environment": "align", "math_row": int64(1), "formula": int64(5), "number": "use c1-s"}
	if row, ok := rows.target(label, block); !ok || row["row"] != "m1.r1" {
		t.Fatalf("label targets %v (%v), want row m1.r1", row, ok)
	}
}
