package papersource

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// The paper's own class and packages are read for front matter; a system style (not in the
// archive) and a commented-out load are not.
func TestOwnStylesAreTheShippedClassAndPackages(t *testing.T) {
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"wiley.cls": "CLASS", "spconf.sty": "SPCONF", "old.sty": "OLD"}
	manifest := rec.Map{}
	for name, text := range files {
		path := filepath.Join(directory, "input/files", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		manifest["input/files/"+name] = command.FileDigest(path)
	}
	source := &Source{directory: directory, manifest: manifest}
	text := "\\documentclass[a4]{wiley}\n\\usepackage{amsmath, spconf}\n% \\usepackage{old}\n"
	if got := source.ownStyles(text); got != "CLASS\nSPCONF\n" {
		t.Errorf("styles %q", got)
	}
}
