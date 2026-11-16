//go:build windows

package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On Windows the temporary root is %TEMP%\relatex, and TMP, TEMP and TMPDIR all point at it
// (decision 129); a TMPDIR the caller chose (a run workspace) is kept, and TMP and TEMP follow it.
func TestUseTempRootSetsEveryTemporaryVariable(t *testing.T) {
	root, ok := TempRoot()
	if !ok || !strings.EqualFold(filepath.Base(root), "relatex") || !filepath.IsAbs(root) {
		t.Fatalf("TempRoot() = %q, %v", root, ok)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, "")
	}
	t.Setenv("TMPDIR", "")
	UseTempRoot()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		if got := os.Getenv(name); got != root {
			t.Errorf("%s = %q, want %q", name, got, root)
		}
	}
	chosen := t.TempDir()
	t.Setenv("TMPDIR", chosen)
	UseTempRoot()
	if os.Getenv("TMP") != chosen || os.TempDir() != chosen {
		t.Errorf("TMP %q, os.TempDir %q, want the caller's %q", os.Getenv("TMP"), os.TempDir(), chosen)
	}
}

// TeX Live for Windows has one platform directory, bin\windows, whatever relatex's own
// architecture (arm64 runs its x86-64 binaries emulated), and its programs end in .exe.
func TestTeXLiveBinIsBinWindowsWithExe(t *testing.T) {
	t.Setenv("RELATEX_TEXLIVE", `C:\texlive\2026`)
	if got := TeXLiveBin("kpsewhich"); got != `C:\texlive\2026\bin\windows\kpsewhich.exe` {
		t.Errorf("TeXLiveBin = %q", got)
	}
	tree := filepath.Join(t.TempDir(), "texlive", "2026")
	bin := filepath.Join(tree, "bin", "windows")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "lualatex.exe"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELATEX_TEXLIVE", "")
	t.Setenv("PATH", bin)
	if got := TeXLiveRoot(); !strings.EqualFold(got, tree) {
		t.Errorf("TeXLiveRoot with lualatex on PATH = %q, want %q", got, tree)
	}
}

// Executable tries PATHEXT's extensions, and a sibling has the platform's suffix.
func TestExecutableFindsExeWithoutTheExtension(t *testing.T) {
	directory := t.TempDir()
	program := filepath.Join(directory, "mutool.exe")
	if err := os.WriteFile(program, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT")
	if got := Executable("mutool"); !strings.EqualFold(got, program) {
		t.Errorf("Executable(mutool) = %q", got)
	}
	if got := Executable(filepath.Join(directory, "mutool")); !strings.EqualFold(got, program) {
		t.Errorf("Executable(path without .exe) = %q", got)
	}
	if got := Sibling(filepath.Join(directory, "lualatex.exe"), "kpsewhich"); got != filepath.Join(directory, "kpsewhich.exe") {
		t.Errorf("Sibling = %q", got)
	}
	if got := SearchList("a", "b", ""); got != "a;b;" {
		t.Errorf("SearchList = %q", got)
	}
}

// A scoop shim (tool.exe beside tool.shim) is looked up as the program it starts, so the tool's
// identity is the tool's, not the shim's; a shim whose target is gone stays the shim.
func TestExecutableLooksBehindAShim(t *testing.T) {
	shims, app := t.TempDir(), t.TempDir()
	target := filepath.Join(app, "tool.exe")
	for _, path := range []string{target, filepath.Join(shims, "tool.exe"), filepath.Join(shims, "gone.exe")} {
		if err := os.WriteFile(path, []byte("MZ"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shim := "path = \"" + target + "\"\r\nargs = -x\r\n"
	if err := os.WriteFile(filepath.Join(shims, "tool.shim"), []byte(shim), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shims, "gone.shim"), []byte(`path = "C:\nowhere\gone.exe"`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shims)
	if got := Executable("tool"); got != target {
		t.Errorf("tool found as %q", got)
	}
	if got := Executable("gone"); got != filepath.Join(shims, "gone.exe") {
		t.Errorf("gone found as %q", got)
	}
}
