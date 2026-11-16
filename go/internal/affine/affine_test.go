package affine

import "testing"

func TestThenAppliesFirstThenSecond(t *testing.T) {
	scale := Matrix{2, 0, 0, 2, 0, 0}
	shift := Matrix{1, 0, 0, 1, 10, 0}
	if got, want := scale.Then(shift), (Matrix{2, 0, 0, 2, 10, 0}); got != want {
		t.Errorf("scale then shift = %v, want %v", got, want)
	}
	if got, want := shift.Then(scale), (Matrix{2, 0, 0, 2, 20, 0}); got != want {
		t.Errorf("shift then scale = %v, want %v", got, want)
	}
}
