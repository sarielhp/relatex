package pdftag

import "testing"

// accentedAndFormula paints "K" with a diaeresis over "a" ("Kä"), then "hler and", a subscript
// "X" (a formula's last glyph, off the baseline) painted right after the word, a full stop on
// the baseline, a space and "One". The dot's paint-order predecessor is the off-baseline
// subscript, so it has no neighbour on its own baseline on the left.
func accentedAndFormula() []paintedGlyph {
	glyphs := line("K", 0, 1)
	glyphs = append(glyphs, paintedGlyph{text: "¨", baseline: [2]float64{0.9, 100}, advance: 1})
	glyphs = append(glyphs, line("a", 1, 1)...)
	glyphs = append(glyphs, line("hler and", 2, 1)...)
	glyphs = append(glyphs, paintedGlyph{text: "X", baseline: [2]float64{10, 97}, advance: 1})
	glyphs = append(glyphs, paintedGlyph{text: ".", baseline: [2]float64{11, 100}, advance: 1})
	return append(glyphs, line("One", 13, 1)...)
}

func TestFormulaDotIsNotALeader(t *testing.T) {
	// The reading form keeps the sentence's full stop after the formula.
	want := expect(piece{text: "Kähler and X. One"})
	if kind := matchKind(read(accentedAndFormula()), want, false); kind != "accents" {
		t.Fatalf("a full stop after a formula matched as %q", kind)
	}
	// A dot whose right neighbour is missing (end of line) is not a leader either.
	if kind := matchKind(read(line("and .", 0, 1)), expect(piece{text: "and"}), false); kind != "" {
		t.Fatalf("a dot at the end of the line was forgiven as %q", kind)
	}
}
