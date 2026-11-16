package a11ylint

import (
	"strings"
	"testing"
)

// An empty \caption{} in a subfigure panel is no item when the figure's own caption says what the
// panels show (papers4_1065, papers5_1185 and papers6_0360 of the night run's sample); without a
// parent caption, or under a generic one, it stays advice.
func TestCaptionItemsEmptyPanelNamedByParent(t *testing.T) {
	source := strings.Join([]string{
		`\begin{figure}`,
		`\begin{subfigure}[t]{0.95\columnwidth}`,
		`    \includegraphics[width=\columnwidth]{plots/LongLatLine.eps}`,
		`    \caption{}`,
		`    \label{fig:scenario1}`,
		`\end{subfigure}`,
		`\caption{(a) Scenario 1: formation of four satellites in a line. (b) Scenario 2.}`,
		`\end{figure}`,
		`\begin{figure*}`,
		`\subfigure[t]{\includegraphics{oca_graph3.png}\caption{}}`,
		`\caption{A graphical illustration of obstacle collision avoidance}`,
		`\end{figure*}`,
		`\begin{figure}`,
		`\begin{subfigure}{.5\textwidth}\includegraphics{a}\caption{}\end{subfigure}`,
		`\end{figure}`,
		`\begin{figure}`,
		`\begin{subfigure}{.5\textwidth}\includegraphics{b}\caption{}\end{subfigure}`,
		`\caption{Figure 2.}`,
		`\end{figure}`,
	}, "\n")
	items := captionItems(newFile("main.tex", source), func(int) bool { return false })
	got := []int{}
	for _, item := range items {
		got = append(got, item.Line)
	}
	if len(got) != 3 || got[0] != 14 || got[1] != 17 || got[2] != 18 {
		t.Errorf("caption item lines = %v, want [14 17 18] (the unnamed panels and the generic parent)", got)
	}
}
