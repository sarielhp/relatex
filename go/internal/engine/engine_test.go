package engine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestTeXFailureNamesTheFileAndLine(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "paper.tex"), []byte("one\n\\maketitle\\undefinedmacro here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	errors := []string{"! Undefined control sequence.", "l.2 \\maketitle\\undefinedmacro", "! ==> Fatal error occurred"}
	want := "LuaLaTeX failed on the paper: Undefined control sequence.\n  at paper.tex:2: \\maketitle\\undefinedmacro"
	if got := describeTeXFailure(errors, source); got != want {
		t.Fatalf("got %q", got)
	}
	if got := describeTeXFailure([]string{"l.3 text"}, source); got != "" {
		t.Fatalf("no error line: got %q", got)
	}
}

func TestLuaErrorIsTheFailureSummary(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "paper.tex"), []byte("a\n\\subsection{September 15}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := "[12] [13]...94619-z8tvxj/external/working/_relatex_probe/relatex.lua:203: Sectio\n" +
		"n inside a list is unsupported\nstack traceback:\n\t[C]: in function 'assert'\n" +
		"l.2 \\subsection{September 15}\n\n!  ==> Fatal error occurred, no output PDF file produced!\n"
	errors := logErrorLines(log)
	want := "LuaLaTeX failed on the paper: relatex.lua:203: Section inside a list is unsupported\n  at paper.tex:2: \\subsection{September 15}"
	if got := describeTeXFailure(stringList(errors), source); got != want {
		t.Fatalf("got %q from %q", got, errors)
	}
}

func TestEngineFailureListsCollectorFailures(t *testing.T) {
	results := rec.Map{"control": rec.Map{"compiled": true, "errors": []any{}},
		"external": rec.Map{"compiled": false, "errors": []any{"relatex: unsupported graphic: x.ps", "relatex: unsupported graphic: x.ps"}}}
	want := "Ordinary/external builds did not both compile and converge: relatex: unsupported graphic: x.ps"
	if got := engineFailure(t.TempDir(), results); got != want {
		t.Fatalf("got %q", got)
	}
}

func TestSourceIdentityKeepsFrozenArchivesAndOtherwiseTheDigest(t *testing.T) {
	frozenID := SourceIdentity(rec.Map{"zip_sha256": "470d0f8421d1718d69e12012ce18c9fec6fae93e6b4df4bd5d56e378e065b0ca", "main": "main.tex"})
	if frozenID["id"] != "2102.05264" || frozenID["draw"] != int64(7) {
		t.Fatalf("frozen: %v", frozenID)
	}
	other := SourceIdentity(rec.Map{"zip_sha256": "abc", "main": "main.tex"})
	if other["id"] != "abc" || other["draw"] != int64(0) {
		t.Fatalf("other: %v", other)
	}
}

// BibTeX is asked for when the aux names databases the paper has; a supplied JOB.bbl
// without the databases is read instead.
func TestBibliographyRequest(t *testing.T) {
	root := t.TempDir()
	b := build{source: filepath.Join(root, "working"), output: filepath.Join(root, "output"), job: "paper", working: filepath.Join(root, "working")}
	for _, directory := range []string{b.source, b.output} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	aux := "\\relax\n\\citation{a}\n\\bibstyle{plain}\n\\bibdata{refs}\n"
	if err := os.WriteFile(filepath.Join(b.output, "paper.aux"), []byte(aux), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Probe{}
	if err := os.WriteFile(filepath.Join(b.source, "paper.bbl"), []byte("supplied"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := p.bibliographyRequest(b); got != nil {
		t.Fatalf("supplied bbl: got %v", got)
	}
	if err := os.WriteFile(filepath.Join(b.source, "refs.bib"), []byte("@misc{a}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := p.bibliographyRequest(b)
	want := []string{"bibtex", hexDigest([]byte("\\citation{a}\n\\bibstyle{plain}\n\\bibdata{refs}\n"))}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A source that ships its JOB.bbl is typeset with it, as the ordinary build does (second cohort 310).
func TestAShippedBibliographyIsUsedAsIs(t *testing.T) {
	source := t.TempDir()
	b := build{source: source, job: "paper", working: source}
	if shippedBibliography(b) {
		t.Fatal("no bibliography shipped yet")
	}
	if err := os.WriteFile(filepath.Join(source, "paper.bbl"), []byte("\\begin{thebibliography}{1}\\end{thebibliography}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !shippedBibliography(b) {
		t.Fatal("the shipped paper.bbl is not seen")
	}
}

func TestARejectedShippedBibliographyIsFoundAcrossWrappedLines(t *testing.T) {
	log := "Package biblatex Warning: File 'NocompositionsArxiv.bbl' is wrong format versio\nn - expected 3.\n"
	if !rejectedBibliography.MatchString(strings.ReplaceAll(log, "\n", "")) {
		t.Fatal("a wrapped wrong-format warning is not recognised")
	}
	if rejectedBibliography.MatchString("Package biblatex Warning: Please (re)run Biber on the file:") {
		t.Fatal("a rerun request is not a rejected bibliography")
	}
}

func TestAMainFileWithAnUppercaseExtensionIsAcceptedAndNamesItsJob(t *testing.T) {
	for _, name := range []string{"paper.tex", "vstatl.TEX", "sub/Main.Tex"} {
		if !safeMainName.MatchString(name) {
			t.Errorf("%s rejected", name)
		}
	}
	for _, name := range []string{"paper.texx", "a;b.tex", "a$b.TEX", "paper.ltx", "paper"} {
		if safeMainName.MatchString(name) {
			t.Errorf("%s accepted", name)
		}
	}
	if got := JobName("sub/vstatl.TEX"); got != "vstatl" {
		t.Errorf("JobName = %q", got)
	}
}

func TestGeneratedFontsRecordDeduplicatesAndSorts(t *testing.T) {
	probe := &Probe{}
	probe.recordGeneratedFonts([]string{"larm1200", "larm1000"})
	probe.recordGeneratedFonts([]string{"larm1000", "labx1000"})
	want := []any{"labx1000", "larm1000", "larm1200"}
	if got := probe.generatedFontsRecord(); !rec.Equal(got, want) {
		t.Fatalf("generatedFontsRecord = %v, want %v", got, want)
	}
}
