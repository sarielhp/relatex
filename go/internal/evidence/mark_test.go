package evidence

import "testing"

// A note's printed mark is text: TeX markup around a symbol mark reads as the symbol.
func TestPrintedMarkResolvesTeX(t *testing.T) {
	cases := map[string]string{
		`\protect \mbox {{$\protect \mbox {${^{\star }}$}\relax $}}`:          "⋆",
		`\TextOrMath {\textasteriskcentered }{*}`:                             "∗",
		`\TextOrMath {\textdagger }{\dagger }`:                                "†",
		`\protect \textsuperscript {\TextOrMath {\textasteriskcentered }{*}}`: "∗",
		`$\ast $`:                                  "∗",
		`\hbox {$\star $}\relax `:                  "⋆",
		`\star \relax `:                            "⋆",
		`\dagger `:                                 "†",
		`\protect \mbox {{$\dagger \relax $}}`:     "†",
		`\protect \ensuremath {\star }`:            "⋆",
		`\protect \ensuremath {{\star }{\star } }`: "⋆⋆",
		`\protect \ensuremath {*}`:                 "*",
		`{5}`:                                      "5",
	}
	for number, want := range cases {
		if got := PrintedMark(number); got != want {
			t.Errorf("PrintedMark(%q) = %q, want %q", number, got, want)
		}
	}
}

// A mark left as code that prints no text (an amsart title note's \@thefnmark) is empty.
func TestPrintedMarkUnprintedCode(t *testing.T) {
	for _, number := range []string{`\@thefnmark `, `\thefootnote `, `\hskip -12pt`, `\relax `, `\protect \ensuremath { }`, `\foo {1}`} {
		if got := PrintedMark(number); got != "" {
			t.Errorf("PrintedMark(%q) = %q, want empty", number, got)
		}
	}
}

// Plain numbers and marks stay as recorded.
func TestPrintedMarkPlainUnchanged(t *testing.T) {
	for _, number := range []string{"", "1", "12.1", "a", "*", "†", "1 ", "(iv)"} {
		if got := PrintedMark(number); got != number {
			t.Errorf("PrintedMark(%q) = %q, want unchanged", number, got)
		}
	}
}
