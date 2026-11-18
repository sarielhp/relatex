package input

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func failure(t *testing.T, run func()) string {
	t.Helper()
	var message string
	func() {
		defer func() {
			recovered := recover()
			var caught rec.Failure
			if err, ok := recovered.(error); ok && errors.As(err, &caught) {
				message = caught.Message
				return
			}
			if recovered != nil {
				panic(recovered)
			}
		}()
		run()
	}()
	return message
}

// A paper directory is zipped as rubyzip zips it, read back and its main file selected.
func TestDirectorySnapshotRoundTripsAndSelectsTheMain(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "paper")
	for name, text := range map[string]string{"paper.tex": "\\documentclass{article}\n\\begin{document}x\\end{document}\n",
		"prefix.tex": "\\begin{document}\n", "figs/a.txt": strings.Repeat("figure ", 1000)} {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(root, "stage")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := Capture(source, output, "", nil, Companions)
	if archive.Manifest["main"] != "paper.tex" || archive.Manifest["selection"] != "only top-level .tex file besides prefix.tex, prelim.tex" {
		t.Fatalf("selection: %v %v", archive.Manifest["main"], archive.Manifest["selection"])
	}
	files := rec.FetchList(archive.Manifest, "files")
	if len(files) != 3 || rec.AsMap(files[0])["path"] != "figs/a.txt" {
		t.Fatalf("inventory: %v", files)
	}
	extracted, err := os.ReadFile(filepath.Join(archive.Directory, "files", "figs/a.txt"))
	if err != nil || string(extracted) != strings.Repeat("figure ", 1000) {
		t.Fatalf("extracted: %v", err)
	}
}

func TestUnsafeNamesAreRejectedWithRubysInspect(t *testing.T) {
	for name, want := range map[string]string{"../escape": `Unsafe ZIP path: "../escape"`, "a\x00b": `Unsafe ZIP path: "a\u0000b"`,
		"a//b": `Unsafe ZIP path: "a//b"`, `a\b`: `Unsafe ZIP path: "a\\b"`, "C:/x.tex": `Unsafe ZIP path: "C:/x.tex"`,
		"c:x.tex": `Unsafe ZIP path: "c:x.tex"`} {
		got := failure(t, func() { checkedName(centralEntry{name: name, regularFileType: true, method: 8}) })
		if got != want {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}
}

func TestAmbiguousMainListsTheCandidatesInOrder(t *testing.T) {
	message := ambiguousMain([]string{"b.tex", "a.tex"}, " besides prefix.tex")
	want := "Cannot tell which file is the main one. Candidates:\n  a.tex\n  b.tex\n" +
		"Choose one with --main FILE, or list the others under main.ignore in relatex/config.jsonc"
	if message != want {
		t.Fatalf("got %q", message)
	}
	if got := ambiguousMain(nil, ""); got != "No .tex file begins a document; choose the main file with --main FILE" {
		t.Fatalf("got %q", got)
	}
}

func TestAColonInsideANameIsSafe(t *testing.T) {
	previous := storableOnly
	defer func() { storableOnly = previous }()
	storableOnly = false // NTFS refuses the colon: D9 refuses it on Windows (names_test.go)
	for _, name := range []string{"21_martingales/shared/fragment/def_median:estimator.tex", "fig:1.pdf", "a/b:c/d.tex"} {
		entry := centralEntry{name: name, regularFileType: true, method: 8}
		if got := checkedName(entry); got != name {
			t.Errorf("%q: got %q", name, got)
		}
	}
}

// Files differing only in letter case are distinct files (LuaLaTeX on Linux reads both): both
// survive the snapshot, identical or not, instead of failing as a colliding ZIP path.
func TestCaseDifferingFilesBothSurviveTheSnapshot(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "paper")
	files := map[string]string{"paper.tex": "\\documentclass{article}\n\\begin{document}x\\end{document}\n",
		"styles/ieeetran.cls": "same\n", "styles/IEEEtran.cls": "same\n", "Fig.txt": "one", "fig.txt": "two"}
	for name, text := range files {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(root, "stage")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := Capture(source, output, "", nil, Companions)
	for name, text := range files {
		got, err := os.ReadFile(filepath.Join(archive.Directory, "files", name))
		if err != nil || string(got) != text {
			t.Fatalf("%s: %q %v", name, got, err)
		}
	}
	if n := len(rec.FetchList(archive.Manifest, "files")); n != len(files) {
		t.Fatalf("inventory has %d files, want %d", n, len(files))
	}
}
