package media

import (
	"testing"

	"github.com/sarielhp/relatex/internal/mediamodel"
)

func TestSafePath(t *testing.T) {
	cases := map[string]bool{
		"figures/a.pdf":       true,
		"figures//a.pdf":      true,
		"figures/":            false,
		"/etc/passwd":         false,
		"figures/../a.pdf":    false,
		"./a.pdf":             false,
		"figures\\a.pdf":      false,
		"":                    false,
		"figures/results//x/": false,
	}
	for path, want := range cases {
		if got := mediamodel.SafeSourcePath(path); got != want {
			t.Errorf("safePath(%q) = %v, want %v", path, got, want)
		}
	}
	if mediamodel.SafeSourcePath(nil) {
		t.Error("a non-string path is unsafe")
	}
}
