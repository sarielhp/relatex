package input

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const document = "\\documentclass{article}\n\\begin{document}\nx\n\\end{document}\n"

// selectIn writes the files under a paper directory and captures it. It returns the selected
// main file (or the failure message) and the recorded reason.
func selectIn(t *testing.T, files map[string]string, ignore []string) (string, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "paper")
	for name, text := range files {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	var archive *Archive
	message := failure(t, func() { archive = Capture(source, filepath.Join(root, "out"), "", nil, ignore) })
	if archive == nil {
		return message, ""
	}
	return archive.Manifest["main"].(string), archive.Manifest["selection"].(string)
}

func TestArxivReadmeJSONNamesTheMainFile(t *testing.T) {
	readme := `{"sources":[{"usage":"toplevel","filename":"neurips.tex"},{"usage":"ignore","filename":"sections/main.tex"}]}`
	main, why := selectIn(t, map[string]string{"neurips.tex": document, "sections/main.tex": document,
		"preamble.tex": "\\def\\a{}\n", "00README.json": readme}, nil)
	if main != "neurips.tex" || !strings.Contains(why, "00README") {
		t.Fatalf("got %q (%s)", main, why)
	}
}

func TestArxivReadmeXXXNamesTheMainFile(t *testing.T) {
	main, why := selectIn(t, map[string]string{"a.tex": document, "b.tex": document, "c.tex": "x\n",
		"00README.XXX": "a.tex ignore\nb.tex toplevelfile\n"}, nil)
	if main != "b.tex" || !strings.Contains(why, "00README") {
		t.Fatalf("got %q (%s)", main, why)
	}
}

// Two top-level documents, as in a paper with its supplement, stay a question for the user.
func TestTwoToplevelDocumentsStayAmbiguous(t *testing.T) {
	readme := `{"sources":[{"usage":"toplevel","filename":"main.tex"},{"usage":"toplevel","filename":"supplement.tex"}]}`
	message, _ := selectIn(t, map[string]string{"main.tex": document, "supplement.tex": document,
		"x.tex": "x\n", "00README.json": readme}, nil)
	want := "Cannot tell which file is the main one. Candidates:\n  main.tex\n  supplement.tex\n"
	if !strings.HasPrefix(message, want) {
		t.Fatalf("got %q", message)
	}
}

func TestAnIncludedDocumentIsNotTheMain(t *testing.T) {
	main, why := selectIn(t, map[string]string{"paper.tex": document + "\\input{sub/part}\n", "sub/part.tex": document,
		"other.tex": "x\n"}, nil)
	if main != "paper.tex" || !strings.Contains(why, "inputs") {
		t.Fatalf("got %q (%s)", main, why)
	}
	main, _ = selectIn(t, map[string]string{"paper.tex": document + "% \\input{part}\n", "part.tex": document}, nil)
	if !strings.HasPrefix(main, "Cannot tell") {
		t.Fatalf("a commented \\input chose %q", main)
	}
}

func TestTheRootDocumentBeatsOnesInSubdirectories(t *testing.T) {
	main, why := selectIn(t, map[string]string{"paper.tex": document, "old/paper.tex": document, "figs.tex": "x\n"}, nil)
	if main != "paper.tex" || !strings.Contains(why, "root") {
		t.Fatalf("got %q (%s)", main, why)
	}
	main, _ = selectIn(t, map[string]string{"a.tex": document, "b.tex": document, "old/c.tex": document}, nil)
	if !strings.HasPrefix(main, "Cannot tell") || strings.Contains(main, "old/c.tex") {
		t.Fatalf("two root documents: %q", main)
	}
}
