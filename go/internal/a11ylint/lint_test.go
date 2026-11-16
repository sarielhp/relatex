package a11ylint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/figdesc"
)

const mainSource = `\documentclass{article}
\usepackage{graphicx}
\begin{document}
% \includegraphics{commented}
\input{body}
\begin{figure}\includegraphics[width=2cm]{figs/plain}\caption{Plain.}\end{figure}
\includegraphics[alt={A disk}]{disk}
\includegraphics[artifact]{rule}
\includegraphics[alt={}]{logo}
\includegraphics{drafted}
\includegraphics{written}
See \href{https://example.org}{here} or \href{https://example.org}{the example site} or \href{https://example.org}{https://example.org}.
$|x| + \|y\|$
\end{document}
`

const bodySource = `\begin{tabular}{cc}
a & b \\
1 & 2 \\
\end{tabular}
\begin{tabular}{cc}
a & b \\ \hline
1 & 2 \\ 3 & 4 \\
\end{tabular}
\begin{figure}\begin{tikzpicture}\draw (0,0) -- (1,1);\end{tikzpicture}\end{figure}
`

func writePaper(t *testing.T, files map[string]string) string {
	t.Helper()
	directory := t.TempDir()
	for name, text := range files {
		if err := authordir.Ensure(filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func lint(t *testing.T, directory string, options Options, drawings []Drawing) []Item {
	t.Helper()
	main, err := FindMain(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	files, err := Load(directory, main)
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := figdesc.Read(filepath.Join(directory, figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return Lint(Paper{Files: files, Sidecar: sidecar, Drawings: drawings}, options)
}

func summary(items []Item) []string {
	lines := []string{}
	for _, item := range items {
		lines = append(lines, item.Kind+" "+item.File+":"+itoa(item.Line))
	}
	return lines
}

func itoa(number int) string {
	digits := ""
	for ; number > 0; number /= 10 {
		digits = string(rune('0'+number%10)) + digits
	}
	if digits == "" {
		return "0"
	}
	return digits
}

func TestLintListsWhatOnlyTheAuthorCanSupply(t *testing.T) {
	directory := writePaper(t, map[string]string{"main.tex": mainSource, "body.tex": bodySource,
		figdesc.FileName: "[figs.drafted]\nalt = \"Draft.\"\nprovenance = \"model\"\n[figs.written]\nalt = \"Mine.\"\n"})
	items := lint(t, directory, Options{}, nil)
	got := strings.Join(summary(items), "\n")
	want := strings.Join([]string{"language main.tex:1", "figure main.tex:6", "draft main.tex:10", "link main.tex:12", "link main.tex:12",
		"table body.tex:1", "drawing body.tex:9"}, "\n")
	if got != want {
		t.Errorf("items:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(items[1].Fix, `\includegraphics[alt={…}]{figs/plain}`) || !strings.Contains(items[1].Fix, "[figs.plain]") {
		t.Errorf("figure fix %q", items[1].Fix)
	}
	math := lint(t, directory, Options{Math: true}, nil)
	last := math[len(math)-1]
	if last.Kind != "math" || last.Line != 3 || !strings.Contains(last.Message, "2 vertical bars and 2 double bars") {
		t.Errorf("math item %+v", last)
	}
	if !strings.Contains(last.Fix, `\DeclarePairedDelimiter\card`) || !strings.Contains(last.Fix, `\det A`) {
		t.Errorf("math fix %q", last.Fix)
	}
	// Decision 114, item 5: the document-level bar declaration is gone; a relatex-intent comment no
	// longer settles the item, and the fix never proposes one.
	declared := writePaper(t, map[string]string{"main.tex": "% relatex-intent: |x| absolute-value\n% relatex-intent: \\|x\\| norm\n" +
		mainSource, "body.tex": bodySource})
	items = lint(t, declared, Options{Math: true}, nil)
	if last := items[len(items)-1]; last.Kind != "math" || strings.Contains(last.Fix, "relatex-intent") {
		t.Errorf("a bar declaration settles nothing: %+v", last)
	}
}

func TestLintReadsDeclarationsAndDescriptions(t *testing.T) {
	directory := writePaper(t, map[string]string{"paper.tex": `\DocumentMetadata{lang=de}
\documentclass[sigconf]{acmart}
\begin{document}
\begin{figure}\includegraphics{a}\Description{A disk.}\end{figure}
\begin{figure}\includegraphics{b}\Description{}\end{figure}
\begin{figure}\begin{tikzpicture}\end{tikzpicture}\Description[Short]{Long.}\end{figure}
\end{document}
`})
	if got := summary(lint(t, directory, Options{}, nil)); strings.Join(got, ",") != "figure paper.tex:5,drawing paper.tex:6" {
		t.Errorf("items %v", got)
	}
	babel := writePaper(t, map[string]string{"main.tex": "\\documentclass{article}\n\\usepackage[ngerman]{babel}\n\\begin{document}\\end{document}\n"})
	if items := lint(t, babel, Options{Math: true}, nil); len(items) != 0 {
		t.Errorf("babel items %+v", items)
	}
}

// relatex's \relatexalt (decision 125, amendment 2) describes its figure's graphics and drawings in
// any class; one without an alternative, or with the annotated copy's placeholder, does not.
func TestLintReadsRelatexAlt(t *testing.T) {
	directory := writePaper(t, map[string]string{"paper.tex": `\documentclass[english]{article}
\usepackage{graphicx,tikz}
\ProvideDocumentCommand\relatexalt{m m o}{}
\begin{document}
\begin{figure}\includegraphics{a}\relatexalt{A disk.}{Its centre is p.}[Drawn in two steps.]\end{figure}
\begin{figure}\begin{tikzpicture}\end{tikzpicture}\relatexalt{A line.}{}\end{figure}
\begin{figure}\includegraphics{b}\relatexalt{ }{Only a short text.}\end{figure}
\begin{figure}\includegraphics{c}\relatexalt{TODO: describe}{}\end{figure}
\end{document}
`})
	if got := summary(lint(t, directory, Options{}, nil)); strings.Join(got, ",") != "figure paper.tex:7,figure paper.tex:8" {
		t.Errorf("items %v", got)
	}
}

func TestLintNamesTheLastConversionsDrawings(t *testing.T) {
	directory := writePaper(t, map[string]string{"main.tex": "\\documentclass[english]{article}\n\\begin{document}\n" +
		"\\begin{figure}\\begin{tikzpicture}\\end{tikzpicture}\\end{figure}\n\\end{document}\n",
		figdesc.FileName: "[figs.figure-2]\nalt = \"Two.\"\n"})
	items := lint(t, directory, Options{}, []Drawing{{Key: "figure-1"}, {Key: "figure-2"}, {Key: "figure-3", Described: true}})
	if len(items) != 1 || items[0].Kind != "drawing" || !strings.Contains(items[0].Message, "figs/figure-1") {
		t.Errorf("items %+v", items)
	}
}

// A complex figure (decision 110) with a short description only is a note at its graphic; one
// with a long description, one the sidecar gives a long text since, one without any description
// (listed as such already) and a simple one are not.
func TestLintNotesComplexFiguresWithAShortDescriptionOnly(t *testing.T) {
	directory := writePaper(t, map[string]string{"main.tex": "\\documentclass[english]{article}\n\\usepackage{graphicx}\n" +
		"\\begin{document}\n\\includegraphics[alt={A plot}]{plot}\n\\includegraphics[alt={B}]{rich}\n" +
		"\\includegraphics[alt={C}]{later}\n\\includegraphics[alt={D}]{small}\n\\end{document}\n",
		figdesc.FileName: "[figs.later]\nalt = \"C\"\nlong = \"Written since.\"\n"})
	main, _ := FindMain(directory, "")
	files, _ := Load(directory, main)
	sidecar, _ := figdesc.Read(filepath.Join(directory, figdesc.FileName))
	rich := figdesc.Complexity{Glyphs: 80, Marks: 300, Measured: true}
	paper := Paper{Files: files, Sidecar: sidecar, Drawings: []Drawing{}, Complex: []ComplexFigure{
		{Key: "plot-1", Source: "figs/plot.pdf", Measure: rich, Described: true}, {Key: "rich", Measure: rich, Described: true, Long: true},
		{Key: "later", Measure: rich, Described: true}, {Key: "small", Measure: figdesc.Complexity{Glyphs: 10, Marks: 20, Measured: true}, Described: true},
		{Key: "bare", Measure: rich}}}
	items := Lint(paper, Options{})
	if len(items) != 1 || items[0].Kind != "long" || !items[0].Advisory || items[0].Line != 4 || items[0].NoteLine != 4 ||
		items[0].Suggestion != `long = "…"` || !strings.Contains(items[0].Note, "[figs.plot-1] in "+figdesc.FileName) ||
		!strings.Contains(items[0].Message, "80 text glyphs") {
		t.Errorf("items %+v", items)
	}
	// The review copy marks the figure's line and REVIEW.md carries the note; no line to uncomment
	// is written into the TeX, as the long text belongs in the sidecar.
	destination := filepath.Join(t.TempDir(), "review")
	if _, err := Annotate(paper, items, directory, destination, nil); err != nil {
		t.Fatal(err)
	}
	copied, review := read(t, filepath.Join(destination, "main.tex")), read(t, filepath.Join(destination, ReviewFile))
	if !strings.Contains(copied, MarkPrefix+" long 1") || strings.Contains(copied, `% long = "…"`) ||
		!strings.Contains(review, "note (line 4 of the original): this figure looks complex") {
		t.Errorf("review copy:\n%s\n%s", copied, review)
	}
}

func TestFindMainNeedsOneCandidate(t *testing.T) {
	directory := writePaper(t, map[string]string{"a.tex": `\begin{document}`, "b.tex": `\begin{document}`})
	if _, err := FindMain(directory, ""); err == nil || !strings.Contains(err.Error(), "--main") {
		t.Errorf("two candidates: %v", err)
	}
	if main, err := FindMain(directory, "b.tex"); err != nil || main != "b.tex" {
		t.Errorf("requested: %q %v", main, err)
	}
}
