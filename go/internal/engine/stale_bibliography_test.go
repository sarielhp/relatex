package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStaleFixture(t *testing.T, tex string) build {
	t.Helper()
	source := t.TempDir()
	files := map[string]string{
		"paper.tex": tex,
		"paper.bbl": "% $ biblatex bbl format version 3.3 $\n\\refsection{0}\n  \\sortlist[entry]{nty/global/}\n  \\endsortlist\n\\endrefsection\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return build{source: source, working: source, output: t.TempDir(), job: "paper"}
}

// A biblatex-format paper.bbl beside a BibTeX document is stale (P8): it is set aside with a warning.
func TestAStaleBiblatexBibliographyOfABibtexDocumentIsSetAside(t *testing.T) {
	b := writeStaleFixture(t, "\\documentclass{article}\\begin{document}\\bibliography{refs}\\end{document}")
	if !staleBiblatexBibliography(b) {
		t.Fatal("the stale biblatex bbl is not recognised")
	}
	p := &Probe{}
	p.setAsideStaleBibliography(b)
	if isFile(filepath.Join(b.working, "paper.bbl")) || !isFile(filepath.Join(b.output, "supplied.bbl")) {
		t.Fatal("paper.bbl was not set aside")
	}
	if len(p.degradations) != 1 || p.degradations[0].(map[string]any)["level"] != "warning" {
		t.Fatalf("degradations: %v", p.degradations)
	}
}

// A biblatex document's rejected paper.bbl would shadow the processor's: it is set aside once, warned once.
func TestARejectedBiblatexBibliographyIsSetAsideOnce(t *testing.T) {
	b := writeStaleFixture(t, "\\documentclass{article}\\usepackage{biblatex}\\begin{document}\\end{document}")
	p := &Probe{}
	p.setAsideRejectedBibliography(b)
	if shippedBibliography(b) || !isFile(filepath.Join(b.output, "supplied.bbl")) {
		t.Fatal("paper.bbl was not set aside")
	}
	if err := os.WriteFile(filepath.Join(b.working, "paper.bbl"), []byte("again"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.setAsideRejectedBibliography(b)
	if len(p.degradations) != 1 {
		t.Fatalf("degradations: %v", p.degradations)
	}
}

func TestABiblatexBibliographyOfABiblatexDocumentIsKept(t *testing.T) {
	b := writeStaleFixture(t, "\\documentclass{article}\\usepackage{biblatex}\\begin{document}\\end{document}")
	if staleBiblatexBibliography(b) {
		t.Fatal("a biblatex document's bbl is not stale")
	}
}
