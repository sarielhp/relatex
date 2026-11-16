package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// With no TeX Live and nothing on PATH, doctor names every required program as missing with its
// install hint for this platform, and is not ready (relatex doctor then exits 1).
func TestDoctorNamesMissingProgramsWithInstallHints(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	t.Setenv("RELATEX_TEXLIVE", empty)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, name := range []string{"RELATEX_ENGINE", "RELATEX_MUTOOL", "RELATEX_QPDF", "RELATEX_CHROMIUM", "RELATEX_BROWSER_ENDPOINT"} {
		t.Setenv(name, "")
	}
	defer func(root string) { Root = root }(Root)
	Root = repositoryRoot(t)
	report := diagnose()
	if report.Ready {
		t.Fatal("ready without any program")
	}
	text := report.text()
	hint := "apt install qpdf"
	if runtime.GOOS == "windows" {
		hint = "scoop install qpdf"
	}
	for _, want := range []string{"MISSING  lualatex", "MISSING  qpdf             install: " + hint, "(no lualatex)", "Not ready"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
}

// A tool named by its RELATEX_* variable is reported with that source.
func TestDoctorReportsWhereAToolWasFound(t *testing.T) {
	directory := t.TempDir()
	qpdf := filepath.Join(directory, "qpdf-custom")
	if runtime.GOOS == "windows" {
		qpdf += ".exe"
	}
	if err := os.WriteFile(qpdf, []byte("#!/bin/sh\necho qpdf version 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELATEX_QPDF", qpdf)
	if path, source := resolved("qpdf")(""); path != qpdf || source != "RELATEX_QPDF" {
		t.Errorf("found %q from %q", path, source)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
