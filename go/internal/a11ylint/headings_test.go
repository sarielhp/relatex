package a11ylint

import (
	"path/filepath"
	"strings"
	"testing"
)

func headingSummary(items []Item) []string {
	lines := []string{}
	for _, item := range items {
		lines = append(lines, item.Kind+" "+itoa(item.Line)+" "+item.Suggestion)
	}
	return lines
}

// A paragraph that is only bold text is a fake heading, in each of the forms an author writes:
// \textbf{X}, {\bf X}, {\bfseries X}, behind \noindent, and across two lines.
func TestHeadingItemsFindBoldParagraphs(t *testing.T) {
	source := strings.Join([]string{
		`\documentclass{article}`,
		`\begin{document}`,
		``,
		`\textbf{Examples:}`,
		``,
		`{\bf Conditions for extension}`,
		``,
		`\noindent {\bf References}`,
		``,
		`{\bfseries A fourth heading}`,
		``,
		`\noindent`,
		`\textbf{Evaluation Metrics.}`,
		``,
		`\end{document}`,
	}, "\n")
	items := headingItems(newFile("main.tex", source))
	want := []string{
		`heading 4 \paragraph{Examples}`,
		`heading 6 \paragraph{Conditions for extension}`,
		`heading 8 \paragraph{References}`,
		`heading 10 \paragraph{A fourth heading}`,
		`heading 12 \paragraph{Evaluation Metrics}`,
	}
	if got := headingSummary(items); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("heading items:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	first := items[0]
	if !first.Advisory || first.Place != PlaceInstead || first.NoteLine != 4 || first.File != "main.tex" ||
		!strings.Contains(first.Message, `("Examples")`) ||
		!strings.Contains(first.Fix, `\subsubsection*{Examples}`) ||
		!strings.Contains(first.Note, `“Examples” is a paragraph of bold text`) {
		t.Errorf("first item %+v", first)
	}
}

// Near misses: bold text in prose, several bold words, a label environment, a macro body, a
// theorem label, more than eight words and inline math are not headings.
func TestHeadingItemsSkipNearMisses(t *testing.T) {
	cases := []struct{ name, source string }{
		{"bold inside a sentence", `Call \textbf{this} a name.`},
		{"two bold words and prose", `\textbf{First} and \textbf{second}`},
		{"inside itemize", "\\begin{itemize}\n\n\\textbf{Examples:}\n\n\\end{itemize}"},
		{"inside a table", "\\begin{table}\n\n\\textbf{Examples:}\n\n\\end{table}"},
		{"inside a macro definition", "\\newcommand{\\foo}{\n\n\\textbf{Examples:}\n\n}"},
		{"theorem label", `\textbf{Theorem 3.}`},
		{"proof label", `\textbf{Proof.}`},
		{"nine words", `\textbf{one two three four five six seven eight nine}`},
		{"math", `\textbf{The $L^2$ space}`},
		{"two levels of braces", `\textbf{The \emph{\textbf{x}} spaces}`},
		{"a sentence", `\textbf{This is a whole sentence of prose with far too many words in it}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if items := headingItems(newFile("main.tex", test.source)); len(items) != 0 {
				t.Errorf("items %+v", items)
			}
		})
	}
}

// A beamer document labels slides with bold text on purpose: its class or a frame settles it.
func TestHeadingItemsSkipBeamer(t *testing.T) {
	slides := map[string]string{
		"beamer class": "\\documentclass{beamer}\n\\begin{document}\n\\begin{frame}\n\n\\textbf{Before:}\n\n\\end{frame}\n\\end{document}\n",
		"frame only":   "\\begin{document}\n\\begin{frame}\n\n\\textbf{Before:}\n\n\\end{frame}\n\\end{document}\n",
	}
	for name, source := range slides {
		t.Run(name, func(t *testing.T) {
			directory := writePaper(t, map[string]string{"main.tex": source})
			for _, item := range lint(t, directory, Options{}, nil) {
				if item.Kind == "heading" {
					t.Errorf("beamer heading %+v", item)
				}
			}
		})
	}
}

// The annotated copy marks a fake heading and writes the replacement in place of its line; the
// original source is unchanged.
func TestAnnotateHeading(t *testing.T) {
	source := "\\documentclass{article}\n\\begin{document}\n\n\\textbf{Examples:}\n\n\\end{document}\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	before := treeHash(t, directory)
	destination := filepath.Join(t.TempDir(), "review")
	marks := annotate(t, directory, destination, Options{})
	if after := treeHash(t, directory); after != before {
		t.Fatal("the original changed")
	}
	copied := read(t, filepath.Join(destination, "main.tex"))
	if !strings.Contains(copied, MarkPrefix+" heading ") ||
		!strings.Contains(copied, `% \paragraph{Examples}  % relatex-a11y`) ||
		!strings.Contains(copied, "uncomment it in place of the line above") {
		t.Errorf("review copy:\n%s", copied)
	}
	placed := false
	for _, mark := range marks {
		placed = placed || mark.Kind == "heading" && mark.CopyLine > 0
	}
	if !placed {
		t.Errorf("marks %+v", marks)
	}
}
