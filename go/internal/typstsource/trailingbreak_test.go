package typstsource

import "testing"

// A paragraph ending in a line break must not end in `\]` (an escaped bracket) once the
// writer closes its content block (books/0003 chapter 6, the algorithm figure).
func TestTrimTrailingBreak(t *testing.T) {
	cases := map[string]string{
		`a \ b \`: `a \ b`,
		`a \ \`:   `a`,
		`a\\`:     `a\\`,
		`plain`:   `plain`,
	}
	for in, want := range cases {
		if got := trimTrailingBreak(in); got != want {
			t.Errorf("trimTrailingBreak(%q) = %q, want %q", in, got, want)
		}
	}
}
