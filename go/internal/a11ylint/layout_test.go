package a11ylint

import (
	"testing"
)

// tableItemsOf is the lint's table items of a paper whose main file is body.
func tableItemsOf(t *testing.T, body string) []Item {
	t.Helper()
	source := "\\documentclass{article}\n\\usepackage{graphicx}\n\\begin{document}\n" + body + "\n\\end{document}\n"
	items := lint(t, writePaper(t, map[string]string{"main.tex": source}), Options{}, nil)
	found := []Item{}
	for _, item := range items {
		if item.Kind == "table" {
			found = append(found, item)
		}
	}
	return found
}

func expectNoTableItem(t *testing.T, name, body string) {
	t.Helper()
	if items := tableItemsOf(t, body); len(items) != 0 {
		t.Errorf("%s: table items %v, want none", name, summary(items))
	}
}

func expectTableItem(t *testing.T, name, body string) {
	t.Helper()
	if items := tableItemsOf(t, body); len(items) != 1 {
		t.Errorf("%s: table items %v, want one", name, summary(items))
	}
}

// A tabular used only to arrange figures is a layout (decision 111): it needs no header row, and
// a tabular in the body of a command is a definition, not a table.
func TestLintSkipsLayoutTabulars(t *testing.T) {
	grid := `\begin{tabular}{cc}
\includegraphics{a} & \includegraphics{b} \\
\includegraphics{c} & \includegraphics{d} \\
\end{tabular}`
	expectNoTableItem(t, "picture grid", grid)

	// The pictures stay figure items: only the table item goes.
	source := "\\documentclass{article}\n\\usepackage{graphicx}\n\\begin{document}\n" + grid + "\n\\end{document}\n"
	figures := 0
	for _, item := range lint(t, writePaper(t, map[string]string{"main.tex": source}), Options{}, nil) {
		if item.Kind == "figure" {
			figures++
		}
	}
	if figures != 4 {
		t.Errorf("picture grid: %d figure items, want 4", figures)
	}

	expectNoTableItem(t, "labels beside the pictures", `\begin{tabular}{cc}
\includegraphics{a} (A) & \includegraphics{b} (B) \\
\includegraphics{c} (C) & \includegraphics{d} (D) \\
\end{tabular}`)
	expectNoTableItem(t, "labels in their own row", `\begin{tabular}{cc}
\includegraphics{a} & \includegraphics{b} \\
(A) & (B) \\
\end{tabular}`)
	expectNoTableItem(t, "macro grid", `\newcommand{\IG}[1]{\includegraphics{#1}}
\begin{tabular}{cc}
\IG{a} & \IG{b} \\
\IG{c} & \IG{d} \\
\end{tabular}`)
	expectNoTableItem(t, "nested macro grid", `\newcommand{\Inner}[1]{\includegraphics{#1}}
\newcommand{\IG}[1]{\Inner{#1}}
\begin{tabular}{cc}
\IG{a} & \IG{b} \\
\IG{c} & \IG{d} \\
\end{tabular}`)
	expectNoTableItem(t, "a definition", `\providecommand{\Two}[2]{\begin{tabular}{cc}#1&#2\\x&y\end{tabular}}`)
}

// A tabular with content of its own keeps its item: numbers, text beside a picture, or a ruled
// booktabs table (whose \toprule is not under the first row).
func TestLintKeepsDataTabulars(t *testing.T) {
	expectTableItem(t, "numbers", `\begin{tabular}{cc}
1 & 2 \\
3 & 4 \\
\end{tabular}`)
	expectTableItem(t, "text beside a picture", `\begin{tabular}{cc}
\includegraphics{a} & data \\
\includegraphics{b} & more \\
\end{tabular}`)
	expectTableItem(t, "booktabs", `\begin{tabular}{ll}
\toprule
\rowcolor{blue} a & b \\
1 & 2 \\
\bottomrule
\end{tabular}`)
}
