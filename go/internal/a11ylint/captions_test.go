package a11ylint

import (
	"strings"
	"testing"
)

// A caption holding no text, or only the kind of float, names the figure or table by its number
// only (advice); an empty \caption* leaves it without a visible name (decision 167). A word that says what the picture shows is not flagged, however short, and a
// caption inside a command's definition or one holding a macro parameter is no caption yet.
func TestCaptionItems(t *testing.T) {
	source := strings.Join([]string{
		`\begin{figure}\caption{}\end{figure}`,
		`\begin{figure}\caption{\label{fig:x}}\end{figure}`,
		`\begin{figure}\caption*{ }\end{figure}`,
		`\begin{subfigure}{.5\textwidth}\subcaption{}\end{subfigure}`,
		`\begin{figure}\caption{Figure 3.}\end{figure}`,
		`\begin{figure}\caption{Plot}\end{figure}`,
		`\begin{figure}\caption{Input}\end{figure}`,
		`\begin{figure}\caption{Parent1}\end{figure}`,
		`\begin{figure}\caption{Varying $\delta$}\end{figure}`,
		`\caption{#1}`,
		`\newcommand{\Fig}[1]{\caption{#1}}`,
		`\begin{figure}\caption{The three hulls.}\label{fig:hulls}\end{figure}`,
		`\captionof{table}{}`,
		`\begin{table}\captionof{table}{Results}\end{table}`,
		`\begin{figure}\caption{\mycaption}\end{figure}`,
		`\begin{figure}\caption{\foo}\end{figure}`,
		`\begin{figure}\caption{\small\centering\label{fig:y}}\end{figure}`,
		`\begin{figure}\caption{\textbf{Results}}\end{figure}`,
		`\begin{figure}\caption{\textbf{Figure}}\end{figure}`,
		`\begin{figure}\caption{\,\;}\end{figure}`,
		`\begin{figure}\caption{\%}\end{figure}`,
	}, "\n")
	items := captionItems(newFile("main.tex", source), func(int) bool { return false })
	got := []captionResult{}
	for _, item := range items {
		got = append(got, captionResult{item.Line, item.Advisory, item.Message})
	}
	want := []captionResult{
		{1, true, "the caption has no text, so the figure/table is named by its number only"},
		{2, true, "the caption has no text, so the figure/table is named by its number only"},
		{3, false, "the caption has no text, so the figure/table has no visible name"},
		{4, true, "the caption has no text, so the figure/table is named by its number only"},
		{5, true, `the caption only repeats what kind of float it is ("Figure 3")`},
		{6, true, `the caption only repeats what kind of float it is ("Figure 3")`},
		{13, true, "the caption has no text, so the figure/table is named by its number only"},
		{17, true, "the caption has no text, so the figure/table is named by its number only"},
		{19, true, `the caption only repeats what kind of float it is ("Figure 3")`},
		{20, true, "the caption has no text, so the figure/table is named by its number only"},
	}
	if len(got) != len(want) {
		t.Fatalf("caption items = %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("caption item %d = %+v, want %+v", index, got[index], want[index])
		}
	}
	for _, item := range items {
		if item.Kind != "caption" || item.File != "main.tex" || item.Suggestion != "" || item.Place != "" {
			t.Errorf("caption item %+v", item)
		}
		if !strings.Contains(item.Fix, `\caption{…}`) || item.Note == "" || item.NoteLine != item.Line {
			t.Errorf("caption item note %+v", item)
		}
	}
}

// captionResult is the part of a caption item the test compares.
type captionResult struct {
	line     int
	advisory bool
	message  string
}
