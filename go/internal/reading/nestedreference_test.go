package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// A tabular set inside an open formula (paper 198: \begin{tabular} inside $...$) is a nested
// formula. Its glyphs print references ("Theorem \ref{9876}" in a cell), and each such use is a
// reference span of the open formula, the one TeX typesets as a formula, not a lost use.
func TestNestedFormulaReferenceBelongsToOpenFormula(t *testing.T) {
	e := &Evidence{
		references: referenceDefinitions(),
		retention: rec.Map{"occurrences": []any{
			rec.Map{"id": "m-outer", "formula": "outer", "fragments": []any{"f-outer"}},
			rec.Map{"id": "m-inner", "formula": "inner", "fragments": []any{"f-inner"}},
		}},
		fragments: omap.New[any, rec.Map](),
		mathNodes: []nodeKey{{"inner", "f-inner", int64(1)}},
	}
	e.fragments.Set("f-outer", rec.Map{"nodes": int64(1)})
	e.fragments.Set("f-inner", rec.Map{"nodes": int64(1)})
	pending := &pendingMath{formula: "outer", remaining: 1, nested: map[any]int64{}, block: rec.Map{}}
	e.pending = pending
	e.mathIndex = 0

	item := rec.Map{"formula": "inner", "fragment": "f-inner", "position": int64(1),
		"reference": int64(7), "text": "1.5"}
	e.observeMath(item, rec.Map{}, int64(1))

	if len(pending.references) != 1 {
		t.Fatalf("open formula references %v", pending.references)
	}
	span := pending.references[0]
	if span["type"] != "reference" || span["text"] != "1.5" || span["target"] != "eq:a" {
		t.Errorf("span %v", span)
	}
	if len(e.diagnostics) != 0 {
		t.Errorf("diagnostics %v", e.diagnostics)
	}
}
