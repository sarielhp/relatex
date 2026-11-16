package artifacts

import "testing"

func TestCanonicalCollapsesDoubledSlashesOnly(t *testing.T) {
	for name, want := range map[string]string{
		"figures//a.pdf": "figures/a.pdf", "a///b//c": "a/b/c", "a/b": "a/b", "//etc": "/etc", "a/../b": "a/../b",
	} {
		if got := Canonical(name); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", name, got, want)
		}
	}
}
