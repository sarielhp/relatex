package a11ylint

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// An undescribed panel is a figure item at its own line whatever its subcaption or the figure's
// caption says: the export counts it open (accessibility.imageItems, htmlkit.Undescribed: no author
// description), and a subcaption names a panel without describing it (decision 167, group 4).
func TestFigureItemsListEveryUndescribedPanel(t *testing.T) {
	source := strings.Join([]string{
		`\begin{figure}`,
		`\subfigure[Cumulative Returns]{\includegraphics{cum.png}}`,
		`\begin{subfigure}{.5\textwidth}\includegraphics{a.png}\subcaption{Left}\end{subfigure}`,
		`\subfloat[Described]{\includegraphics[alt={A line.}]{d.png}}`,
		`\caption{(a) Returns. (b) Left.}`,
		`\end{figure}`,
	}, "\n")
	items := figureItems(newFile("main.tex", source), Paper{Sidecar: &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}}, true)
	got := []int{}
	for _, item := range items {
		got = append(got, item.Line)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("figure item lines = %v, want [2 3]", got)
	}
}
