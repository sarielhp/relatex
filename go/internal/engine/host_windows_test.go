//go:build windows

package engine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The author's TEXINPUTS on Windows: entries split at ";" (a drive letter has a ":"), "~\" is the
// home directory, and the collector goes before kpathsea's default path.
func TestWindowsHostSearchPathSplitsAtSemicolons(t *testing.T) {
	t.Setenv("TZ", "")
	home, _ := os.UserHomeDir()
	host := NewHost(`~\styles//;.;..\;!!C:\opt\tex;;`, `C:\papers\26\dog`)
	want := []string{filepath.Join(home, "styles") + "//", ".", `C:\papers\26`, `!!C:\opt\tex`, "", ""}
	if !slices.Equal(host.SearchPath, want) {
		t.Fatalf("search path %q, want %q", host.SearchPath, want)
	}
	if got := host.searchPath(`C:\c`); got != filepath.Join(home, "styles")+`//;.;C:\papers\26;!!C:\opt\tex;C:\c;;` {
		t.Fatalf("TEXINPUTS %q", got)
	}
	if host.Given != `~\styles//;.;..\;!!C:\opt\tex;;` {
		t.Fatalf("given %q", host.Given)
	}
	if got := host.mounts(); len(got) != 0 {
		t.Fatalf("without a sandbox nothing is mounted, not even the local time: %q", got)
	}
	given := SearchHost(filepath.Join(home, "styles")+";.", `C:\d`).Given
	if given != `~\styles;.` {
		t.Fatalf("the home directory is written ~: %q", given)
	}
}
