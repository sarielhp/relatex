package a11ylint

import (
	"strings"
	"testing"
)

// A caption naming colours and prose pointing to a colour are items; a figure's own description,
// a colour command's argument and a red-black tree are not.
func TestColourItems(t *testing.T) {
	source := strings.Join([]string{
		`\begin{figure}\includegraphics[alt={A grey square}]{square}`,
		`\caption{The bound (red) and the samples (blue dots).}\end{figure}`,
		`The points shown in green are optimal; \textcolor{red}{this} is not prose about red-black trees.`,
		`\Description{Blue bars over a grey background.}`,
	}, "\n")
	items := colourItems(newFile("main.tex", source), nil)
	got := []string{}
	for _, item := range items {
		got = append(got, item.Message)
	}
	want := []string{
		"the caption names colours (red, blue); colour may be the only thing that tells them apart",
		`the text points to a colour ("shown in green")`,
	}
	if strings.Join(got, "|") != strings.Join(want, "|") || items[0].Line != 2 || items[1].Line != 3 {
		t.Errorf("colour items = %+v", items)
	}
}

// A colour word that names a thing (the red carpet of a navigation instruction) or sits in a string typed
// as code (a quoted prompt) is no cue; a caption whose colour tells data apart stays (papers6_1423,
// papers6_0722, papers6_0302 of the night run's sample).
func TestColourItemsSkipThingsAndCode(t *testing.T) {
	source := strings.Join([]string{
		`\caption{\textbf{R2R:} Walk straight out the door in front of you and follow the red carpet. Keep going.}`,
		"For the prompt ``\\texttt{\\small the cap of the blue marker}\", the model succeeded; \\verb|the red line| too.",
		`\caption{The color of points represent their cluster ids; red stars highlight the cluster seeds.}`,
		`The red line is the bound; a \texttt{blue} string is not.`,
	}, "\n")
	items := colourItems(newFile("main.tex", source), nil)
	got := []int{}
	for _, item := range items {
		got = append(got, item.Line)
	}
	if len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Errorf("colour items on lines %v, want [3 4]: %+v", got, items)
	}
}
