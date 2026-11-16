package figscan

import (
	"strings"
	"testing"
)

func textNode(text string, x, y float64, size string) *Node {
	return &Node{Kind: KindText, Text: text, Point: &Point{x, y}, Size: size, TextType: "label"}
}

// TestAttachPrintedMapsMacrosToPrintedText: an Ipe label's source uses the author's macro; the
// printed runs at its place (base, smaller lowered subscript, base again) give what a reader sees.
func TestAttachPrintedMapsMacrosToPrintedText(t *testing.T) {
	macro := textNode(`\rgnY1v`, 10, 50, "")
	macro.Math = true
	far := textNode(`\pnt`, 80, 10, "")
	unmapped := textNode(`\odd`, 40, 80, "")
	traced := []*Node{
		textNode("Δ", 10.2, 50, "10pt"), textNode("R", 12, 48.5, "7pt"), textNode("(v)", 13.5, 50, "10pt"),
		textNode("x�", 40.5, 80.2, "10pt"),
	}
	attachPrinted([]*Node{macro, {Kind: KindGroup, Children: []*Node{far, unmapped}}}, traced, 0.7)
	if macro.Printed != "Δ_R(v)" || macro.PrintedNote != "" {
		t.Errorf("macro label: printed %q note %q", macro.Printed, macro.PrintedNote)
	}
	if far.Printed != "" || far.PrintedNote != printedNone {
		t.Errorf("unmatched label: printed %q note %q", far.Printed, far.PrintedNote)
	}
	if unmapped.PrintedNote != unmappedText {
		t.Errorf("unmapped glyph not flagged: %q", unmapped.PrintedNote)
	}
	got := printedText(macro.Text, macro.Math, macro.Printed, macro.PrintedNote)
	if got != `printed "Δ_R(v)" (source "$\rgnY1v$")` {
		t.Errorf("text form %s", got)
	}
	if got = printedText(far.Text, false, far.Printed, far.PrintedNote); got != `source "\pnt"` {
		t.Errorf("unmatched text form %s", got)
	}
	if got = printedText("plain", false, "", ""); got != `"plain"` {
		t.Errorf("trace label text form %s", got)
	}
}

// TestAttachPrintedNearestAnchorWins: two labels close together each get their own run.
func TestAttachPrintedNearestAnchorWins(t *testing.T) {
	a, b := textNode("$a$", 10, 10, ""), textNode("$b$", 14, 10, "")
	attachPrinted([]*Node{a, b}, []*Node{textNode("α", 10.1, 10, "10pt"), textNode("β", 14.1, 10, "10pt")}, 1)
	if a.Printed != "α" || b.Printed != "β" {
		t.Errorf("printed %q and %q", a.Printed, b.Printed)
	}
}

// TestPrintedLabelsOnIpeFigure: a real Ipe figure with \pnt macros prints p_3, and the header states
// the orientation.
func TestPrintedLabelsOnIpeFigure(t *testing.T) {
	text := scanCase(t, "barycentric", 3).Text()
	for _, want := range []string{`printed "p_3" (source "$\pnt^{}_3$")`, "(x right, y up; 0-100"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in\n%s", want, text)
		}
	}
}

// TestTraceLabelScripts: a run whose tail is lowered and smaller is one label with "_", a raised
// one "^"; regular text and a same-size glyph off the baseline are left as before.
func TestTraceLabelScripts(t *testing.T) {
	r := &traceReader{scale: 1, extent: [4]float64{-100, -100, 100, 100}}
	at := func(text string, x, y, size float64) glyph {
		return glyph{text: text, at: Point{x, y}, end: Point{x + 0.5*size, y}, dir: Point{1, 0}, size: size, color: "black"}
	}
	first := r.glyph(at("x", 0, 0, 10))
	for _, g := range []glyph{at("1", 5, -2, 7), at("=", 11, 0, 10), at("y", 18, 0, 10), at("2", 23, 3.5, 7)} {
		r.glyph(g)
	}
	if first.Text != "x_1 = y^2" {
		t.Errorf("scripted run %q", first.Text)
	}
	plain := r.glyph(at("a", 0, 50, 10))
	r.glyph(at("b", 5, 50, 10))
	shifted := r.glyph(at("c", 10, 47, 10))
	if plain.Text != "ab" || shifted == nil || shifted.Text != "c" {
		t.Errorf("regular text %q, shifted glyph %v", plain.Text, shifted)
	}
}

// TestSmallCapsFont: small-caps font names by their marks, regular fonts not.
func TestSmallCapsFont(t *testing.T) {
	for name, want := range map[string]bool{
		"QZSVYU+rtxsc": true, "YYRVMG+rtxbsc": true, "ABCDEF+CMCSC10": true, "NimbusRomNo9L-ReguSC": true,
		"LMRoman10-CapsRegular": true, "SourceSerif-smcp": true,
		"SOTZTZ+rtxr": false, "WPOQGB+NimbusRomNo9L-ReguItal": false, "CMR10": false, "Helvetica": false, "Script": false,
	} {
		if got := smallCapsFont(name); got != want {
			t.Errorf("smallCapsFont(%q) = %v", name, got)
		}
	}
}

// TestSmallCapsLabels: books/0003 confused_2 prints its vertex classes and region labels in small
// caps (txfonts rtxsc); the trace carries lowercase Unicode, the printed text is uppercase.
func TestSmallCapsLabels(t *testing.T) {
	text := scanCase(t, "confused_2", 1).Text()
	for _, want := range []string{
		`printed "∆R(v)" (case "small caps", traced "∆r(v)"; source "$\rgnY{1}{v}$")`,
		`printed "R" (case "small caps", traced "r"; source "$\vX{1}$")`,
		`printed "G" (case "small caps", traced "g"; source "$\vX{2}$")`,
		`printed "B" (case "small caps", traced "b"; source "$\vX{3}$")`,
		`printed "v" (source "$v$")`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in\n%s", want, text)
		}
	}
}
