package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// TestSplitUseResolvesAsOneUnit: \eqref{61} of {NLS$_a$} prints "NLS" as a reference span and $_a$
// as a formula holding the piece "a" (papers4/1172). The pieces spell NLSa: both link the target
// and the formula takes the link. Pieces spelling another number stay unresolved.
func TestSplitUseResolvesAsOneUnit(t *testing.T) {
	for words, linked := range map[string]bool{"NLS": true, "NL": false} {
		references := omap.New[any, rec.Map]()
		references.Set(int64(3), rec.Map{"target": "61", "selector": `\@firstoffive`})
		math := rec.Map{"type": "math", "occurrence": "m1", "formula": int64(1)}
		text := rec.Map{"type": "reference", "id": "r1", "reference": int64(3), "text": words, "target": "61"}
		inner := rec.Map{"type": "reference", "id": "r1", "reference": int64(3), "text": "a", "target": "61"}
		e := &Evidence{references: references, targets: []rec.Map{{"label": "61", "number": "{NLS$_a$}", "block": "b1"}},
			blocks:         []rec.Map{{"id": "b2", "content": []any{text, math}}},
			mathReferences: []rec.Map{{"occurrence": "m1", "content": []any{inner}}},
			enclosingLinks: map[any]enclosingLink{"m1": {reference: int64(3), span: math}}}
		e.resolveReferences()
		_, got := math["link"].(rec.Map)
		if linked && (!got || len(e.diagnostics) != 0 || text["target_block"] != "b1" || inner["target_block"] != "b1") {
			t.Errorf("%s: math %v text %v diagnostics %v", words, math, text, e.diagnostics)
		}
		if !linked && (got || len(e.diagnostics) == 0) {
			t.Errorf("%s: math %v diagnostics %v", words, math, e.diagnostics)
		}
	}
}
