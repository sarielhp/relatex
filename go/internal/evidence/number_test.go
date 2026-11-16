package evidence

import "testing"

// IEEEtran's \thesubsection is \mbox{III-A}; a paragraph counter appends its own letter.
func TestLiteralNumberBoxedWithTail(t *testing.T) {
	for number, want := range map[string]string{`\mbox  {III-A}`: "III-A", `\mbox  {III-A}0a`: "III-A0a"} {
		got := LiteralNumber(number)
		if got == nil || *got != want {
			t.Errorf("LiteralNumber(%q) = %v, want %q", number, got, want)
		}
	}
}

// A plain group inside a number prints nothing (a theorem counter written 44.1.{5}).
func TestLiteralNumberPlainGroup(t *testing.T) {
	if got := LiteralNumber("44.1.{5}"); got == nil || *got != "44.1.5" {
		t.Errorf("LiteralNumber(44.1.{5}) = %v", got)
	}
	if got := LiteralNumber("44.1.{5"); got != nil {
		t.Errorf("unbalanced group = %v", *got)
	}
}

// enumitem records a custom item label inside nested groups ({{(i)}}, {{1:}}): they print nothing.
func TestLiteralNumberNestedGroups(t *testing.T) {
	for number, want := range map[string]string{"{{(i)}}": "(i)", "{{1:}}": "1:", "{(A)}": "(A)"} {
		if got := LiteralNumber(number); got == nil || *got != want {
			t.Errorf("LiteralNumber(%q) = %v, want %q", number, got, want)
		}
	}
	for _, number := range []string{"{{(i)}", "{(i)}}", "{{a}b}"} {
		if got := LiteralNumber(number); got != nil {
			t.Errorf("LiteralNumber(%q) = %q", number, *got)
		}
	}
}

// A biblatex alphabetic label may hold one space before its year part (G E97); prose may not.
func TestBibliographyLabel(t *testing.T) {
	for _, label := range []string{"1", "ADM25", "BLT+25", "G E97", "2012b"} {
		if !bibliographyLabel.MatchString(label) {
			t.Errorf("label %q rejected", label)
		}
	}
	for _, label := range []string{"0", "a b", "[1]", "2012bc", "12B"} {
		if bibliographyLabel.MatchString(label) {
			t.Errorf("label %q accepted", label)
		}
	}
}

// A number typeset as math prints its glyphs: scripts flatten, groups drop; a macro or a number
// with no math run is not one.
func TestMathNumber(t *testing.T) {
	for number, want := range map[string]string{"{$A$}": "A", "{NLS$_a$}": "NLSa", "{($R_1$)}": "(R1)", "$x^{2}$": "x2",
		`$\bm  {\mathcal  {Q}}_1$`: "Q1", `{$\ddagger $}`: "‡", `{$\ddagger \ddagger $}`: "‡‡", `{$\dagger $}`: "†"} {
		if got := MathNumber(number); got == nil || *got != want {
			t.Errorf("MathNumber(%q) = %v, want %q", number, got, want)
		}
	}
	for _, number := range []string{"1.2", `{$\alpha$}`, "{$A}", "", `{$\mathcal{\alpha}$}`, `{$\dagger$}x$`} {
		if got := MathNumber(number); got != nil {
			t.Errorf("MathNumber(%q) = %q, want nil", number, *got)
		}
	}
}

// A colour prints no glyph (papers3/0552's \theequation is {\color{blue}\arabic{equation}}).
func TestLiteralNumberColoured(t *testing.T) {
	for number, want := range map[string]string{`{\color  {blue}8}`: "8", `\textcolor{red}{4.2}`: "4.2"} {
		if got := LiteralNumber(number); got == nil || *got != want {
			t.Errorf("LiteralNumber(%q) = %v, want %q", number, got, want)
		}
	}
}
