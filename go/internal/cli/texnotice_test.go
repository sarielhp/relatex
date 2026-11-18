package cli

import (
	"strings"
	"testing"
)

const texFailure = "LuaLaTeX failed on the paper: Undefined control sequence.\n  at a.tex:3: \\foo"

func TestNoticeIsPlainOffATerminal(t *testing.T) {
	t.Setenv("TERM", "xterm")
	got := luaLaTeXNoticeText(false)
	if strings.Contains(got, "\x1b") || !strings.HasSuffix(got, whyLuaLaTeXURL) {
		t.Errorf("plain notice = %q", got)
	}
}

func TestNoticeLinksOnATerminal(t *testing.T) {
	t.Setenv("TERM", "xterm")
	want := "\x1b]8;;" + whyLuaLaTeXURL + "\x1b\\" + whyLuaLaTeXURL + "\x1b]8;;\x1b\\"
	if got := luaLaTeXNoticeText(true); !strings.HasSuffix(got, want) {
		t.Errorf("terminal notice = %q", got)
	}
}

func TestNoticeIsPlainOnADumbTerminal(t *testing.T) {
	t.Setenv("TERM", "dumb")
	if got := luaLaTeXNoticeText(true); strings.Contains(got, "\x1b") {
		t.Errorf("dumb terminal notice = %q", got)
	}
}

func TestNoticeComesFirstOnATeXBuildFailure(t *testing.T) {
	t.Setenv("TERM", "xterm")
	previous := conversionHuman
	conversionHuman = nil
	defer func() { conversionHuman = previous }()
	got := failureOutput(texFailure, false)
	if !strings.HasPrefix(got, "This program requires a paper that compiles correctly on LuaLaTeX") {
		t.Errorf("no notice first: %q", got)
	}
	if !strings.Contains(got, "\n\nrelatex: error: LuaLaTeX failed on the paper") {
		t.Errorf("details missing after the notice: %q", got)
	}
}

func TestNoNoticeForOtherFailures(t *testing.T) {
	previous := conversionHuman
	conversionHuman = nil
	defer func() { conversionHuman = previous }()
	for _, message := range []string{"No such file: /tmp/a.zip", "Workspace path escapes root"} {
		if got := failureOutput(message, false); strings.Contains(got, "requires a paper") {
			t.Errorf("notice on %q: %q", message, got)
		}
	}
}
