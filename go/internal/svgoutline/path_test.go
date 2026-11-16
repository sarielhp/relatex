package svgoutline

import (
	"slices"
	"testing"
)

func TestRubyFloat(t *testing.T) {
	for token, valid := range map[string]bool{"1.": false, "1.e5": false, ".5": true, "+.5": true, "5e+3": true, "-0": true, "1e999": true} {
		if _, ok := rubyFloat(token); ok != valid {
			t.Errorf("rubyFloat(%q) = %v, Ruby says %v", token, ok, valid)
		}
	}
}

// The expected bounds are Ruby's SVGPathBounds#bounds.
func TestPathBoundsMatchRuby(t *testing.T) {
	curve := newPathBounds("M 0 0 C 10 20 30 -20 40 0 Z").bounds([]float64{2, 0, 0, 2, 1, 1})
	if want := []float64{1.0, -10.547005383792516, 81.0, 12.547005383792515}; !slices.Equal(curve, want) {
		t.Errorf("curve bounds %v, Ruby %v", curve, want)
	}
	box := newPathBounds("m1 1 h 5 v 5 z").bounds([]float64{1, 0, 0, 1, 0, 0})
	if want := []float64{1, 1, 6, 6}; !slices.Equal(box, want) {
		t.Errorf("relative bounds %v, Ruby %v", box, want)
	}
}
