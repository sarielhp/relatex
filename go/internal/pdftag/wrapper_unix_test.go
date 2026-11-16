//go:build !windows

package pdftag

import (
	"os"
	"path/filepath"
	"testing"
)

// countingWrapper writes a qpdf wrapper that logs its arguments, so a test can count the runs.
func countingWrapper(t *testing.T, qpdf string) (wrapper, calls string) {
	t.Helper()
	directory := t.TempDir()
	wrapper, calls = filepath.Join(directory, "qpdf"), filepath.Join(directory, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$QPDF_CALLS\"\nexec \"$QPDF_REAL\" \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QPDF_CALLS", calls)
	t.Setenv("QPDF_REAL", qpdf)
	return wrapper, calls
}
