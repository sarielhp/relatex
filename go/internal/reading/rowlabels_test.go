package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func alignRowEvidence(tags map[int64]string) *Evidence {
	records := []rec.Map{}
	for formula := int64(80); formula <= 83; formula++ {
		records = append(records,
			rec.Map{"kind": "math", "formula": formula, "mathml": `<math><mtable class="align"><mtr><mtd intent=":equation-label"><mtext>` +
				tags[formula] + `</mtext></mtd><mtd><mi>x</mi></mtd></mtr></mtable></math>`},
			rec.Map{"kind": "diagnostic", "code": "external_alignment_row", "source": "native-display-alignment",
				"entity": int64(6), "formula": formula})
	}
	return &Evidence{records: records}
}

// An align read row by row whose label the collector bound to the row after the one printing its
// number (second cohort 268: \row@ counted an empty "& \notag" row the collector never read): the
// label names the one row of its alignment that prints "(1b)".
func TestRowLabelNamesTheRowPrintingItsNumber(t *testing.T) {
	e := alignRowEvidence(map[int64]string{80: "(1a)", 82: "(1b)"})
	label := rec.Map{"name": "evLambda2", "number": "1b", "formula": int64(83), "math_row": int64(1), "entity": int64(6)}
	if got := e.printedRowLabel(label); !rec.Equal(got["formula"], int64(82)) {
		t.Fatalf("row label names formula %v, want 82", got["formula"])
	}
	own := rec.Merge(label, rec.Map{"name": "evphi", "number": "1a", "formula": int64(80)})
	if got := e.printedRowLabel(own); !rec.Equal(got["formula"], int64(80)) {
		t.Fatalf("a row printing its own number moved to %v", got["formula"])
	}
	twice := alignRowEvidence(map[int64]string{81: "(1b)", 82: "(1b)"})
	if got := twice.printedRowLabel(label); !rec.Equal(got["formula"], int64(83)) {
		t.Fatalf("a number printed by two rows moved the label to %v", got["formula"])
	}
}

// A label cell holding its tag's formula (\tag{$*$}, books/0007) prints the tag's text and the
// formula's glyphs: a row label finds the row that prints "(∗)".
func TestRowLabelReadsATagFormula(t *testing.T) {
	cell := `<mtd intent=":equation-label"><mtext>(</mtext><mrow class="tag-formula"><mo>∗</mo></mrow><mtext>)</mtext></mtd>`
	records := []rec.Map{{"kind": "math", "formula": int64(5), "mathml": `<math display="block"><mtable class="align"><mtr>` +
		cell + `<mtd><mi>x</mi></mtd></mtr></mtable></math>`}}
	e := &Evidence{records: records}
	if !e.formulaPrintsTag(int64(5), "(∗)") {
		t.Fatal("the row does not print (∗)")
	}
	if e.formulaPrintsTag(int64(5), "()") {
		t.Fatal("the row prints its tag without the formula")
	}
}
