package fontbinding

import "testing"

// skipWithoutTeX skips a test that needs the TeX Live tree: New refuses a tree without pdftex.map.
func skipWithoutTeX(t *testing.T) {
	t.Helper()
	if failure(func() { New(nil) }) != "" {
		t.Skip("TeX Live's pdftex.map is not found (RELATEX_TEXLIVE names the tree)")
	}
}
