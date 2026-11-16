//go:build windows

package pdftag

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// wrapperVariable makes a copy of the test binary act as the counting qpdf wrapper: Windows
// cannot start a #! script, so the wrapper is this program.
const wrapperVariable = "RELATEX_TEST_QPDF_WRAPPER"

func TestMain(m *testing.M) {
	if os.Getenv(wrapperVariable) == "1" {
		os.Exit(wrap())
	}
	os.Exit(m.Run())
}

// wrap logs its arguments to QPDF_CALLS and runs QPDF_REAL with them.
func wrap() int {
	calls, err := os.OpenFile(os.Getenv("QPDF_CALLS"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 2
	}
	_, err = io.WriteString(calls, strings.Join(os.Args[1:], " ")+"\n")
	if errors.Join(err, calls.Close()) != nil {
		return 2
	}
	program := exec.Command(os.Getenv("QPDF_REAL"), os.Args[1:]...)
	program.Stdin, program.Stdout, program.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = program.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return exit.ExitCode()
	case err != nil:
		return 2
	}
	return 0
}

// countingWrapper copies the test binary as a qpdf wrapper that logs its arguments, so a test can
// count the runs.
func countingWrapper(t *testing.T, qpdf string) (wrapper, calls string) {
	t.Helper()
	directory := t.TempDir()
	wrapper, calls = filepath.Join(directory, "qpdf.exe"), filepath.Join(directory, "calls.log")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, data, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(wrapperVariable, "1")
	t.Setenv("QPDF_CALLS", calls)
	t.Setenv("QPDF_REAL", qpdf)
	return wrapper, calls
}
