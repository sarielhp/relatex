package a11ylint

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// A book laid out as the author's (books store 0003, decision 115): chapters in their own
// directories, input by a command of the preamble (\InputChap) that a companion file defines and
// that another command (\InputXChap) calls; a preamble file found through \input@path; figures
// named through \File{…} (\Dir/…), included by \IncludeGraphics[opts]{…} (passes its options on),
// by \PicPar{file}{paragraph} (does not), by a qed sign defined once in the preamble and by an
// xfig overlay the import package reads from the figures directory, and pages of one file through a
// command the chapter defines (the calls are the places, not the definition).
var bookFiles = map[string]string{
	"book.tex": `\documentclass{book}
\usepackage{graphicx}
\input{prefix}
\begin{document}
\InputXChap{00_intro}{intro.tex}
\InputChap{01_disk}{disk.tex}
\end{document}
`,
	"prefix.tex": "\\makeatletter\\def\\input@path{{shared/}}\\makeatother\n\\input{prefix_latex}\n",
	"shared/prefix_latex.tex": `\newcommand{\InputChap}[2]{\def\Dir{#1}\typeout{#1/#2}\input{#1/#2}}
\providecommand{\InputXChap}[2]{\InputChap{#1}{#2}}
\newcommand{\File}[1]{\Dir/#1}
\providecommand{\IncludeGraphics}[2][]{\includegraphics[#1]{#2}}
\newcommand{\PicPar}[2]{\includegraphics[width=2cm]{\File{#1}}#2}
\newcommand{\Qed}{\includegraphics[height=1em]{shared/duck.pdf}}
`,
	"00_intro/intro.tex": "\\chapter{Intro}\n\\begin{tabular}{cc}\na & b \\\\\n1 & 2 \\\\\n\\end{tabular}\n",
	"01_disk/disk.tex": `\chapter{Disk}
\begin{figure}
\centering
\IncludeGraphics[width=3cm]{\File{figs/cover}}
\caption{Cover.}
\end{figure}
\PicPar{figs/grid}{A paragraph
beside the picture, read whole before it is placed.}

\includegraphics[width=2cm]{\File{figs/plain}}
\IncGraphPage{figs}{pages}{2}\Qed
\Qed
\subimport*{figs/}{over.pdf_t}
\newcommand{\Pg}[1]{%
   \IncludeGraphics[page=#1]{\File{figs/multi}}}
\Pg{1}\Pg{2}
`,
	"01_disk/figs/over.pdf_t": "\\includegraphics{over.pdf}\n",
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	for name, text := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func paths(files []*File) []string {
	result := []string{}
	for _, file := range files {
		result = append(result, file.Path)
	}
	return result
}

func TestLoadFollowsTheAuthorsInputCommandsAndInputPath(t *testing.T) {
	directory := writeTree(t, bookFiles)
	files, err := Load(directory, "book.tex")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"book.tex", "prefix.tex", "shared/prefix_latex.tex", "00_intro/intro.tex", "01_disk/disk.tex", "01_disk/figs/over.pdf_t"}
	if got := paths(files); !slices.Equal(got, want) {
		t.Fatalf("files %v, want %v", got, want)
	}
	if main, err := FindMain(directory, ""); err != nil || main != "book.tex" {
		t.Fatalf("main %q (%v), want book.tex beside the companion prefix.tex", main, err)
	}
}

// bookPaper is the book with the places a conversion records (the line TeX was reading when each
// \includegraphics ran): the \PicPar's at the end of its paragraph argument, the qed signs at their calls.
func bookPaper(t *testing.T, directory string) Paper {
	t.Helper()
	files, err := Load(directory, "book.tex")
	if err != nil {
		t.Fatal(err)
	}
	place := func(line int) []figdesc.Place { return []figdesc.Place{{File: "01_disk/disk.tex", Line: line}} }
	return Paper{Files: files, Sidecar: &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}, Drawings: []Drawing{},
		Included: []IncludedFigure{
			{Key: "cover", Source: "01_disk/figs/cover.pdf", Places: place(4)},
			{Key: "grid", Source: "01_disk/figs/grid.pdf", Places: place(8)},
			{Key: "plain", Source: "01_disk/figs/plain.pdf", Places: place(10)},
			{Key: "pages", Source: "01_disk/figs/pages.pdf", Places: place(11)},
			{Key: "duck", Source: "shared/duck.pdf", Places: append(place(11), place(12)...)},
			{Key: "multi", Source: "01_disk/figs/multi.pdf", Places: place(16)},
			{Key: "multi-2", Source: "01_disk/figs/multi.pdf", Places: place(16)},
			{Key: "done", Source: "01_disk/figs/done.pdf", Places: []figdesc.Place{{File: "01_disk/disk.tex", Line: 4, Described: true}}},
		}}
}

func TestLintFindsEveryFigureTheConversionIncludedAtItsCall(t *testing.T) {
	directory := writeTree(t, bookFiles)
	got := []string{}
	for _, item := range Lint(bookPaper(t, directory), Options{}) {
		if item.Kind == "figure" {
			got = append(got, item.File+":"+strconv.Itoa(item.Line)+" "+item.Key)
		}
	}
	// The duck once, at its \includegraphics in the preamble; plain at its own literal
	// \includegraphics; grid moved back from the paragraph's end to the call that names it.
	want := []string{"shared/prefix_latex.tex:6 duck", "01_disk/disk.tex:10 plain", "01_disk/figs/over.pdf_t:1 over", "01_disk/disk.tex:4 cover",
		"01_disk/disk.tex:7 grid", "01_disk/disk.tex:11 pages",
		"01_disk/disk.tex:16 multi", "01_disk/disk.tex:16 multi-2"}
	if !slices.Equal(got, want) {
		t.Fatalf("figure items %v, want %v", got, want)
	}
}

func TestAnnotatePutsThePlaceholderIntoACommandThatPassesItsOptionsOn(t *testing.T) {
	directory := writeTree(t, bookFiles)
	paper := bookPaper(t, directory)
	items := Lint(paper, Options{})
	destination := filepath.Join(t.TempDir(), "copy")
	if _, err := Annotate(paper, items, directory, destination, nil); err != nil {
		t.Fatal(err)
	}
	text := read(t, filepath.Join(destination, "01_disk", "disk.tex"))
	for _, want := range []string{
		`\IncludeGraphics[width=3cm, alt={TODO: describe 01\_disk/figs/cover.pdf}]{\File{figs/cover}}`,
		`\includegraphics[width=2cm, alt={TODO: describe \textbackslash{}File\{figs/plain\}}]{\File{figs/plain}}`,
		"%% relatex-a11y: figure", `\PicPar{figs/grid}{A paragraph`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the copy lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `\PicPar[`) || strings.Contains(text, `\IncGraphPage[`) {
		t.Errorf("a placeholder went into a command that does not pass options on:\n%s", text)
	}
}

func TestADrawingIsListedWhereItsPictureBegins(t *testing.T) {
	directory := writeTree(t, bookFiles)
	paper := bookPaper(t, directory)
	paper.Drawings = []Drawing{{Key: "rendered-1", Places: []figdesc.Place{{File: "00_intro/intro.tex", Line: 2}}}, {Key: "rendered-2"}}
	got := []string{}
	for _, item := range Lint(paper, Options{}) {
		if item.Kind == "drawing" {
			got = append(got, item.File+":"+strconv.Itoa(item.Line))
		}
	}
	if want := []string{"00_intro/intro.tex:2", "book.tex:0"}; !slices.Equal(got, want) {
		t.Fatalf("drawings at %v, want %v", got, want)
	}
}
