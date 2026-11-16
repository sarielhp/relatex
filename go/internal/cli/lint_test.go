package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLintAccessibilityReportsAndExitsNonzeroOnlyWhenStrict: the lint prints compiler-style
// items with their fixes; the exit status is 1 only with --strict (decision 096).
func TestLintAccessibilityReportsAndExitsNonzeroOnlyWhenStrict(t *testing.T) {
	paper := t.TempDir()
	source := "\\documentclass{article}\n\\begin{document}\n\\includegraphics{plot}\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(paper, "main.tex"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(arguments ...string) (string, int) {
		var out bytes.Buffer
		status := Run(append([]string{"lint"}, arguments...), &out, os.Stderr)
		return out.String(), status
	}
	output := filepath.Join(paper, "none")
	text, status := run("--accessibility", paper, "--output", output)
	if status != 0 || !strings.Contains(text, "main.tex:3: figure: figure plot has no description") ||
		!strings.Contains(text, "2 items only the author can supply (1 language, 1 figure)") {
		t.Errorf("status %d, report:\n%s", status, text)
	}
	if _, status := run("--accessibility", "--strict", paper, "--output", output); status != 1 {
		t.Errorf("--strict status %d", status)
	}
	if text, _ := run("--accessibility", "--json", paper, "--output", output); !strings.Contains(text, `"count": 2`) {
		t.Errorf("json:\n%s", text)
	}
	if _, status := run(paper); status != 2 {
		t.Errorf("without --accessibility: status %d", status)
	}
}

// TestLintAnnotateWritesTheReviewCopy: --annotate DIR writes the marked copy and says so; the
// report is the lint's (decision 098).
func TestLintAnnotateWritesTheReviewCopy(t *testing.T) {
	paper := t.TempDir()
	source := "\\documentclass[english]{article}\n\\begin{document}\n\\includegraphics{plot}\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(paper, "main.tex"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(t.TempDir(), "review")
	var out bytes.Buffer
	status := Run([]string{"lint", "--accessibility", paper, "--output", filepath.Join(paper, "none"), "--annotate", copied}, &out, os.Stderr)
	if status != 0 || !strings.Contains(out.String(), "Annotated copy: "+copied+" (1 marks in the source, 0 listed only in REVIEW.md)") {
		t.Errorf("status %d, report:\n%s", status, out.String())
	}
	data, err := os.ReadFile(filepath.Join(copied, "main.tex"))
	if err != nil || !strings.Contains(string(data), "\\includegraphics[alt={TODO: describe plot}]{plot}") {
		t.Errorf("copy %q %v", data, err)
	}
}

// TestLintReviewPageWritesThePageAndRefusesAForeignDirectory: --review-page DIR writes the page and
// says so; a non-empty directory that is not an earlier page is refused and left alone.
func TestLintReviewPageWritesThePageAndRefusesAForeignDirectory(t *testing.T) {
	paper := t.TempDir()
	source := "\\documentclass{article}\n\\begin{document}\n\\includegraphics{plot}\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(paper, "main.tex"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(paper, "none")
	page := filepath.Join(t.TempDir(), "review")
	var out bytes.Buffer
	status := Run([]string{"lint", "--accessibility", paper, "--output", output, "--review-page", page}, &out, os.Stderr)
	want := "Review page: " + filepath.Join(page, "index.html") + " (2 items, 1 figure groups)"
	if status != 0 || !strings.Contains(out.String(), want) {
		t.Errorf("status %d, report:\n%s", status, out.String())
	}
	data, err := os.ReadFile(filepath.Join(page, "index.html"))
	if err != nil || !strings.Contains(string(data), `name="generator" content="relatex lint review"`) {
		t.Errorf("page %q, %v", data, err)
	}
	if status := Run([]string{"lint", "--accessibility", paper, "--output", output, "--review-page", page}, &out, os.Stderr); status != 0 {
		t.Errorf("an earlier review page was not replaced: status %d", status)
	}
	foreign := t.TempDir()
	keep := filepath.Join(foreign, "keep.txt")
	if err := os.WriteFile(keep, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status := Run([]string{"lint", "--accessibility", paper, "--output", output, "--review-page", foreign}, &out, os.Stderr); status == 0 {
		t.Error("a foreign directory was not refused")
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "mine\n" {
		t.Errorf("the foreign directory changed: %q, %v", data, err)
	}
}

// TestPatchStatsCountsSuggestionsNotReaddedLines: a last line re-added only for its newline is not
// an edit; a replacement is one; each file header counts once.
func TestPatchStatsCountsSuggestionsNotReaddedLines(t *testing.T) {
	diff := "--- a/main.tex\n+++ b/main.tex\n@@ -1,2 +1,3 @@\n a\n-b\n\\ No newline at end of file\n+b\n+x\n\\ No newline at end of file\n" +
		"--- a/other.tex\n+++ b/other.tex\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	if edits, files := patchStats(diff); edits != 2 || files != 2 {
		t.Errorf("edits %d, files %d", edits, files)
	}
}
