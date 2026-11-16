package a11ylint

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// prefix is modelled on the CS 574 homeworks' styles/prefix_latex.tex (the QED duck, lines 160-170
// and the \qedsymbol alias of line 1066): the image sits in a macro the QED symbol calls.
const qedPrefix = `\newlength{\CharHeight}
\AtBeginDocument{\setlength{\CharHeight}{\fontcharht\font` + "`X" + `}}

\IfFileExists{styles/qed_duck.pdf}{%
   \providecommand{\myqedsymbol}{%
      \reflectbox{\includegraphics[height=1.4\CharHeight]{styles/qed_duck.pdf}}%
   }%
}{%
   \providecommand{\myqedsymbol}{$\blacksquare$}%
}
\def\qedsymbol{\myqedsymbol}%
`

func figureTargets(items []Item) []string {
	targets := []string{}
	for _, item := range items {
		if item.Kind == "figure" {
			targets = append(targets, item.Target)
		}
	}
	return targets
}

func qedPaper(t *testing.T, body string) Paper {
	t.Helper()
	paper := hintPaper(t, "\\documentclass[english]{article}\n\\usepackage{graphicx}\n"+qedPrefix+"\\begin{document}\n"+body+"\\end{document}\n")
	paper.Sidecar = &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}
	return paper
}

// An image in the QED symbol's macro is a mark the export names "End of proof": no figure item;
// an ordinary figure of the same file still is one.
func TestQEDImageMacroIsNoFigureItem(t *testing.T) {
	paper := qedPaper(t, "\\includegraphics{figs/plot}\n")
	got := strings.Join(figureTargets(Lint(paper, Options{})), ",")
	if got != "figs/plot" {
		t.Errorf("figure items = %q, want only figs/plot", got)
	}
}

// A macro the QED symbol does not call keeps its figure item.
func TestImageMacroOutsideQEDKeepsItem(t *testing.T) {
	paper := hintPaper(t, "\\documentclass{article}\n\\usepackage{graphicx}\n"+
		"\\newcommand{\\logo}{\\includegraphics{styles/logo.pdf}}\n\\def\\qedsymbol{\\blacksquare}\n\\begin{document}\\logo\\end{document}\n")
	paper.Sidecar = &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}
	if got := figureTargets(Lint(paper, Options{})); len(got) != 1 || got[0] != "styles/logo.pdf" {
		t.Errorf("figure items = %v", got)
	}
}

// With a conversion the document's own QED images decide, whatever the source's macros look like.
func TestConversionQEDKeysDropFigureItem(t *testing.T) {
	paper := hintPaper(t, "\\documentclass{article}\n\\usepackage{graphicx}\n"+
		"\\newcommand{\\mark}{\\includegraphics{styles/duck.pdf}}\n\\begin{document}\\mark\\includegraphics{figs/plot}\\end{document}\n")
	paper.Sidecar = &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}
	paper.QEDKeys = map[string]bool{"duck": true}
	if got := figureTargets(Lint(paper, Options{})); len(got) != 1 || got[0] != "figs/plot" {
		t.Errorf("figure items = %v", got)
	}
}
