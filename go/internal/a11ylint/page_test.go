package a11ylint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReviewPageGroupsKindsEscapesAndMarksTheItemLine: the page has one section per kind present,
// groups figure items of one target, escapes the source, carries no script, and keeps the excerpt
// inside the file at its first and last line.
func TestReviewPageGroupsKindsEscapesAndMarksTheItemLine(t *testing.T) {
	source := "\\documentclass{article} <tag> & more\n" +
		"\\begin{document}\n" +
		"\\usepackage[<babel>]{babel}\n" +
		"\\begin{tabular}{cc}\n" +
		"a & b \\\\\n" +
		"\\includegraphics{figs/plot}\n" +
		"\\end{document}\n"
	file := newFile("main.tex", source)
	product := t.TempDir()
	if err := os.MkdirAll(filepath.Join(product, "figs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, "figs", "plot.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A figure published only as its source PDF gets no picture: <img> cannot show a PDF.
	if err := os.WriteFile(filepath.Join(product, "figs", "edge.pdf"), []byte("%PDF-1.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items := []Item{
		{Kind: "language", File: "main.tex", Line: 2, NoteLine: 2, Message: "document has no language",
			Fix: "add <lang> & name", Suggestion: `\usepackage[<babel>]{babel}`},
		{Kind: "table", File: "main.tex", Line: 4, NoteLine: 4, Message: "table has no header row",
			Fix: "mark the header row", Suggestion: `\hline`},
		{Kind: "figure", File: "main.tex", Line: 6, NoteLine: 6, Key: "plot", Target: "figs/plot.pdf"},
		{Kind: "figure", File: "main.tex", Line: 6, NoteLine: 6, Key: "plot-2", Target: "figs/plot.pdf"},
		{Kind: "figure", File: "main.tex", Line: 6, NoteLine: 6, Key: "edge", Target: "figs/edge.pdf"},
		{Kind: "language", File: "main.tex", Line: 1, NoteLine: 1, Message: "first line", Fix: "fix first"},
		{Kind: "language", File: "main.tex", Line: 7, NoteLine: 7, Message: "last line", Fix: "fix last"},
	}
	page, figures := ReviewPage(Paper{Files: []*File{file}}, items, product,
		Crop{Item: 1, Page: 2, PNG: "crop-001.png", Box: Box{MinX: 1, MinY: 2, MaxX: 3, MaxY: 4}})
	for _, kind := range []string{"language", "table", "figure"} {
		if !strings.Contains(page, `<section id="`+kind+`">`) {
			t.Errorf("no section for %s", kind)
		}
	}
	if !strings.Contains(page, `<figure><img src="crops/crop-001.png" alt="Excerpt of the PDF page 2 around this item"><figcaption>Page 2</figcaption></figure>`) {
		t.Error("the table item's crop is not in its place")
	}
	if !strings.Contains(page, "document-level") {
		t.Error("a language item without a crop does not say it is document-level")
	}
	if strings.Contains(page, `<section id="draft">`) {
		t.Error("a draft section without draft items")
	}
	if got := strings.Count(page, "<script"); got != 1 || strings.Contains(page, "<script src") {
		t.Errorf("the page carries %d scripts, want one inline and no external load", got)
	}
	if !strings.Contains(page, `name="generator" content="`+ReviewPageGenerator+`"`) {
		t.Error("no generator marker")
	}
	if !strings.Contains(page, "&lt;tag&gt; &amp; more") || strings.Contains(page, "<tag>") {
		t.Error("the source line is not escaped")
	}
	if !strings.Contains(page, "add &lt;lang&gt; &amp; name") || !strings.Contains(page, "&lt;babel&gt;") {
		t.Error("the message or suggestion is not escaped")
	}
	if got := strings.Count(page, "<h3>"); got != 2 {
		t.Errorf("%d figure groups, want 2", got)
	}
	if !strings.Contains(page, "2 places") || !strings.Contains(page, "No picture of this figure") {
		t.Error("the figure group does not show its count and missing picture")
	}
	if len(figures) != 1 || figures["plot.svg"] == "" {
		t.Errorf("figures %v", figures)
	}
	if _, err := os.Stat(figures["plot.svg"]); err != nil {
		t.Errorf("published picture: %v", err)
	}
	if !strings.Contains(page, "    1  \\documentclass{article} &lt;tag&gt; &amp; more") {
		t.Error("the first line is not in the excerpt")
	}
	if !strings.Contains(page, "    7  \\end{document}") {
		t.Error("the last line is not in the excerpt")
	}
	if strings.Contains(page, "    0  ") {
		t.Error("the excerpt reaches line 0")
	}
}
