package evidence

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// empheq sets its alignat in a box (an equation context of its own), then the box in an equation of the
// enclosing paragraph: the composed formula adopts a cell of another entity, but only that kind.
func TestAdoptableCellOfAnEquationContext(t *testing.T) {
	boxed := rec.Map{"kind": "math_candidate", "entity": int64(9), "seq": int64(5)}
	parent := rec.Map{"kind": "math_candidate", "entity": int64(1), "seq": int64(8)}
	m := &mathEvidence{records: []rec.Map{{"code": "equation_context", "entity": int64(9), "seq": int64(2)}}}
	if !m.adoptable(boxed, parent) {
		t.Error("a cell of an earlier equation context must be adoptable")
	}
	if m.adoptable(rec.Map{"entity": int64(7), "seq": int64(5)}, parent) {
		t.Error("a cell of a foreign entity must not be adoptable")
	}
	if m.adoptable(rec.Map{"entity": int64(9), "seq": int64(1)}, parent) {
		t.Error("a cell set before its context opened must not be adoptable")
	}
}
