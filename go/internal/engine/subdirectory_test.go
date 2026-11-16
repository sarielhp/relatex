package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// A main file in a subdirectory is built there, as `cd sub && lualatex main.tex` would: TeX's
// working directory is the main file's, and the wrapper inputs it by its own name.
func TestAMainFileInASubdirectoryIsBuiltInItsDirectory(t *testing.T) {
	p := &Probe{main: "slides/deck.tex"}
	working := filepath.FromSlash("/attempt/working")
	if got := p.workingIn(working); got != filepath.Join(working, "slides") {
		t.Errorf("workingIn = %q", got)
	}
	if got := p.wrapper("control"); !strings.HasSuffix(got, "\\input{deck.tex}\n") {
		t.Errorf("wrapper = %q", got)
	}
	root := &Probe{main: "paper.tex"}
	if got := root.workingIn(working); got != working {
		t.Errorf("workingIn of a main file at the root = %q", got)
	}
	if got := searchPath("/w/slides", "/w", "/w"); got != command.SearchList("/w/slides", "/w", "") {
		t.Errorf("searchPath = %q", got)
	}
}

// The recorder names files relative to TeX's working directory: ../preamble.tex read from
// slides/ is the source's own file, not a dependency outside it.
func TestRecordedDependenciesAreResolvedFromTheWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "working")
	outside := filepath.Join(root, "outside.tex")
	for _, path := range []string{filepath.Join(source, "slides", "deck.tex"), filepath.Join(source, "preamble.tex"), outside} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	recorder := filepath.Join(root, "deck.fls")
	text := "PWD " + filepath.Join(source, "slides") + "\nINPUT ./deck.tex\nINPUT ../preamble.tex\nINPUT ../../outside.tex\n"
	if err := os.WriteFile(recorder, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	found := recordedDependencies(recorder, source, filepath.Join(source, "slides"))
	if len(found) != 1 || rec.AsMap(found[0])["path"] != outside {
		t.Errorf("dependencies = %v", found)
	}
}
