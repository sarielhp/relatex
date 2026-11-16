package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func putFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func present(root, name string) bool {
	_, err := os.Lstat(filepath.Join(root, name))
	return err == nil
}

func refuses(t *testing.T, output string) (refused bool) {
	t.Helper()
	defer func() { refused = recover() != nil }()
	checkOutput(output)
	return false
}

func TestCheckOutputAcceptsNewEmptyAndRelatexOutputs(t *testing.T) {
	root := t.TempDir()
	if refuses(t, filepath.Join(root, "missing")) {
		t.Error("a new output directory was refused")
	}
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if refuses(t, empty) {
		t.Error("an empty output directory was refused")
	}
	earlier := filepath.Join(root, "earlier")
	putFile(t, earlier, "manifest.json", "{}")
	putFile(t, earlier, "index.html", "old")
	if refuses(t, earlier) {
		t.Error("an earlier conversion's output was refused")
	}
	workspace := filepath.Join(root, "workspace")
	putFile(t, workspace, "_work/x", "x")
	putFile(t, workspace, "_cache/y", "y")
	putFile(t, workspace, "run-20260101-000000-abcd/index.html", "older layout")
	if refuses(t, workspace) {
		t.Error("relatex's own directories and run-* directories were refused")
	}
}

func TestCheckOutputRefusesADirectoryHoldingOtherFiles(t *testing.T) {
	root := t.TempDir()
	putFile(t, root, "thesis.tex", "not ours")
	if !refuses(t, root) {
		t.Error("a directory with a user's file was accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	putFile(t, filepath.Dir(file), "file", "x")
	if !refuses(t, file) {
		t.Error("a file was accepted as an output directory")
	}
}

func TestPublishIntoReplacesProductsAndKeepsRelatexDirectories(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "output")
	putFile(t, output, "manifest.json", "old")
	putFile(t, output, "index.html", "old page")
	putFile(t, output, "figs/old.svg", "old figure")
	putFile(t, output, "_work/attempt/log", "work")
	putFile(t, output, "_cache/stages/s", "cache")
	putFile(t, output, "run-20260101-000000-abcd/index.html", "older layout")
	staged := filepath.Join(parent, ".relatex-publish-1")
	putFile(t, staged, "manifest.json", "new")
	putFile(t, staged, "index.html", "new page")
	putFile(t, staged, "figs/new.svg", "new figure")

	publishInto(staged, output)

	for name, want := range map[string]string{"manifest.json": "new", "index.html": "new page", "figs/new.svg": "new figure"} {
		data, err := os.ReadFile(filepath.Join(output, name))
		if err != nil || string(data) != want {
			t.Errorf("%s = %q, %v, want %q", name, data, err, want)
		}
	}
	if present(output, "figs/old.svg") {
		t.Error("a figure of the earlier conversion survived")
	}
	for _, kept := range []string{"_work/attempt/log", "_cache/stages/s", "run-20260101-000000-abcd/index.html"} {
		if !present(output, kept) {
			t.Errorf("%s was removed", kept)
		}
	}
	if present(parent, ".relatex-publish-1/index.html") {
		t.Error("the staged files were left behind")
	}
}

func TestPublishIntoANewOutputIsTheStagedDirectory(t *testing.T) {
	parent := t.TempDir()
	staged := filepath.Join(parent, ".relatex-publish-2")
	putFile(t, staged, "index.html", "page")
	output := filepath.Join(parent, "fresh")
	publishInto(staged, output)
	if !present(output, "index.html") || present(parent, ".relatex-publish-2") {
		t.Error("a new output was not made from the staged directory")
	}
}
