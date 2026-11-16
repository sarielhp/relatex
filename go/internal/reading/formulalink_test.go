package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// TestFormulaInsideAHyperrefTextJoinsItsLink: \hyperref[dfn:a]{$c$-dense} prints a formula and
// words in one use; the words link as the author's name, and the formula, set wholly inside the
// use, takes the same link instead of staying outside it. A formula only partly inside the use
// (a node printed outside it) stays unlinked.
func TestFormulaInsideAHyperrefTextJoinsItsLink(t *testing.T) {
	references := omap.New[any, rec.Map]()
	references.Set(int64(3), rec.Map{"target": "dfn:a", "selector": `\hyperref`})
	for _, whole := range []bool{true, false} {
		math := rec.Map{"type": "math", "occurrence": "m1", "formula": int64(1)}
		words := rec.Map{"type": "reference", "id": "r2", "reference": int64(3), "text": "-dense", "target": "dfn:a"}
		inner := rec.Map{"type": "reference", "id": "r1", "reference": int64(3), "text": "c", "target": "dfn:a"}
		e := &Evidence{references: references, targets: []rec.Map{{"label": "dfn:a", "number": "2.1", "block": "b1"}},
			blocks:         []rec.Map{{"id": "b2", "content": []any{math, words}}},
			mathReferences: []rec.Map{{"occurrence": "m1", "content": []any{inner}}}}
		pending := &pendingMath{}
		pending.noteEnclosing(rec.Map{"reference": int64(3)})
		if !whole {
			pending.noteEnclosing(rec.Map{})
		}
		pending.noteEnclosing(rec.Map{"reference": int64(3)})
		if pending.enclosing != nil {
			e.enclosingLinks = map[any]enclosingLink{"m1": {reference: pending.enclosing, span: math}}
		}
		e.resolveReferences()
		link, linked := math["link"].(rec.Map)
		if words["target_block"] != "b1" || len(e.diagnostics) != 0 || inner["target_block"] != nil {
			t.Errorf("words %v inner %v diagnostics %v", words, inner, e.diagnostics)
		}
		if linked != whole || whole && (link["target"] != "dfn:a" || link["target_block"] != "b1") {
			t.Errorf("whole %v: math %v", whole, math)
		}
	}
}

// TestMathNumberFormulaLinksToItsLabel: \eqref{A} of an equation tagged \tag{$A$} prints the
// number as a formula; the printed "A" verifies against the math number and the formula, set wholly
// inside the use, takes the label's link. A number that is literal leaves the formula unlinked.
func TestMathNumberFormulaLinksToItsLabel(t *testing.T) {
	for number, linked := range map[string]bool{"{$A$}": true, "{$B$}": false} {
		references := omap.New[any, rec.Map]()
		references.Set(int64(3), rec.Map{"target": "A", "selector": `\@firstoffive`})
		math := rec.Map{"type": "math", "occurrence": "m1", "formula": int64(1)}
		inner := rec.Map{"type": "reference", "id": "r1", "reference": int64(3), "text": "A", "target": "A"}
		e := &Evidence{references: references, targets: []rec.Map{{"label": "A", "number": number, "block": "b1"}},
			blocks:         []rec.Map{{"id": "b2", "content": []any{math}}},
			mathReferences: []rec.Map{{"occurrence": "m1", "content": []any{inner}}},
			enclosingLinks: map[any]enclosingLink{"m1": {reference: int64(3), span: math}}}
		e.resolveReferences()
		_, got := math["link"].(rec.Map)
		if linked && (!got || len(e.diagnostics) != 0 || inner["target_block"] != "b1") {
			t.Errorf("%s: math %v inner %v diagnostics %v", number, math, inner, e.diagnostics)
		}
		if !linked && (got || len(e.diagnostics) != 1) {
			t.Errorf("%s: math %v diagnostics %v", number, math, e.diagnostics)
		}
	}
}
