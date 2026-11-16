package a11ylint

import (
	"slices"
	"testing"
)

// Decision 167: the lint's items are the author's real gaps, each once.

// A package of the paper's own is read for its commands; one TeX Live provides is not in the paper.
func TestLoadReadsThePapersOwnPackages(t *testing.T) {
	directory := writeTree(t, map[string]string{
		"main.tex":  `\documentclass{book}\usepackage[x]{amsmath, setup}\begin{document}\end{document}`,
		"setup.sty": `\newcommand{\fig}[4]{\includegraphics[#1]{figures/#3}}`,
	})
	files, err := Load(directory, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(files); !slices.Equal(got, []string{"main.tex", "setup.sty"}) {
		t.Fatalf("files %v, want main.tex and setup.sty", got)
	}
}

// TeX reads \input{shared/prefix} from the main file's directory first, so a chapter's own copy of
// the shared directory (and a styles/styles copy in a homework) is never loaded: its graphics would
// repeat every item once per chapter.
func TestLoadReadsInputsInTeXsOrder(t *testing.T) {
	directory := writeTree(t, map[string]string{
		"book.tex":                       `\documentclass{book}\begin{document}\input{01_intro/intro}\input{02_disk/disk}\end{document}`,
		"shared/prefix.tex":              `\newcommand{\Duck}{\includegraphics{shared/duck}}`,
		"01_intro/intro.tex":             `\input{shared/prefix}Intro.`,
		"01_intro/shared/prefix.tex":     `\newcommand{\Duck}{\includegraphics{shared/duck}}`,
		"02_disk/disk.tex":               `\input{shared/prefix}\input{local}Disk.`,
		"02_disk/shared/prefix.tex":      `\newcommand{\Duck}{\includegraphics{shared/duck}}`,
		"02_disk/local.tex":              `Only beside the chapter.`,
		"styles/prefix.tex":              `\input{styles/prefix_latex}`,
		"styles/prefix_latex.tex":        `\relax`,
		"styles/styles/prefix_latex.tex": `\relax`,
	})
	files, err := Load(directory, "book.tex")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"book.tex", "01_intro/intro.tex", "shared/prefix.tex", "02_disk/disk.tex", "02_disk/local.tex"}
	if got := paths(files); !slices.Equal(got, want) {
		t.Fatalf("files %v, want %v", got, want)
	}
	files, err = Load(directory, "styles/prefix.tex")
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(files); !slices.Equal(got, []string{"styles/prefix.tex", "styles/prefix_latex.tex"}) {
		t.Fatalf("files %v, want the main directory's styles/prefix_latex.tex", got)
	}
}
