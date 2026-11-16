package assets

import (
	"os"
	"path/filepath"
	"testing"
)

// mutool exits 1 after repairing a shading it cannot read in full (third cohort 786) but draws
// the page; a document it cannot open still fails.
func TestRecoveredSyntax(t *testing.T) {
	cases := map[string]bool{
		"warning: ICC support is not available\nsyntax error: too many sub-functions in stitching function\n" +
			"warning: encountered syntax errors; page may not be correct\n": true,
		"warning: ICC support is not available\n":                                                          false,
		"format error: cannot find version marker\nwarning: repairing PDF document\ncannot draw 'g.pdf'\n": false,
		"syntax error: bad shading\nerror: cannot render page\n":                                           false,
	}
	directory := t.TempDir()
	for text, want := range cases {
		path := filepath.Join(directory, "media.log")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := recoveredSyntax(path); got != want {
			t.Errorf("recoveredSyntax(%q) = %v, want %v", text, got, want)
		}
	}
	if recoveredSyntax(filepath.Join(directory, "missing.log")) {
		t.Error("a missing log is not a recovered drawing")
	}
}
