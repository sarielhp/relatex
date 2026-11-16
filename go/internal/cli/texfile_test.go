package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// `relatex paper.tex` converts the file's directory with that file as the main file; it was opened
// as a ZIP archive ("Truncated or missing ZIP end record").
func TestTexFileInputIsItsDirectoryAndMain(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "quick_dog.tex")
	if err := os.WriteFile(file, []byte("\\documentclass{article}"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := map[string]any{}
	if got := texFileInput(file, options); got != directory {
		t.Fatalf("input = %q, want %q", got, directory)
	}
	if options["main"] != "quick_dog.tex" {
		t.Fatalf("main = %v, want quick_dog.tex", options["main"])
	}
}

func TestTexFileInputKeepsDirectoriesAndArchives(t *testing.T) {
	directory := t.TempDir()
	archive := filepath.Join(directory, "paper.zip")
	if err := os.WriteFile(archive, []byte("PK"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{directory, archive} {
		options := map[string]any{}
		if got := texFileInput(input, options); got != input || options["main"] != nil {
			t.Fatalf("%s changed to %q (main %v)", input, got, options["main"])
		}
	}
}

func TestTexFileInputRefusesAnotherMain(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "a.tex")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a different --main was accepted")
		}
	}()
	texFileInput(file, map[string]any{"main": "b.tex"})
}
