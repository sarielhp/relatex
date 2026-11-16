package figdesc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// TestQuarterTurnsReadThePlacement: the placement's rotate operations, summed, as counterclockwise
// quarter turns (decision 123 amendment 3); any other angle, or none, is no turn.
func TestQuarterTurnsReadThePlacement(t *testing.T) {
	rotate := func(cos, sin float64) rec.Map { return rec.Map{"kind": "rotate", "cos": cos, "sin": sin} }
	scale := rec.Map{"kind": "scale", "x": 2.0, "y": 2.0}
	cases := []struct {
		operations []any
		want       int
	}{
		{nil, 0},
		{[]any{scale}, 0},
		{[]any{rotate(0, -1)}, 3},
		{[]any{rotate(0, 1), scale}, 1},
		{[]any{rotate(-1, 0)}, 2},
		{[]any{rotate(0.7071068, 0.7071068), rotate(0.7071068, 0.7071068)}, 1},
		{[]any{rotate(0.8660254, 0.5)}, 0},
		{[]any{rec.Map{"kind": "rotate", "cos": "0"}}, 0},
	}
	for _, test := range cases {
		span := rec.Map{"placement": rec.Map{"operations": test.operations}}
		if got := quarterTurns(span); got != test.want {
			t.Errorf("%v: %d, want %d", test.operations, got, test.want)
		}
	}
	if quarterTurns(rec.Map{}) != 0 {
		t.Error("a span without a placement is turned")
	}
}
