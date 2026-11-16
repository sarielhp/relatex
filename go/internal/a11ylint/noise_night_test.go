package a11ylint

import (
	"strings"
	"testing"
)

// A bare address named by its sentence is no link item (papers3_0726, papers3_0797 of the night run's
// sample); a bare address with no introduction and the vague "here" stay.
func TestLinkItemsSkipIntroducedAddresses(t *testing.T) {
	cases := []struct {
		name, source string
		items        int
	}{
		{"available at", `The code is available at: \href{https://github.com/a/b}{https://github.com/a/b}`, 0},
		{"code with spaces", `First, let us recall that the code is available at  \href{ https://github.com/a/b}{ https://github.com/a/b}. It is`, 0},
		{"repository and footnote", `We refer to the public code repository\footnote{\href{https://github.com/e/B}{https://github.com/e/B}} for details.`, 0},
		{"footnote only", `Data follow.\footnote{\href{https://example.org/x}{https://example.org/x}}`, 0},
		{"no introduction", `See \href{https://example.org/x}{https://example.org/x} for more.`, 1},
		{"another sentence names it", `The code is public. See \href{https://example.org/x}{https://example.org/x}.`, 1},
		{"here", `The code is available \href{https://github.com/a/b}{here}.`, 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if items := linkItems(newFile("main.tex", test.source)); len(items) != test.items {
				t.Errorf("items %+v, want %d", items, test.items)
			}
		})
	}
}

// A definition whose body types the operator's own name has said what it reads as (papers2_0232,
// papers2_0320, papers4_0996); one that spells another thing, or a DeclareMathOperator, keeps the hint.
func TestHintSkipsADefinitionTypingItsName(t *testing.T) {
	skipped := [][2]string{
		{`\Im`, `\renewcommand{\Im}{\operatorname{Im}}`},
		{`\Re`, `\renewcommand{\Re}{\operatorname{Re}}`},
		{`\Im`, `\renewcommand{\Im}{\textup{Im}}`},
		{`\tr`, `\newcommand{\tr}{\text{Tr}}`},
		{`\tr`, `\newcommand{\tr}{\mathrm{tr}}`},
	}
	for _, test := range skipped {
		paper := hintPaper(t, "\\documentclass{article}\n"+test[1]+"\n")
		if item := hintOf(t, Lint(paper, Options{}), test[0]); item != nil {
			t.Errorf("%s: a hint %+v", test[1], item)
		}
	}
	kept := map[string]string{`\Im`: `\renewcommand{\Im}{\mathrm{Imag}}`, `\tr`: `\newcommand{\tr}{\operatorname{Tr}\,}`}
	for name, definition := range kept {
		paper := hintPaper(t, "\\documentclass{article}\n"+definition+"\n")
		if item := hintOf(t, Lint(paper, Options{}), name); item == nil {
			t.Errorf("%s: no hint", definition)
		}
	}
}

// A bold line directly before the paragraph's text is a run-in label, even when a comment sits
// between them (papers6_0823 "Stage 1:", papers4_1118 "Evaluation Metrics."); a bold line without a closing
// colon or point (papers6_0496 "Advantages") or followed by a blank line stays a heading.
func TestHeadingItemsSkipRunInLabels(t *testing.T) {
	source := strings.Join([]string{
		`Figure \ref{fig:stl-all}(d) shows the core:`,
		``,
		`% \textbf{(1)}`,
		`\textbf{Stage 1:}`,
		`% We perform TopK indexing and compression on the activation by blocks.`,
		`The dense activation with ASM is read from the buffer.`,
		`\textbf{Stage 2:}`,
		`The dense activation then passes through precomputed logic.`,
		``,
		`\noindent`,
		`\textbf{Evaluation Metrics.}`,
		`% ViSQOL V3 is adopted.`,
		`ViSQOL V3 and PESQ are used to evaluate the quality.`,
		``,
		`\textbf{Examples:}`,
		`% a comment`,
		``,
		`Text.`,
		``,
		`\textbf{Advantages}`,
		`% a commented-out sentence`,
		`Compared to token-level decoding, this is better.`,
	}, "\n")
	items := headingItems(newFile("main.tex", source))
	if got := headingSummary(items); len(got) != 2 || got[0] != `heading 15 \paragraph{Examples}` || got[1] != `heading 20 \paragraph{Advantages}` {
		t.Errorf("heading items = %v", got)
	}
}
