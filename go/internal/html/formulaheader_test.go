package html

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// A header cell of a formula alone is a <th> with its scope, the formula's alttext as aria-label and as
// clipped text beside the cell content (decision 095, amendment 2): Ace's empty-table-header reads neither MathML nor aria-label.
func TestFormulaHeaderCellIsNamed(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	grid := testGrid(2, 2, []any{int64(0)}, nil)
	for _, value := range rec.FetchList(grid, "cells") {
		if cell := rec.AsMap(value); rec.FetchInt(cell, "row") == 0 && rec.FetchInt(cell, "column") == 0 {
			cell["content"] = []any{rec.Map{"type": "math", "node": "m1"}}
		}
	}
	nodes := rec.Map{"m1": rec.Map{"id": "m1", "type": "math",
		"mathml": `<math xmlns="http://www.w3.org/1998/Math/MathML"><msup><mi>x</mi><mn>2</mn></msup></math>`, "children": []any{}}}
	page := (&Paper{document: rec.Map{}, nodes: nodes}).richTableSpan(tableSpan("t", grid))
	for _, want := range []string{`<th scope="col" aria-label="x^2" data-rich-cell="c0-0"`, `<span data-header-name`, `</div><span data-header-name`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Count(page, "<th ") != 2 {
		t.Errorf("th cells = %d, want 2", strings.Count(page, "<th "))
	}
}
