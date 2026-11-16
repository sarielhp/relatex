package a11ylint

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
)

const reviewSource = `\documentclass{article}
\usepackage{graphicx}
\usepackage{hyperref}
\begin{document}
\begin{figure}
  \includegraphics[width=2cm]{figs/plot_a}% the plot
  \caption{Plain.}
\end{figure}
\includegraphics{drafted}
\includegraphics[alt={A grey square}]{square}
\input{table}
For the data, click \href{https://example.org}{here}.
$|x| + |y|$
\end{document}
`

const reviewTable = `\begin{tabular}{cc}
a & b \\
1 & 2 \\
\end{tabular}
`

const reviewSidecar = "[figs.drafted]\nalt = \"A 50% bar_chart of {x} & y.\"\nprovenance = \"model\"\n"

const wantMain = `%% relatex-a11y: language 1 — the document declares no language; the export says English. Fix: english as a \documentclass option, or \usepackage[english]{babel} (polyglossia: \setdefaultlanguage{english}; \DocumentMetadata{lang=en})
\documentclass{article}
% \documentclass[english]{article}  % relatex-a11y 1: uncomment it in place of the line above
\usepackage{graphicx}
\usepackage{hyperref}
%% relatex-a11y: math 6 — 4 vertical bars and 0 double bars in formulas carry no meaning a reader can announce. Fix: write each use with a macro that says what its bars mean, e.g. \DeclarePairedDelimiter\abs{\lvert}{\rvert}, \DeclarePairedDelimiter\card{\lvert}{\rvert} and \DeclarePairedDelimiter\norm{\lVert}{\rVert} (mathtools) for \abs{x}, \card{S} and \norm{v}, and \det A for a determinant; unmarked bars stay valid and are read as symbols
\begin{document}
\begin{figure}
%% relatex-a11y: figure 2 — figure figs/plot_a has no description. Fix: replace TODO: … by a description of the figure, or the alt key by artifact if it is decoration
  \includegraphics[width=2cm, alt={TODO: describe figs/plot\_a}]{figs/plot_a}% the plot
  \caption{Plain.}
\end{figure}
%% relatex-a11y: draft 3 — figure drafted has a model draft nobody has reviewed. Fix: check the draft against the figure, correct it and remove DRAFT:
\includegraphics[alt={DRAFT: A 50\% bar\_chart of \{x\} \& y.}]{drafted}
\includegraphics[alt={A grey square}]{square}
\input{table}
%% relatex-a11y: link 4 — the link text "here" does not say where the link goes. Fix: a text naming the target (\href{…}{the dataset on Zenodo})
For the data, click \href{https://example.org}{here}.
$|x| + |y|$
\end{document}
`

func treeHash(t *testing.T, directory string) string {
	t.Helper()
	sum := sha256.New()
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		sum.Write([]byte(path))
		sum.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func annotate(t *testing.T, directory, destination string, options Options) []Mark {
	t.Helper()
	files, err := Load(directory, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := figdesc.Read(filepath.Join(directory, figdesc.FileName))
	if err != nil {
		t.Fatal(err)
	}
	paper := Paper{Files: files, Sidecar: sidecar}
	marks, err := Annotate(paper, Lint(paper, options), directory, destination, nil)
	if err != nil {
		t.Fatal(err)
	}
	return marks
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestAnnotateMarksACopyAndLeavesTheOriginal: every lint item is a comment before its line, the
// figures carry alt placeholders (the model's draft as DRAFT:), the table and language their
// declarations commented out, the math one opt-in note; REVIEW.md lists the copy's lines, the
// original is byte for byte unchanged, and the lint of the copy still lists the placeholders.
func TestAnnotateMarksACopyAndLeavesTheOriginal(t *testing.T) {
	directory := writePaper(t, map[string]string{"main.tex": reviewSource, "table.tex": reviewTable, figdesc.FileName: reviewSidecar})
	if err := os.Mkdir(filepath.Join(directory, "figs"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := treeHash(t, directory)
	destination := filepath.Join(t.TempDir(), "review")
	marks := annotate(t, directory, destination, Options{Math: true})
	if after := treeHash(t, directory); after != before {
		t.Fatal("the original changed")
	}
	if got := read(t, filepath.Join(destination, "main.tex")); got != wantMain {
		t.Errorf("main.tex:\n%s", got)
	}
	table := read(t, filepath.Join(destination, "table.tex"))
	if !strings.HasPrefix(table, MarkPrefix+" table 5 — ") ||
		!strings.HasSuffix(table, "a & b \\\\\n% \\hline  % relatex-a11y 5: uncomment to add it\n1 & 2 \\\\\n\\end{tabular}\n") {
		t.Errorf("table.tex:\n%s", table)
	}
	if read(t, filepath.Join(destination, figdesc.FileName)) != reviewSidecar {
		t.Error("the sidecar is not copied")
	}
	summary := read(t, filepath.Join(destination, ReviewFile))
	for _, want := range []string{"6 marks.", "1. `main.tex:1` language:", "2. `main.tex:9` figure: figure figs/plot_a has no description",
		"   - placeholder: `alt={TODO: describe figs/plot\\_a}`", "3. `main.tex:13` draft:", "5. `table.tex:1` table:",
		"4. `main.tex:17` link:", "6. `main.tex:6` math:"} {
		if !strings.Contains(summary, want) {
			t.Errorf("REVIEW.md lacks %q:\n%s", want, summary)
		}
	}
	if len(marks) != 6 || marks[1].CopyLine != 9 {
		t.Errorf("marks %+v", marks)
	}
	got := strings.Join(placeholderSummary(lint(t, destination, Options{}, nil)), ",")
	if got != "language main.tex:2,figure main.tex:10,draft main.tex:14,link main.tex:18,table table.tex:2" {
		t.Errorf("lint of the copy: %s", got)
	}
}

func placeholderSummary(items []Item) []string {
	lines := summary(items)
	for index, item := range items {
		if item.Kind == "figure" && !strings.Contains(item.Message, "placeholder") ||
			item.Kind == "draft" && !strings.Contains(item.Message, DraftPrefix) {
			lines[index] += " (not a placeholder)"
		}
	}
	return lines
}

// TestAnnotatePlaceholdersOnlyWhereKeysCompile: no alt key for the graphics package's
// \includegraphics, none for the bounding-box form (no lint item); a drawing named only by the last conversion is
// listed in REVIEW.md without a line; an earlier copy is replaced, any other directory refused.
func TestAnnotatePlaceholdersOnlyWhereKeysCompile(t *testing.T) {
	source := "\\documentclass[english]{article}\n\\usepackage{graphics}\n\\begin{document}\n\\includegraphics{a}\n\\end{document}\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	destination := filepath.Join(t.TempDir(), "review")
	annotate(t, directory, destination, Options{})
	if got := read(t, filepath.Join(destination, "main.tex")); !strings.HasSuffix(got, "\\includegraphics{a}\n\\end{document}\n") ||
		!strings.Contains(got, MarkPrefix+" figure 1") {
		t.Errorf("graphics package:\n%s", got)
	}
	keyed := strings.Replace(source, "{graphics}", "{graphicx}", 1)
	keyed = strings.Replace(keyed, `\includegraphics{a}`, `\includegraphics[0,0][1,1]{a}`, 1)
	if err := os.WriteFile(filepath.Join(directory, "main.tex"), []byte(keyed), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Load(directory, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	paper := Paper{Files: files, Sidecar: &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}, Drawings: []Drawing{{Key: "figure-1"}}}
	marks, err := Annotate(paper, Lint(paper, Options{}), directory, destination, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(destination, "main.tex")); !strings.Contains(got, `\includegraphics[0,0][1,1]{a}`) {
		t.Errorf("bounding-box form:\n%s", got)
	}
	if len(marks) != 1 || marks[0].CopyLine != 0 || !strings.Contains(read(t, filepath.Join(destination, ReviewFile)),
		"1. `main.tex (no line in the source)` drawing:") {
		t.Errorf("marks %+v", marks)
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, refused := range []string{other, directory, filepath.Dir(directory)} {
		if _, err := Annotate(paper, nil, directory, refused, nil); err == nil {
			t.Errorf("annotating into %s was not refused", refused)
		}
	}
}
