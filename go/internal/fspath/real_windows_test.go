//go:build windows

package fspath

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A file reached through a directory junction (scoop's apps\NAME\current) resolves to the file in
// the junction's target, where filepath.EvalSymlinks fails (decision 129).
func TestRealResolvesThroughAJunction(t *testing.T) {
	powershell, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("no powershell to make a junction")
	}
	root := t.TempDir()
	target := filepath.Join(root, "1.0")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "tool.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "current")
	script := "New-Item -ItemType Junction -Path '" + junction + "' -Target '" + target + "' | Out-Null"
	if output, err := exec.Command(powershell, "-NoProfile", "-Command", script).CombinedOutput(); err != nil {
		t.Skipf("cannot make a junction: %v %s", err, output)
	}
	got, err := Real(filepath.Join(junction, "tool.exe"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(target, "tool.exe"))
	if !strings.EqualFold(got, want) {
		t.Errorf("Real = %q, want %q", got, want)
	}
	if _, err := Real(filepath.Join(junction, "missing.exe")); err == nil {
		t.Error("a missing file resolved")
	}
}
