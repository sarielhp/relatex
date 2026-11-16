package cli

import (
	"strings"
	"testing"
)

// The expectations are TerminalReportTest's (test/terminal_report_test.rb): only the level
// word is red, and a file name too long to read inside a sentence gets its own line.

func TestFailurePaintsOnlyTheLevelWord(t *testing.T) {
	got := failureText("Failed\n  at paper.tex:3: x\nEvidence: short", true)
	want := "relatex: \x1b[31merror\x1b[0m: Failed\n\x1b[1m  at paper.tex:3: x\x1b[0m\n\x1b[2mEvidence: short\x1b[0m"
	if got != want {
		t.Errorf("coloured failure = %q, want %q", got, want)
	}
	if plain := failureText("Failed", false); plain != "relatex: error: Failed" {
		t.Errorf("plain failure = %q", plain)
	}
}

func TestALongFileNameIsReadOnItsOwnLine(t *testing.T) {
	name := "/tmp/" + strings.Repeat("a", 50) + ".zip"
	got := failureText("No such file or directory - "+name, false)
	want := "relatex: error: No such file or directory -\n  " + name
	if got != want {
		t.Errorf("long name = %q, want %q", got, want)
	}
}

func TestAShortFileNameStaysInTheSentence(t *testing.T) {
	got := failureText("No such file: /tmp/a.zip", false)
	if want := "relatex: error: No such file: /tmp/a.zip"; got != want {
		t.Errorf("short name = %q, want %q", got, want)
	}
}

// books/0003: a URL in a browser check's message is no file name; it stays whole in its sentence.
func TestAURLIsNotBrokenAsAFileName(t *testing.T) {
	message := `the page shows {"href"=>"http://cs.uiuc.edu/~sariel/papers/03/min_disk/a-long-tail"}`
	if got, want := failureText(message, false), "relatex: error: "+message; got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
}

func TestAHomeShortenedLinkIsNotBroken(t *testing.T) {
	got := failureText("Evidence: ~/.cache/relatex/output/a-paper-with-a-long-name/err/abcdef", false)
	if want := "relatex: error: Evidence: ~/.cache/relatex/output/a-paper-with-a-long-name/err/abcdef"; got != want {
		t.Errorf("link = %q, want %q", got, want)
	}
}
