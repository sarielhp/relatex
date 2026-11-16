package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func slidePrefixOf(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deck.tex")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return SlidePrefix(path)
}

// Decision 082: a beamer deck is built in handout mode unless it chooses a static mode.
func TestSlidePrefix(t *testing.T) {
	cases := map[string]string{
		"\\documentclass[slides]{beamer}\n":                          OneUpPrefix + HandoutPrefix,
		"\\documentclass{beamer}\n":                                  OneUpPrefix + HandoutPrefix,
		"\\documentclass[compress,trans,9pt]{beamer}\n":              OneUpPrefix,
		"\\documentclass[ handout ]{beamer}\n":                       OneUpPrefix,
		"\\documentclass{article}\n":                                 "",
		"%\\documentclass[handout]{beamer}\n\\documentclass{beamer}": OneUpPrefix + HandoutPrefix,
		"% \\documentclass{beamer}\n\\documentclass{article}\n":      "",
		"\\documentclass{beamer} % 50\\% {article}\n":                OneUpPrefix + HandoutPrefix,
	}
	for text, want := range cases {
		if got := slidePrefixOf(t, text); got != want {
			t.Errorf("SlidePrefix(%q) = %q, want %q", text, got, want)
		}
	}
	if got := SlidePrefix(filepath.Join(t.TempDir(), "missing.tex")); got != "" {
		t.Errorf("a missing main file has prefix %q", got)
	}
}

// A deck whose \documentclass sits in a file the main file \inputs (slides/0027: day-1.tex inputs
// ../preamble.tex) is a deck; a main file with its own class line is not read through its inputs.
func TestSlidePrefixFollowsAnInputPreamble(t *testing.T) {
	root := t.TempDir()
	slides := filepath.Join(root, "slides")
	if err := os.MkdirAll(slides, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, text string) {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "preamble.tex"), "\\documentclass[aspectratio=43]{beamer}\n")
	write(filepath.Join(slides, "day.tex"), "% \\input{nothing}\n\\input{../preamble.tex}\n\\begin{document}\n")
	if got := SlidePrefix(filepath.Join(slides, "day.tex")); got != OneUpPrefix+HandoutPrefix {
		t.Errorf("a deck through its preamble has prefix %q", got)
	}
	write(filepath.Join(slides, "paper.tex"), "\\documentclass{article}\n\\input{../preamble}\n")
	if got := SlidePrefix(filepath.Join(slides, "paper.tex")); got != "" {
		t.Errorf("a main file with its own class has prefix %q", got)
	}
}
