package pdftag

import "testing"

// A header cell of formulas alone is a TH with the formula's alternative text as its own /Alt (decision
// 095, amendment 2), as the page's <th> carries it as aria-label.
func TestFormulaHeaderCellHasAlt(t *testing.T) {
	grid := headerFixture(t, "inferred_formulas_over_numbers")
	doc := &document{Nodes: map[string]node{}}
	for _, name := range []string{"n-fixture-0-0", "n-fixture-0-1", "n-fixture-0-2"} {
		doc.Nodes[name] = node{ID: name, Type: "math",
			MathML: `<math xmlns="http://www.w3.org/1998/Math/MathML"><msub><mi>x</mi><mn>1</mn></msub></math>`}
	}
	table := newBuilder(doc).table(grid)
	head := table.kids[0].kids
	if len(head) != 3 {
		t.Fatalf("head cells %d, want 3", len(head))
	}
	for _, cell := range head {
		if cell.role != "TH" || cell.scope != "/Column" || cell.alt != "x_1" {
			t.Errorf("head cell %s scope %q alt %q, want a TH /Column named x_1", cell.role, cell.scope, cell.alt)
		}
	}
}
