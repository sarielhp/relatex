package cli

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/a11ylint"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func loadNeeds(t *testing.T, files map[string]string) ([]paperNeed, bibUse) {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, files)
	loaded, err := a11ylint.Load(root, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	return scanNeeds(loaded)
}

func needFiles(needs []paperNeed) []string {
	var names []string
	for _, need := range needs {
		names = append(names, need.File)
	}
	return names
}

// Comments are skipped; option brackets and comma lists give one file each; \RequirePackage, the
// class, the bibliography style, beamer themes and TikZ libraries count; a nested \input is followed
// and a missing one is not an error.
func TestScanNeedsParsesCommandsOptionsListsAndNestedInputs(t *testing.T) {
	needs, bib := loadNeeds(t, map[string]string{
		"main.tex": "\\documentclass[11pt]{revtex4-2}\n% \\usepackage{commented}\n\\usepackage[utf8]{inputenc, foo}\n" +
			"\\RequirePackage{bar}\n\\usetheme{Madrid}\n\\usetikzlibrary{calc,arrows.meta}\n\\input{sec/a}\n\\input{nothere}\n" +
			"\\bibliographystyle{plain}\n\\bibliography{refs}\n50\\% \\usepackage{afterpercent}\n",
		"sec/a.tex": "\\usepackage{deep}\n",
	})
	want := []string{"revtex4-2.cls", "inputenc.sty", "foo.sty", "bar.sty", "beamerthemeMadrid.sty",
		"tikzlibrarycalc.code.tex", "tikzlibraryarrows.meta.code.tex", "plain.bst", "afterpercent.sty", "deep.sty"}
	if got := needFiles(needs); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("needs %v, want %v", got, want)
	}
	if !bib.Bibtex || bib.Biblatex || bib.program() != "bibtex" {
		t.Errorf("bibliography %+v", bib)
	}
	if needs[0].Where != "main.tex:1" || needs[len(needs)-1].Where != "sec/a.tex:1" {
		t.Errorf("places %q, %q", needs[0].Where, needs[len(needs)-1].Where)
	}
}

func TestScanNeedsFindsTheBiblatexBackend(t *testing.T) {
	for source, program := range map[string]string{
		"\\usepackage{biblatex}":                        "biber",
		"\\usepackage[backend=biber,style=x]{biblatex}": "biber",
		"\\usepackage[backend=bibtex]{biblatex}":        "bibtex",
		"\\usepackage{cite}":                            "bibtex",
	} {
		_, bib := loadNeeds(t, map[string]string{"main.tex": source + "\n\\bibliography{x}\n"})
		if got := bib.program(); got != program || (bib.Biblatex && bib.Bibtex) {
			t.Errorf("%s: program %q (%+v), want %q", source, got, bib, program)
		}
	}
}

// fakeTeXLive is a directory on PATH with a lualatex, a kpsewhich that finds only the given files
// and the named extra programs; it returns the directory.
func fakeTeXLive(t *testing.T, found []string, programs ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes")
	}
	directory := t.TempDir()
	script := "#!/bin/sh\nfor name in \"$@\"; do case \"$name\" in " + strings.Join(quoted(found), "|") +
		") echo /texlive/$name;; esac; done\n"
	if len(found) == 0 {
		script = "#!/bin/sh\nexit 1\n"
	}
	files := map[string]string{"kpsewhich": script, "lualatex": "#!/bin/sh\necho fake\n"}
	for _, name := range programs {
		files[name] = "#!/bin/sh\necho fake\n"
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(text), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	t.Setenv("RELATEX_TEXLIVE", t.TempDir())
	t.Setenv("RELATEX_ENGINE", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return directory
}

func quoted(names []string) []string {
	var out []string
	for _, name := range names {
		out = append(out, "'"+name+"'")
	}
	return out
}

func TestLookupTeXFilesReadsKpsewhichOutput(t *testing.T) {
	directory := fakeTeXLive(t, []string{"caption.sty", "article.cls"})
	found, err := lookupTeXFiles(filepath.Join(directory, "kpsewhich"), []string{"caption.sty", "nothere.sty", "article.cls"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found["caption.sty"] != "/texlive/caption.sty" || found["nothere.sty"] != "" {
		t.Errorf("found %v", found)
	}
}

// A file the paper has itself (beside the main file or an included one) is not looked up; a missing
// one is listed with the generic hint; biber is needed by biblatex and found beside the engine.
func TestDiagnosePaperListsWhatIsMissing(t *testing.T) {
	fakeTeXLive(t, []string{"article.cls", "caption.sty"}, "biber")
	paper := t.TempDir()
	writeTree(t, paper, map[string]string{
		"main.tex":  "\\documentclass{article}\n\\usepackage{caption,mine,ghost}\n\\usepackage[backend=biber]{biblatex}\n\\input{sec/a}\n",
		"mine.sty":  "\\ProvidesPackage{mine}\n",
		"sec/a.tex": "\\usepackage{other}\n",
	})
	report := diagnosePaper(paper, "", false)
	var names []string
	for _, missing := range report.Missing {
		names = append(names, missing.File)
	}
	want := "ghost.sty biblatex.sty other.sty"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("missing %q, want %q", got, want)
	}
	if report.InPaper != 1 || report.Ready || report.Program == nil || !report.Program.Found || report.Program.Name != "biber" {
		t.Errorf("report %+v program %+v", report, report.Program)
	}
	text := report.text()
	for _, want := range []string{"MISSING  ghost.sty", "tlmgr install ghost (the TeX Live package may be named differently",
		"biblatex, backend biber: ok", "Not complete by design", "Not ready"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
}

func TestDiagnosePaperIsReadyWhenEverythingIsThere(t *testing.T) {
	fakeTeXLive(t, []string{"article.cls", "plain.bst"}, "bibtex")
	paper := t.TempDir()
	writeTree(t, paper, map[string]string{"main.tex": "\\documentclass{article}\n\\bibliographystyle{plain}\n\\bibliography{r}\n"})
	report := diagnosePaper(filepath.Join(paper, "main.tex"), "", false)
	if !report.Ready || len(report.Missing) != 0 || report.Program == nil || report.Program.Name != "bibtex" {
		t.Errorf("report %+v", report)
	}
	if !strings.Contains(report.text(), "Ready.") {
		t.Errorf("text %s", report.text())
	}
}

// The bibliography program missing makes the paper not ready even when every file is found.
func TestDiagnosePaperReportsAMissingBibliographyProgram(t *testing.T) {
	fakeTeXLive(t, []string{"article.cls", "biblatex.sty"})
	paper := t.TempDir()
	writeTree(t, paper, map[string]string{"main.tex": "\\documentclass{article}\n\\usepackage{biblatex}\n"})
	report := diagnosePaper(paper, "", false)
	if report.Ready || report.Program == nil || report.Program.Found {
		t.Errorf("report %+v", report)
	}
	if !strings.Contains(report.text(), "MISSING biber") {
		t.Errorf("text %s", report.text())
	}
}

// Without kpsewhich the lookup cannot be done, which is reported rather than every file called missing.
func TestDiagnosePaperWithoutKpsewhich(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("RELATEX_TEXLIVE", t.TempDir())
	t.Setenv("RELATEX_ENGINE", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	paper := t.TempDir()
	writeTree(t, paper, map[string]string{"main.tex": "\\documentclass{article}\n"})
	report := diagnosePaper(paper, "", false)
	if report.Ready || !strings.Contains(report.LookupError, "kpsewhich was not found") {
		t.Errorf("report %+v", report)
	}
}

func TestDiagnosePaperReadsAZip(t *testing.T) {
	fakeTeXLive(t, []string{"article.cls"})
	archive := filepath.Join(t.TempDir(), "paper.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, text := range map[string]string{
		"paper/main.tex": "\\documentclass{article}\n\\usepackage{ghost}\n", "paper/fig.png": "x",
	} {
		entry, _ := writer.Create(name)
		entry.Write([]byte(text))
	}
	writer.Close()
	file.Close()
	report := diagnosePaper(archive, "", false)
	if report.Main != "main.tex" || len(report.Missing) != 1 || report.Missing[0].File != "ghost.sty" {
		t.Errorf("report %+v", report)
	}
}

// A ZIP entry that leaves the extraction directory is skipped.
func TestExtractSourcesRefusesNamesOutsideTheDirectory(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "p.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{"main.tex", "../escape.tex", "/abs.tex"} {
		entry, _ := writer.Create(name)
		entry.Write([]byte("x"))
	}
	writer.Close()
	file.Close()
	target := filepath.Join(root, "out")
	if err := extractSources(archive, target); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"escape.tex", "abs.tex"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("%s was written outside", name)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "main.tex")); err != nil {
		t.Error(err)
	}
}

func TestParseTlmgrSearchNamesThePackageOfTheFile(t *testing.T) {
	output := "tlmgr: package repository https://example.org\ncollection-latexextra:\n\ttexmf-dist/tex/latex/other/x.sty\n" +
		"caption:\n\ttexmf-dist/tex/latex/caption/caption.sty\n\ttexmf-dist/doc/latex/caption/caption.pdf\n"
	if got := parseTlmgrSearch(output, "caption.sty"); got != "caption" {
		t.Errorf("package %q", got)
	}
	if got := parseTlmgrSearch(output, "zzz.sty"); got != "" {
		t.Errorf("package %q for an unknown file", got)
	}
	if got := installHint("caption.sty", "caption", true); got != "tlmgr install caption" {
		t.Errorf("hint %q", got)
	}
}

// The program list holds the programs the product starts besides the TeX Live core: rsvg-convert,
// tesseract, Metafont and mktextfm everywhere, fontconfig's fc-list where it exists; the missing
// ones say what is lost.
func TestDoctorListsTheOtherProgramsTheProductStarts(t *testing.T) {
	names := map[string]bool{}
	for _, spec := range append(doctorTools(), extraTools()...) {
		names[spec.name] = true
		if spec.linux == "" || spec.windows == "" && spec.name != "fc-list" {
			t.Errorf("%s has no install hint", spec.name)
		}
	}
	for _, want := range []string{"rsvg-convert", "tesseract", "mf", "mktextfm", "biber", "bibtex"} {
		if !names[want] {
			t.Errorf("no %s in the program list", want)
		}
	}
	if runtime.GOOS != "windows" && !names["fc-list"] {
		t.Error("no fc-list in the program list")
	}
	for name := range names {
		if doctorEffects[name] == "" && name != "lualatex" && name != "kpsewhich" && name != "luaotfload-tool" &&
			name != "vftovp" && name != "mutool" && name != "qpdf" && !strings.HasPrefix(name, "pdf") {
			t.Errorf("%s has no effect line", name)
		}
	}
	check := toolSpec{"rsvg-convert", "", onPath("rsvg-convert-missing"), false, "apt install librsvg2-bin", "x"}.check("")
	if text := check.text(); !strings.Contains(text, "optional") || !strings.Contains(text, "install: ") {
		t.Errorf("text %s", text)
	}
	check.Without = doctorEffects["rsvg-convert"]
	if !strings.Contains(check.text(), "without it: the check that a redrawn SVG") {
		t.Errorf("text %s", check.text())
	}
}
