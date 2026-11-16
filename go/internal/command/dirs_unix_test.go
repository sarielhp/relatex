//go:build !windows

package command

import (
	"os"
	"regexp"
	"testing"
)

func TestUseTempRootReplacesTheSystemTemporaryDirectories(t *testing.T) {
	root, ok := TempRoot()
	if !ok || !regexp.MustCompile(`^/tmp/[^/]+/relatex$`).MatchString(root) {
		t.Fatalf("TempRoot() = %q, %v", root, ok)
	}
	for _, chosen := range []string{"", "/tmp", "/tmp/", "/var/tmp"} {
		t.Setenv("TMPDIR", chosen)
		UseTempRoot()
		if got := tmpdir(); got != root {
			t.Errorf("TMPDIR %q became %q, want %q", chosen, got, root)
		}
	}
}

func TestUseTempRootKeepsADirectoryTheCallerChose(t *testing.T) {
	t.Setenv("TMPDIR", "/home/someone/run/_tmp")
	UseTempRoot()
	if got := tmpdir(); got != "/home/someone/run/_tmp" {
		t.Errorf("TMPDIR = %q, want the caller's", got)
	}
}

func tmpdir() string { return os.Getenv("TMPDIR") }

// On Unix the search lists use ":", siblings have no suffix, and helpers see /usr/bin and /bin.
func TestUnixSearchListsAndSiblings(t *testing.T) {
	if got := SearchList(".", "/collector", ""); got != ".:/collector:" {
		t.Errorf("SearchList = %q", got)
	}
	if got := Sibling("/tl/bin/x86_64-linux/lualatex", "kpsewhich"); got != "/tl/bin/x86_64-linux/kpsewhich" {
		t.Errorf("Sibling = %q", got)
	}
	if got := ToolPath("/tl/bin/x86_64-linux/lualatex"); got != "/tl/bin/x86_64-linux:/usr/bin:/bin" {
		t.Errorf("ToolPath = %q", got)
	}
}
