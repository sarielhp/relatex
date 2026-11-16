package synctexout

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
)

// replacing is a directory holding another tool's pair and a Replacement of it by relatex's.
func replacing(t *testing.T) (string, Replacement, *[][]string) {
	t.Helper()
	directory := t.TempDir()
	pdf := filepath.Join(directory, "paper.pdf")
	writeFile(t, pdf, "%PDF-xelatex")
	writeFile(t, Path(pdf), "xelatex synctex")
	writeFile(t, filepath.Join(directory, "paper.tex"), "source")
	partials := &[][]string{}
	built := Built{Input: directory,
		Sources: map[string]string{"paper.tex": command.FileDigest(filepath.Join(directory, "paper.tex"))}}
	seen := Digest(pdf)
	return directory, Replacement{PDF: pdf, Ours: []byte("%PDF-lualatex"), Synctex: []byte("word synctex"),
		Current:       func() error { return built.Current(pdf, seen) },
		Partial:       func(paths ...string) { *partials = append(*partials, paths) },
		Uninterrupted: func(body func() error) error { return body() }}, partials
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contents(t *testing.T, directory string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, entry := range entries {
		data, _ := os.ReadFile(filepath.Join(directory, entry.Name()))
		found[entry.Name()] = string(data)
	}
	return found
}

func TestReplacePutsRelatexsPairInPlaceOfTheOtherOne(t *testing.T) {
	directory, replacement, partials := replacing(t)
	name, err := Replace(replacement)
	if err != nil || name != "paper.pdf and paper.synctex.gz" {
		t.Fatalf("replace: %q %v", name, err)
	}
	want := map[string]string{"paper.pdf": "%PDF-lualatex", "paper.synctex.gz": "word synctex", "paper.tex": "source"}
	if got := contents(t, directory); len(got) != len(want) || got["paper.pdf"] != want["paper.pdf"] ||
		got["paper.synctex.gz"] != want["paper.synctex.gz"] {
		t.Fatalf("the directory holds %v", got)
	}
	pdf, _ := os.Stat(replacement.PDF)
	synctex, _ := os.Stat(Path(replacement.PDF))
	if synctex.ModTime().Before(pdf.ModTime()) {
		t.Fatal("the SyncTeX file is written after the PDF, as TeX writes them")
	}
	if len(*partials) != 2 || len((*partials)[0]) != 2 || len((*partials)[1]) != 0 {
		t.Fatalf("partial files %v", *partials)
	}
}

func TestReplaceInstallsNothingOverANewerPDF(t *testing.T) {
	directory, replacement, _ := replacing(t)
	writeFile(t, replacement.PDF, "%PDF-newer xelatex build")
	_, err := Replace(replacement)
	var superseded *Superseded
	if !errors.As(err, &superseded) || err.Error() !=
		"superseded: paper.pdf changed after relatex saw it; nothing installed, paper.pdf and paper.synctex.gz left as they were" {
		t.Fatalf("replace over a newer PDF: %v", err)
	}
	got := contents(t, directory)
	if len(got) != 3 || got["paper.pdf"] != "%PDF-newer xelatex build" || got["paper.synctex.gz"] != "xelatex synctex" {
		t.Fatalf("the directory holds %v", got)
	}
}

func TestReplaceInstallsNothingWhenASourceChanged(t *testing.T) {
	directory, replacement, _ := replacing(t)
	writeFile(t, filepath.Join(directory, "paper.tex"), "edited")
	_, err := Replace(replacement)
	if !strings.HasPrefix(fmtError(err), "superseded: changed after relatex read them: paper.tex;") {
		t.Fatalf("replace after an edit: %v", err)
	}
	if got := contents(t, directory); got["paper.pdf"] != "%PDF-xelatex" || got["paper.synctex.gz"] != "xelatex synctex" {
		t.Fatalf("the directory holds %v", got)
	}
}

// The SyncTeX file goes into place before the PDF: when the PDF's rename fails, the SyncTeX file
// is already relatex's.
func TestReplaceRenamesTheSynctexFileFirst(t *testing.T) {
	directory, replacement, _ := replacing(t)
	replacement.PDF = filepath.Join(directory, "occupied.pdf")
	if err := os.MkdirAll(filepath.Join(replacement.PDF, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	replacement.Current = func() error { return nil }
	if _, err := Replace(replacement); err == nil {
		t.Fatal("renaming over a directory must fail")
	}
	got := contents(t, directory)
	if got["occupied.synctex.gz"] != "word synctex" || slices.Contains(sortedKeys(got), "occupied.pdf.tmp") {
		t.Fatalf("the directory holds %v", got)
	}
}

func fmtError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func sortedKeys(set map[string]string) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
