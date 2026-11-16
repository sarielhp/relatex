package input

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// linkedSource is a paper directory with one extra entry made by add, captured into a stage.
func linkedSource(t *testing.T, add func(source, outside string) error) *Archive {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "paper")
	outside := filepath.Join(root, "outside")
	for _, directory := range []string{filepath.Join(source, "sub"), outside, filepath.Join(root, "stage")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{filepath.Join(source, "main.tex"): "\\documentclass{article}\n\\begin{document}x\\end{document}\n",
		filepath.Join(source, "sub", "body.tex"): "body\n", filepath.Join(outside, "secret.tex"): "secret\n"}
	for path, text := range files {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := add(source, outside); err != nil {
		t.Fatal(err)
	}
	return Capture(source, filepath.Join(root, "stage"), "main.tex", nil, Companions)
}

func paths(archive *Archive) map[string]bool {
	found := map[string]bool{}
	for _, file := range rec.FetchList(archive.Manifest, "files") {
		found[rec.AsString(rec.AsMap(file)["path"])] = true
	}
	return found
}

func onlySkip(t *testing.T, archive *Archive, path, code string) {
	t.Helper()
	skipped, _ := archive.Manifest["skipped"].([]any)
	if len(skipped) != 1 {
		t.Fatalf("skipped: %v", skipped)
	}
	entry := rec.AsMap(skipped[0])
	if entry["path"] != path || entry["code"] != code {
		t.Fatalf("skipped: %v", entry)
	}
	if paths(archive)[path] {
		t.Fatalf("%s kept", path)
	}
}

// An editor's lock file (Emacs .#main.tex) is a dangling link: skipped with a warning.
func TestDanglingSymlinkIsSkipped(t *testing.T) {
	archive := linkedSource(t, func(source, _ string) error {
		return os.Symlink("user@host.1234", filepath.Join(source, ".#main.tex"))
	})
	onlySkip(t, archive, ".#main.tex", "input.symlink_skipped")
}

// A link to a file inside the source is kept under its own name with the target's bytes.
func TestSymlinkInsideTheSourceIsKept(t *testing.T) {
	archive := linkedSource(t, func(source, _ string) error {
		return os.Symlink("sub/body.tex", filepath.Join(source, "alias.tex"))
	})
	if _, ok := archive.Manifest["skipped"]; ok || !paths(archive)["alias.tex"] {
		t.Fatalf("manifest: %v", archive.Manifest)
	}
	data, err := os.ReadFile(filepath.Join(archive.Directory, "files", "alias.tex"))
	if err != nil || string(data) != "body\n" {
		t.Fatalf("alias: %q %v", data, err)
	}
}

func TestSymlinkLeavingTheSourceIsSkipped(t *testing.T) {
	archive := linkedSource(t, func(source, outside string) error {
		return os.Symlink(filepath.Join(outside, "secret.tex"), filepath.Join(source, "secret.tex"))
	})
	onlySkip(t, archive, "secret.tex", "input.symlink_skipped")
}

func TestSymlinkToADirectoryIsSkipped(t *testing.T) {
	archive := linkedSource(t, func(source, _ string) error {
		return os.Symlink(".", filepath.Join(source, "loop"))
	})
	onlySkip(t, archive, "loop", "input.symlink_skipped")
}
