package a11ylint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/paperdoc"
)

func hintPaper(t *testing.T, source string) Paper {
	t.Helper()
	paperdoc.Root = "../../.."
	return Paper{Files: oneFile(t, "main.tex", source)}
}

// oneFile builds one source file without reading the disk.
func oneFile(t *testing.T, path, source string) []*File {
	t.Helper()
	return []*File{newFile(path, source)}
}

func hintOf(t *testing.T, items []Item, text string) *Item {
	t.Helper()
	for index := range items {
		if items[index].Kind == "hint" && strings.Contains(items[index].Message, text) {
			return &items[index]
		}
	}
	return nil
}

func TestHintSuggestionForDefinedOperator(t *testing.T) {
	paper := hintPaper(t, "\\documentclass{article}\n\\DeclareMathOperator{\\Im}{Im}\n\\begin{document}\n$\\Im f$\n\\end{document}\n")
	item := hintOf(t, Lint(paper, Options{}), `\Im`)
	if item == nil {
		t.Fatal("no hint for a defined \\Im")
	}
	if item.Suggestion != `%% relatex speech: \Im = "…"` || item.NoteLine != 2 || item.Place != PlaceAfter {
		t.Errorf("suggestion = %+v", item)
	}
}

func TestHintSuggestionQuietOnceGiven(t *testing.T) {
	paper := hintPaper(t, "%% relatex speech: \\Im = \"image\"\n\\DeclareMathOperator{\\Im}{Im}\n")
	if item := hintOf(t, Lint(paper, Options{}), `\Im`); item != nil {
		t.Errorf("a hint was still suggested: %+v", item)
	}
}

func TestHintWarnings(t *testing.T) {
	paper := hintPaper(t, "%% relatex sound: \\Im = \"image\"\n"+
		"%% relatex speech: \\Foo = \"foo\"\n"+
		"%% relatex speech: \\Im = \"image\"\n"+
		"%% relatex speech: \\Im = \"imaginary part\"\n")
	items := Lint(paper, Options{})
	if hintOf(t, items, `unknown hint section "sound"`) == nil {
		t.Error("no unknown-section item")
	}
	if hintOf(t, items, `neither defines nor uses`) == nil {
		t.Error("no undefined-macro item")
	}
	if hintOf(t, items, "two hints for speech") == nil {
		t.Error("no conflict item")
	}
}

func TestHintTokenCountsAsUse(t *testing.T) {
	paper := hintPaper(t, "\\documentclass{article}\n\\begin{document}\n$\\mathrm{Im} f$\n\\end{document}\n"+
		"%% relatex speech: Im = \"image\"\n")
	if item := hintOf(t, Lint(paper, Options{}), `neither defines nor uses`); item != nil {
		t.Errorf("an operator the paper writes as a token was called unused: %+v", item)
	}
}

// TestHintValueMustNameIntent: a spoken name becomes a MathML intent concept only when its words
// can name one; the lint says so instead of applying nothing silently.
func TestHintValueMustNameIntent(t *testing.T) {
	bad := hintPaper(t, "%% relatex speech: \\Im = \"image (of)\"\n")
	if hintOf(t, Lint(bad, Options{}), "cannot name a MathML intent") == nil {
		t.Error("no item for a value that cannot name an intent")
	}
	good := hintPaper(t, "%% relatex speech: \\Im = \"image of\"\n")
	if hintOf(t, Lint(good, Options{}), "cannot name a MathML intent") != nil {
		t.Error("a multi-word value was called invalid")
	}
}

func TestHintJSONFromDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := authordir.Ensure(filepath.Join(directory, "relatex/hints.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "relatex/hints.json"), []byte(`{"speech": {"Foo": "foo"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	paperdoc.Root = "../../.."
	paper := Paper{Files: []*File{newFile("main.tex", "\\documentclass{article}\n")}, Directory: directory}
	if hintOf(t, Lint(paper, Options{}), `"Foo"`) == nil {
		t.Error("relatex/hints.json's hint for an undefined macro was not read")
	}
}
