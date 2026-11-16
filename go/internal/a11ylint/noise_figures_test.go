package a11ylint

import (
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// An empty caption in a float whose figures are described (alt key, the author's sidecar entry, the
// conversion's described places) is no item: the description says what the figure shows. A model
// draft, an undescribed place or a float without figures keeps the advice (decision 167).
func TestCaptionOfADescribedFloatIsNoItem(t *testing.T) {
	source := strings.Join([]string{
		`\begin{figure}\includegraphics[alt={A disk}]{disk}\caption{}\end{figure}`,
		`\begin{figure}\includegraphics{written}\caption{}\end{figure}`,
		`\begin{figure}\includegraphics{drafted}\caption{}\end{figure}`,
		`\begin{figure}\IncGraph{figs/walk}\caption{}\end{figure}`,
		`\begin{figure}\IncGraph{figs/open}\caption{}\end{figure}`,
		`\begin{figure}\caption{}\end{figure}`,
		`\begin{figure}\includegraphics[alt={A disk}]{disk}\caption*{}\end{figure}`,
	}, "\n")
	file := newFile("main.tex", source)
	sidecar := &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{
		"written": {Alt: "Written by the author"},
		"drafted": {Alt: "A model draft", Provenance: figdesc.Model},
	}}
	paper := Paper{Files: []*File{file}, Sidecar: sidecar, Included: []IncludedFigure{
		{Key: "walk", Places: []figdesc.Place{{File: "main.tex", Line: 4, Described: true}}},
		{Key: "open", Places: []figdesc.Place{{File: "main.tex", Line: 5}}},
	}}
	items := captionItems(file, floatDescriber(file, paper))
	want := []captionResult{
		{3, true, captionNumberedMessage},
		{5, true, captionNumberedMessage},
		{6, true, captionNumberedMessage},
	}
	got := []captionResult{}
	for _, item := range items {
		got = append(got, captionResult{item.Line, item.Advisory, item.Message})
	}
	if !slices.Equal(got, want) {
		t.Fatalf("caption items %+v, want %+v", got, want)
	}
}

// A figure included at several places is one item naming the others; an xfig pdf_t picture that the
// conversion rendered as a drawing is the drawing's item, not its \includegraphics's (decision 167).
func TestFigureItemsOncePerFigure(t *testing.T) {
	directory := writeTree(t, map[string]string{
		"main.tex": strings.Join([]string{
			`\documentclass{article}\usepackage{graphicx}\begin{document}`,
			`\includegraphics{figs/walk}`,
			`\includegraphics{figs/walk}`,
			`\input{sets.pdf_t}`,
			`\includegraphics{figs/walk}\end{document}`,
		}, "\n"),
		"sets.pdf_t": strings.Join([]string{
			`\begin{picture}(0,0)%`,
			`\includegraphics{sets.pdf}%`,
			`\end{picture}%`,
			`\begin{picture}(2775,910)(346,-870)\put(361,-601){$T$}\end{picture}`,
		}, "\n"),
	})
	files, err := Load(directory, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	paper := Paper{Files: files, Sidecar: &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}, Drawings: []Drawing{
		{Key: "fig-1", Places: []figdesc.Place{{File: "sets.pdf_t", Line: 1}}},
		{Key: "fig-2", Places: []figdesc.Place{{File: "sets.pdf_t", Line: 4}}},
	}}
	items := Lint(paper, Options{})
	got := summary(items)
	want := []string{"language main.tex:1", "figure main.tex:2", "drawing sets.pdf_t:1", "drawing sets.pdf_t:4"}
	if !slices.Equal(got, want) {
		t.Fatalf("items %v, want %v", got, want)
	}
	if !strings.HasSuffix(items[1].Message, "(also at main.tex:3, main.tex:5)") {
		t.Errorf("collapsed message %q", items[1].Message)
	}
}

// Two files whose shortened sidecar keys coincide stay two items; one file spelled with and without
// its extension is one.
func TestFigureItemsCollapseByFileNotByKey(t *testing.T) {
	items := collapseRepeats([]Item{
		{Kind: "figure", File: "m.tex", Line: 1, Key: "maris_carraro", Target: "maris_carraro_parisi_f2a.eps"},
		{Kind: "figure", File: "m.tex", Line: 2, Key: "maris_carraro", Target: "maris_carraro_parisi_f2b.eps"},
		{Kind: "figure", File: "s.tex", Line: 3, Key: "qed_duck", Target: "shared/qed_duck"},
		{Kind: "figure", File: "t.tex", Line: 4, Key: "qed_duck", Target: "shared/qed_duck.pdf"},
	})
	if got := summary(items); !slices.Equal(got, []string{"figure m.tex:1", "figure m.tex:2", "figure s.tex:3"}) {
		t.Fatalf("items %v", got)
	}
}
