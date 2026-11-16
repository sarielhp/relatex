package fspath

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var unixStyle = style{separator: '/'}

// The Unix rules are the byte comparison the callers made before (strings.HasPrefix(path,
// root+"/"), strings.TrimPrefix).
func TestUnixRulesAreThePlainPrefix(t *testing.T) {
	cases := []struct {
		root, path, name string
		inside           bool
	}{
		{"/a/b", "/a/b/c/d.tex", "c/d.tex", true},
		{"/a/b", "/a/bc/d.tex", "/a/bc/d.tex", false},
		{"/a/b", "/a/b", "/a/b", false},
		{"/a/b", "/A/b/c", "/A/b/c", false},
		{"/", "/x/y", "x/y", true},
	}
	for _, c := range cases {
		if got := unixStyle.inside(c.root, c.path); got != c.inside {
			t.Errorf("inside(%q, %q) = %v", c.root, c.path, got)
		}
		if got := unixStyle.name(c.root, c.path); got != c.name {
			t.Errorf("name(%q, %q) = %q, want %q", c.root, c.path, got, c.name)
		}
	}
	if !unixStyle.within("/a/b", "/a/b") || unixStyle.within("/a/b", "/a/B") {
		t.Error("within compares the root itself byte for byte")
	}
}

// Windows compares without case, takes either slash, keeps a volume root's own separator, and
// names come back with "/" for records.
func TestWindowsRulesFoldCaseAndSlashes(t *testing.T) {
	cases := []struct {
		root, path, name string
		inside           bool
	}{
		{`C:\Users\a\paper`, `C:\Users\a\paper\figs\x.pdf`, "figs/x.pdf", true},
		{`C:\Users\a\paper`, `c:/users/A/Paper/figs/x.pdf`, "figs/x.pdf", true},
		{`C:\Users\a\paper`, `C:\Users\a\paper2\x.pdf`, `C:/Users/a/paper2/x.pdf`, false},
		{`C:\`, `C:\texlive\2026`, "texlive/2026", true},
		{`C:/Users/a`, `C:\Users\a\b`, "b", true},
	}
	for _, c := range cases {
		if got := windowsStyle.inside(c.root, c.path); got != c.inside {
			t.Errorf("inside(%q, %q) = %v", c.root, c.path, got)
		}
		if got := windowsStyle.name(c.root, c.path); got != c.name {
			t.Errorf("name(%q, %q) = %q, want %q", c.root, c.path, got, c.name)
		}
	}
	if !windowsStyle.within(`C:\Users\a`, `c:/users/a`) || !windowsStyle.equal(`C:\A\b`, `c:/a/B`) {
		t.Error("the same directory spelled in another case or with slashes is the same place")
	}
	if got := windowsStyle.slash(`C:\texlive\2026\font.otf`); got != "C:/texlive/2026/font.otf" {
		t.Errorf("slash = %q", got)
	}
}

func TestBelowRefusesTheParent(t *testing.T) {
	for relative, below := range map[string]bool{"a/b": true, ".": true, "..": false, "../x": false, "..x": true} {
		if Below(relative) != below {
			t.Errorf("Below(%q) = %v", relative, !below)
		}
	}
}

func TestExpandHomeAndAbbreviate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got := ExpandHome("~/styles"); got != filepath.Join(home, "styles") {
		t.Errorf("ExpandHome(~/styles) = %q", got)
	}
	if got := ExpandHome("~"); got != home {
		t.Errorf("ExpandHome(~) = %q", got)
	}
	if got := ExpandHome("~other/x"); got != "~other/x" {
		t.Errorf("another user's home stays: %q", got)
	}
	if got := Abbreviate(filepath.Join(home, "err", "a.log")); got != "~/err/a.log" {
		t.Errorf("Abbreviate = %q", got)
	}
}

func TestLostRaceAndReplace(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "done")
	if err := os.MkdirAll(filepath.Join(target, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(directory, "mine")
	if err := os.MkdirAll(filepath.Join(mine, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := os.Rename(mine, target)
	if err == nil || !LostRace(err, target) {
		t.Fatalf("renaming a directory onto a finished one: %v, lost race %v", err, LostRace(err, target))
	}
	if LostRace(errors.New("other"), target) {
		t.Error("an unrelated error is no lost race")
	}
	from, to := filepath.Join(directory, "a.tmp"), filepath.Join(directory, "a.pdf")
	for _, path := range []string{from, to} {
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Replace(from, to); err != nil {
		t.Fatalf("Replace over a closed file: %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := Replace(filepath.Join(directory, "missing"), to); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Replace of a missing file: %v", err)
	}
}
