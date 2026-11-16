package a11ylint

import "testing"

// \Re defined as the reals' double-struck R has one reading, so no hint is proposed; defined as an
// operator it keeps the hint (decision 167).
func TestHintSkipsASetSymbol(t *testing.T) {
	paper := hintPaper(t, "\\documentclass{article}\n\\AtBeginDocument{\\renewcommand{\\Re}{\\mathbb{R}}}\n\\begin{document}\n$x\\in\\Re$\n\\end{document}\n")
	if item := hintOf(t, Lint(paper, Options{}), `\Re`); item != nil {
		t.Errorf("a hint for \\Re as the reals: %+v", item)
	}
	paper = hintPaper(t, "\\documentclass{article}\n\\renewcommand{\\Re}{\\mathbb{R}}\n\\DeclareMathOperator{\\Re}{Re}\n")
	if item := hintOf(t, Lint(paper, Options{}), `\Re`); item == nil || item.NoteLine != 3 {
		t.Errorf("hint for \\Re as an operator = %+v, want line 3", item)
	}
}
