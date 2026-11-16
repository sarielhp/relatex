package a11ylint

import (
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// A graphic in the body of a command nothing calls is a template, not a place (papers6_1423's icon
// macros); a command that is called keeps its item at the definition.
func TestFigureItemsSkipAnUnusedTemplate(t *testing.T) {
	source := "\\documentclass{article}\n\\usepackage{graphicx}\n" +
		"\\newcommand*{\\CharMinar}{\\includegraphics[scale=0.08]{images/icons/char-minar}}%\n" +
		"\\newcommand*{\\Google}{\\includegraphics[scale=0.08]{images/icons/glass-building}}%\n" +
		"\\newcommand{\\Chain}[1]{\\Google #1}\n" +
		"\\begin{document}\n\\Chain{x}\\includegraphics{plain}\n\\end{document}\n"
	paper := hintPaper(t, source)
	paper.Sidecar = &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}
	got := figureTargets(Lint(paper, Options{}))
	if len(got) != 2 || got[0] != "images/icons/glass-building" || got[1] != "plain" {
		t.Errorf("figure items = %v, want the called Google icon and plain only", got)
	}
}
