package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A one-line caption is an hbox around formulas already captured: "of $d$ and $\alpha$" lost the
// space before each formula (folded_polygons Fig. 12). The box records it (math_space), and the
// formula's span gets it as a run of its own.
func TestRecordedSpaceBeforeACapturedFormula(t *testing.T) {
	e := New([]rec.Map{{"kind": "diagnostic", "code": "math_space", "owner": int64(7), "slot": int64(1), "space": " "}}, nil, nil)
	owner := rec.Map{"owner": int64(7)}
	e.activeSlot = 1
	if got := e.spaceBeforeMath(rec.Map{"space": ""}, owner); got != " " {
		t.Errorf("space before the captured formula = %q, want one space", got)
	}
	if got := e.spaceBeforeMath(rec.Map{"space": ""}, rec.Map{"owner": int64(8)}); got != "" {
		t.Errorf("another owner's formula got %q", got)
	}
	if got := e.spaceBeforeMath(rec.Map{"space": " "}, owner); got != " " {
		t.Errorf("the item's own space is kept, got %q", got)
	}
}
