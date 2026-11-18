package reading

import "testing"

// A short line geometry reads as centred (a list item, a bibliography entry's last line, an
// abstract's single line: robust, separator, 2slab) stays flush left when TeX broke it with
// neither skip stretching; a centred paragraph keeps its alignment.
func TestSkipsConfirmOrVetoTheGeometry(t *testing.T) {
	for _, test := range []struct {
		name      string
		alignment string
		skips     []int
		want      string
	}{
		{"centred title, fil skips", Centred, []int{SkipCentre, SkipCentre}, Centred},
		{"list item line, plain skips", Centred, []int{SkipPlain}, ""},
		{"bibliography entry: full line, short line", Centred, []int{SkipPlain, SkipPlain}, ""},
		{"centred in a mixed paragraph", Centred, []int{SkipCentre, SkipPlain}, ""},
		{"centred, skips unknown (a \\centerline box)", Centred, []int{SkipUnknown}, Centred},
		{"centred, no lines recorded", Centred, nil, Centred},
		{"flush right, left fil only", Right, []int{SkipRight}, Right},
		{"flush right read from a centred line", Right, []int{SkipCentre}, ""},
		{"no geometry alignment stays none", "", []int{SkipCentre}, ""},
	} {
		if got := Confirm(test.alignment, test.skips); got != test.want {
			t.Errorf("%s: Confirm(%q, %v) = %q, want %q", test.name, test.alignment, test.skips, got, test.want)
		}
	}
}
