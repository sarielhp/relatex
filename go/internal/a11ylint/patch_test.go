package a11ylint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// patchPaper loads a synthetic paper's files for Patch.
func patchPaper(t *testing.T, directory string) Paper {
	t.Helper()
	files, err := Load(directory, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	return Paper{Files: files, Sidecar: &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}}
}

func patched(t *testing.T, directory string, items []Item) string {
	t.Helper()
	diff, err := Patch(patchPaper(t, directory), items)
	if err != nil {
		t.Fatal(err)
	}
	return diff
}

// TestPatchAddsALanguageLine: a PlaceAfter suggestion is a new line after NoteLine, one diff
// section in git format with three lines of context.
func TestPatchAddsALanguageLine(t *testing.T) {
	source := "\\documentclass{article}\n\\begin{document}\nSome text.\n\\end{document}\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	diff := patched(t, directory, Lint(patchPaper(t, directory), Options{}))
	want := "--- a/main.tex\n+++ b/main.tex\n@@ -1,4 +1,4 @@\n-\\documentclass{article}\n" +
		"+\\documentclass[english]{article}\n \\begin{document}\n Some text.\n \\end{document}\n"
	if diff != want {
		t.Errorf("diff:\n%s\nwant:\n%s", diff, want)
	}
}

// TestPatchAddsTheLanguageWhereTheClassIs: a preamble the main file inputs (the author's book: book.tex inputs
// a prefix that sets \documentclass) takes the line after its \documentclass, never before it; a language
// declared in any input file is the document's.
func TestPatchAddsTheLanguageWhereTheClassIs(t *testing.T) {
	prefix := "\\documentclass{book}\n\\usepackage{amsmath}\n"
	main := "% book.tex\n\\input{prefix}\n\\begin{document}\nSome text.\n\\end{document}\n"
	directory := writePaper(t, map[string]string{"main.tex": main, "prefix.tex": prefix})
	diff := patched(t, directory, Lint(patchPaper(t, directory), Options{}))
	want := "--- a/prefix.tex\n+++ b/prefix.tex\n@@ -1,2 +1,2 @@\n-\\documentclass{book}\n" +
		"+\\documentclass[english]{book}\n \\usepackage{amsmath}\n"
	if diff != want {
		t.Errorf("diff:\n%s\nwant:\n%s", diff, want)
	}
	declared := writePaper(t, map[string]string{"main.tex": main, "prefix.tex": prefix + "\\usepackage[english]{babel}\n"})
	for _, item := range Lint(patchPaper(t, declared), Options{}) {
		if item.Kind == "language" {
			t.Errorf("babel in an input file declares the language: %+v", item)
		}
	}
}

// TestPatchAvoidsABabelClash: a document that defines a command babel defines (the author's book:
// \newcommand{\frq}{f}) would stop if the patch loaded babel; the language is a class option, which defines
// nothing.
func TestPatchAvoidsABabelClash(t *testing.T) {
	source := "\\documentclass[12pt]{memoir}\n\\newcommand{\\frq}{f}\n\\begin{document}\nSome text.\n\\end{document}\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	diff := patched(t, directory, Lint(patchPaper(t, directory), Options{}))
	want := "--- a/main.tex\n+++ b/main.tex\n@@ -1,4 +1,4 @@\n-\\documentclass[12pt]{memoir}\n" +
		"+\\documentclass[english,12pt]{memoir}\n \\newcommand{\\frq}{f}\n \\begin{document}\n Some text.\n"
	if diff != want {
		t.Errorf("diff:\n%s\nwant:\n%s", diff, want)
	}
}

// TestPatchReplacesTheNoteLine:a PlaceInstead suggestion replaces the line it names.
func TestPatchReplacesTheNoteLine(t *testing.T) {
	source := "\\DocumentMetadata{tagging=on}\n\\documentclass{article}\n\\begin{document}\nText.\n\\end{document}\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	diff := patched(t, directory, Lint(patchPaper(t, directory), Options{}))
	want := "--- a/main.tex\n+++ b/main.tex\n@@ -1,4 +1,4 @@\n-\\DocumentMetadata{tagging=on}\n" +
		"+\\DocumentMetadata{lang=en, tagging=on}\n \\documentclass{article}\n \\begin{document}\n Text.\n"
	if diff != want {
		t.Errorf("diff:\n%s\nwant:\n%s", diff, want)
	}
}

// TestPatchMergesOverlappingHunks: suggestions two lines apart share one hunk.
func TestPatchMergesOverlappingHunks(t *testing.T) {
	source := "line1\nline2\nline3\nline4\nline5\nline6\nline7\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	items := []Item{
		{Kind: "table", File: "main.tex", Place: PlaceAfter, NoteLine: 2, Suggestion: `\hline`},
		{Kind: "table", File: "main.tex", Place: PlaceAfter, NoteLine: 5, Suggestion: `\midrule`},
	}
	want := "--- a/main.tex\n+++ b/main.tex\n@@ -1,7 +1,9 @@\n line1\n line2\n+\\hline\n line3\n line4\n line5\n" +
		"+\\midrule\n line6\n line7\n"
	if diff := patched(t, directory, items); diff != want {
		t.Errorf("diff:\n%s\nwant:\n%s", diff, want)
	}
}

// TestPatchKeepsOneHeaderPerFile: two hunks far apart share their file's one git header.
func TestPatchKeepsOneHeaderPerFile(t *testing.T) {
	source := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	items := []Item{
		{Kind: "table", File: "main.tex", Place: PlaceAfter, NoteLine: 1, Suggestion: "a"},
		{Kind: "table", File: "main.tex", Place: PlaceAfter, NoteLine: 12, Suggestion: "b"},
	}
	diff := patched(t, directory, items)
	if strings.Count(diff, "--- a/main.tex\n") != 1 || strings.Count(diff, "@@ ") != 2 {
		t.Errorf("diff:\n%s", diff)
	}
}

// TestPatchMarksALastLineWithoutNewline: adding after a last line with no newline deletes and
// re-adds that line, with the standard marker on each side.
func TestPatchMarksALastLineWithoutNewline(t *testing.T) {
	directory := writePaper(t, map[string]string{"main.tex": "a\nb"})
	items := []Item{{Kind: "table", File: "main.tex", Place: PlaceAfter, NoteLine: 2, Suggestion: "x"}}
	want := "--- a/main.tex\n+++ b/main.tex\n@@ -1,2 +1,3 @@\n a\n-b\n\\ No newline at end of file\n" +
		"+b\n+x\n\\ No newline at end of file\n"
	if diff := patched(t, directory, items); diff != want {
		t.Errorf("diff:\n%s\nwant:\n%s", diff, want)
	}
}

// TestPatchLeavesFiguresToTheSidecar: a figure's suggestion is not a source hunk.
func TestPatchLeavesFiguresToTheSidecar(t *testing.T) {
	directory := writePaper(t, map[string]string{"main.tex": "line1\nline2\n"})
	items := []Item{{Kind: "figure", File: "main.tex", Place: PlaceAfter, NoteLine: 1, Suggestion: "figure"}}
	if diff := patched(t, directory, items); diff != "" {
		t.Errorf("figure in patch:\n%s", diff)
	}
}

// TestPatchAppliesAndSettlesTheLint: the patch applies with patch -p1 to a copy of the paper and
// the copy's lint no longer lists the language and table items.
func TestPatchAppliesAndSettlesTheLint(t *testing.T) {
	if _, err := exec.LookPath("patch"); err != nil {
		t.Skip("patch is not on PATH")
	}
	source := "\\documentclass{article}\n\\usepackage{booktabs}\n\\begin{document}\n\\begin{tabular}{cc}\n\\toprule\na & b \\\\\n1 & 2 \\\\\n\\bottomrule\n\\end{tabular}\n\\end{document}\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	paper := patchPaper(t, directory)
	before := summary(Lint(paper, Options{}))
	if len(before) != 2 {
		t.Fatalf("items before: %v", before)
	}
	diff := patched(t, directory, Lint(paper, Options{}))
	copied := t.TempDir()
	if err := os.WriteFile(filepath.Join(copied, "main.tex"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("patch", "-p1", "--quiet")
	command.Dir = copied
	command.Stdin = strings.NewReader(diff)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("patch: %v\n%s\n%s", err, output, diff)
	}
	if after := summary(Lint(patchPaper(t, copied), Options{})); len(after) != 0 {
		t.Errorf("items after the patch: %v\ndiff:\n%s", after, diff)
	}
}
