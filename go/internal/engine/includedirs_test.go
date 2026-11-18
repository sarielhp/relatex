package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// An included file in a subdirectory writes its .aux beside itself in the output directory, which
// LaTeX does not create (2611.18378: "I can't write on file `Appendix/main.aux'").
func TestAnIncludedFilesDirectoryExistsInTheOutput(t *testing.T) {
	root := t.TempDir()
	working, output := filepath.Join(root, "working"), filepath.Join(root, "output")
	for _, path := range []string{"Appendix/main.tex", "a/b/c.tex", "figs/x.pdf", "_relatex_probe/p.tex"} {
		full := filepath.Join(working, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := mirrorIncludeDirectories(working, output); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{"Appendix": true, "a/b": true, "figs": false, "_relatex_probe": false} {
		_, err := os.Stat(filepath.Join(output, filepath.FromSlash(path)))
		if (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", path, err == nil, want)
		}
	}
}
