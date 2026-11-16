package resourcestager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, root, relative, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResourceFilesSelection(t *testing.T) {
	root := t.TempDir()
	write(t, root, "VERSION", "1\n")
	write(t, root, "tex/a.sty", "a")
	write(t, root, "tex/.hidden/b.sty", "b")
	write(t, root, "tex/.dot", "c")
	write(t, root, "schemas/s.json", "s")
	write(t, root, "profiles/p/q.json", "p")
	write(t, root, "lib/relatex/x.rb", "ruby")
	write(t, root, "lib/relatex/axe/axe.js", "js")
	if err := os.Symlink("a.sty", filepath.Join(root, "tex", "link.sty")); err != nil {
		t.Skip("symlinks unavailable")
	}
	got, err := resourceFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"VERSION", "lib/relatex/axe/axe.js", "profiles/p/q.json", "schemas/s.json", "tex/a.sty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resourceFiles = %v, want %v", got, want)
	}
}

func TestClearKeepsReadme(t *testing.T) {
	target := t.TempDir()
	write(t, target, "README.md", "keep")
	write(t, target, "tex/old.sty", "old")
	if err := clearTarget(target); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 1 || entries[0].Name() != "README.md" {
		t.Fatalf("clear left %v", entries)
	}
}

func TestSourceFilesSkipTestsAndOtherTypes(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"b.go", "a.go", "a_test.go", "x.c", "y.h", "z.s", "notes.txt"} {
		write(t, directory, name, name)
	}
	files, err := sourceFiles(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, file := range files {
		names = append(names, filepath.Base(file))
	}
	if want := []string{"a.go", "b.go", "x.c", "y.h"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("sourceFiles = %v, want %v", names, want)
	}
}

func TestPackageDigestDependencyAndFormat(t *testing.T) {
	got, err := packageDigest("example.org/m/a|/x|example.org/m@v0.1.0", nil)
	if err != nil || got != "example.org/m/a@example.org/m@v0.1.0" {
		t.Fatalf("packageDigest = %q, %v", got, err)
	}
	text := digestJSON([]stageDigest{{"input", "aa"}, {"engine", "bb"}})
	if text != "{\n  \"input\": \"aa\",\n  \"engine\": \"bb\"\n}\n" {
		t.Fatalf("digestJSON = %q", text)
	}
}
