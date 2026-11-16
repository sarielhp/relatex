package a11ylint

import (
	"strings"
	"testing"
)

// TestPatchKeepsCRLFLineEnds: a source written on Windows has CRLF line ends; the lines the patch
// adds have them too, so the patched file keeps one convention (decision 129).
func TestPatchKeepsCRLFLineEnds(t *testing.T) {
	source := "\\documentclass{article}\r\n\\begin{document}\r\nSome text.\r\n\\end{document}\r\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	diff := patched(t, directory, Lint(patchPaper(t, directory), Options{}))
	want := "--- a/main.tex\n+++ b/main.tex\n@@ -1,4 +1,4 @@\n-\\documentclass{article}\r\n" +
		"+\\documentclass[english]{article}\r\n \\begin{document}\r\n Some text.\r\n \\end{document}\r\n"
	if diff != want {
		t.Errorf("diff:\n%q\nwant:\n%q", diff, want)
	}
}

// The annotated copy of a CRLF source inserts its comment lines with CRLF too.
func TestAnnotatedCopyKeepsCRLFLineEnds(t *testing.T) {
	text := withLineEnds("%% relatex-a11y: x\n", "a\r\nb\r\n")
	if text != "%% relatex-a11y: x\r\n" {
		t.Errorf("inserted %q", text)
	}
	if text := withLineEnds("x\n", "a\nb\n"); text != "x\n" {
		t.Errorf("an LF source got %q", text)
	}
	if strings.Contains(withLineEnds("alt={TODO}", "a\r\n"), "\r") {
		t.Error("text without a line end stays as it is")
	}
}
