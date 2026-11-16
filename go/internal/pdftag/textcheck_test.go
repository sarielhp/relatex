package pdftag

import "testing"

// line paints text on one baseline: each glyph at x with its advance; a space advances x only.
func line(text string, x, advance float64) []paintedGlyph {
	var glyphs []paintedGlyph
	for _, r := range text {
		if r != ' ' {
			glyphs = append(glyphs, paintedGlyph{text: string(r), baseline: [2]float64{x, 100}, advance: advance})
		}
		x += advance
	}
	return glyphs
}

// read reads every glyph of a page as one marked content.
func read(glyphs []paintedGlyph) readText {
	indices := make([]int, len(glyphs))
	for index := range indices {
		indices[index] = index
	}
	return glyphText(glyphs, newGlyphRoles(glyphs), indices)
}

func expect(pieces ...piece) expectedText {
	for index := range pieces {
		pieces[index].leaf, pieces[index].kind = "p", pieceText
	}
	return leafExpectations(pieces)["p"]
}

// contentsLine paints "1.4 Disk", leader dots every 3 advances with a blank of 2 advances
// around each, and the page number.
func contentsLine(dots string) []paintedGlyph {
	glyphs := line("1.4 Disk", 0, 1)
	x := 10.0
	for range dots {
		glyphs = append(glyphs, paintedGlyph{text: ".", baseline: [2]float64{x, 100}, advance: 1})
		x += 3
	}
	return append(glyphs, line("11", x+1, 1)...)
}

func TestLeaderDotsAreDroppedOnlyAsLeaders(t *testing.T) {
	want := expect(piece{text: "1.4 Disk 11"})
	for _, dots := range []string{".", "...."} {
		if kind := matchKind(read(contentsLine(dots)), want, false); kind != "leaders" {
			t.Fatalf("%d leader dots: matched as %q", len(dots), kind)
		}
	}
	// A full stop touches its word: not a leader, so the extra dot still fails.
	if kind := matchKind(read(line("1.4 Disk. 11", 0, 1)), want, false); kind != "" {
		t.Fatalf("a full stop the model lacks matched as %q", kind)
	}
	// Leaders forgive no missing word.
	if kind := matchKind(read(contentsLine("...")), expect(piece{text: "1.4 Enclosing Disk 11"}), false); kind != "" {
		t.Fatalf("a missing word matched as %q", kind)
	}
	// An ellipsis's dots are 0.6 of a dot apart: they stay text.
	ellipsis := line("so", 0, 1)
	for index := range 3 {
		ellipsis = append(ellipsis, paintedGlyph{text: ".", baseline: [2]float64{2 + 1.6*float64(index), 100}, advance: 1})
	}
	if kind := matchKind(read(ellipsis), expect(piece{text: "so"}), false); kind != "" {
		t.Fatalf("an ellipsis the model lacks matched as %q", kind)
	}
}

// accented paints "Milenkovi", then an accent over the next letter, then "c,".
func accented(accent string, shift float64) []paintedGlyph {
	glyphs := line("Milenkovi", 0, 1)
	glyphs = append(glyphs, paintedGlyph{text: accent, baseline: [2]float64{9 + shift, 100}, advance: 1})
	return append(glyphs, line("c,", 9, 1)...)
}

func TestTeXAccentFollowsItsBase(t *testing.T) {
	if kind := matchKind(read(accented("´", 0.1)), expect(piece{text: "Milenković,"}), false); kind != "accents" {
		t.Fatalf("an acute over c matched as %q", kind)
	}
	if kind := matchKind(read(accented("ˇ", 0)), expect(piece{text: "Milenkovič,"}), false); kind != "accents" {
		t.Fatalf("a caron over c matched as %q", kind)
	}
	// The accent is not the model's: still a mismatch.
	if kind := matchKind(read(accented("ˇ", 0)), expect(piece{text: "Milenković,"}), false); kind != "" {
		t.Fatalf("a caron read for an acute matched as %q", kind)
	}
	// A circumflex beside the next glyph, not over it, is text (x^c), not an accent.
	beside := append(line("x^", 0, 1), line("c", 2, 1)...)
	if kind := matchKind(read(beside), expect(piece{text: "xĉ"}), false); kind != "" {
		t.Fatalf("a circumflex beside its neighbour matched as %q", kind)
	}
	if kind := matchKind(read(beside), expect(piece{text: "x^c"}), false); kind != "exact" {
		t.Fatalf("x^c matched as %q", kind)
	}
}

func TestDotlessUnderAnAccentReadsAsItsLetter(t *testing.T) {
	// \'{\i}: the font paints the acute, then the dotless i over it.
	dotlessLine := append(line("Milenkov", 0, 1),
		paintedGlyph{text: "´", baseline: [2]float64{8.1, 100}, advance: 1},
		paintedGlyph{text: "ı", baseline: [2]float64{8.2, 100}, advance: 1})
	dotlessLine = append(dotlessLine, line(",", 9.2, 1)...)
	if kind := matchKind(read(dotlessLine), expect(piece{text: "Milenkoví,"}), false); kind != "accents" {
		t.Fatalf("an acute over the dotless i matched as %q", kind)
	}
	// \"{\j}: the diaeresis, then the dotless j.
	base := append(line("a", 0, 1),
		paintedGlyph{text: "¨", baseline: [2]float64{1.1, 100}, advance: 1},
		paintedGlyph{text: "ȷ", baseline: [2]float64{1.2, 100}, advance: 1})
	if kind := matchKind(read(base), expect(piece{text: "aj̈"}), false); kind != "accents" {
		t.Fatalf("a diaeresis over the dotless j matched as %q", kind)
	}
	// Without an accent the dotless letter is no i.
	if kind := matchKind(read(line("Fısica", 0, 1)), expect(piece{text: "Física"}), false); kind != "" {
		t.Fatalf("a lone dotless i matched the i as %q", kind)
	}
	// The plain form is still the glyphs' own: a lone accent before the dotless i is no accent
	// over it (the model spells the accent, then the letter).
	if kind := matchKind(read(line("´ı", 0, 1)), expect(piece{text: "í"}), false); kind != "" {
		t.Fatalf("an acute before a dotless i matched as %q", kind)
	}
	// An accent beside its base, not over it, does not pair with the dotless letter either.
	beside := append(line("Milenkov´", 0, 1), line("ı", 9, 1)...)
	if kind := matchKind(read(beside), expect(piece{text: "Milenkoví"}), false); kind != "" {
		t.Fatalf("an acute beside a dotless i matched as %q", kind)
	}
}

// A full stop the source spaced from its word ("faint fluxes . We", papers 0069) satisfies the
// leader geometry; when the model spells the dot, the reading form that keeps it matches.
func TestSpacedFullStopIsKeptWhenTheModelSpellsIt(t *testing.T) {
	glyphs := []paintedGlyph{
		{text: "K", baseline: [2]float64{0, 100}, advance: 1},
		{text: "¨", baseline: [2]float64{1.1, 100}, advance: 1},
		{text: "a", baseline: [2]float64{1.2, 100}, advance: 1},
		{text: "x", baseline: [2]float64{3, 100}, advance: 1},
		{text: ".", baseline: [2]float64{5, 100}, advance: 1},
		{text: "O", baseline: [2]float64{7, 100}, advance: 1},
		{text: "n", baseline: [2]float64{8, 100}, advance: 1},
		{text: "e", baseline: [2]float64{9, 100}, advance: 1},
	}
	if kind := matchKind(read(glyphs), expect(piece{text: "Käx.One"}), false); kind != "accents" {
		t.Fatalf("a spaced full stop matched as %q", kind)
	}
	// A model without the dot still reads the spaced dot as a leader.
	if kind := matchKind(read(glyphs), expect(piece{text: "KäxOne"}), false); kind != "leaders" {
		t.Fatalf("a spaced dot the model lacks matched as %q", kind)
	}
}

func TestCaseIsFreeOnlyInSmallCapitals(t *testing.T) {
	want := expect(piece{text: "Maple", caps: true}, piece{text: " code"})
	if kind := matchKind(read(line("MAPLE code", 0, 1)), want, false); kind != "small_caps" {
		t.Fatalf("small capitals matched as %q", kind)
	}
	// Outside the small capitals case counts.
	if kind := matchKind(read(line("MAPLE CODE", 0, 1)), want, false); kind != "" {
		t.Fatalf("capitals outside small caps matched as %q", kind)
	}
	// Small capitals forgive case, not letters.
	if kind := matchKind(read(line("MAPEL code", 0, 1)), want, false); kind != "" {
		t.Fatalf("a misspelling in small caps matched as %q", kind)
	}
}

func TestCompatibilityCharactersCompareByNFKC(t *testing.T) {
	if kind := matchKind(read(line("step ①", 0, 1)), expect(piece{text: "step 1"}), false); kind != "exact" {
		t.Fatalf("a circled one matched as %q", kind)
	}
	if kind := matchKind(read(line("step ②", 0, 1)), expect(piece{text: "step 1"}), false); kind != "" {
		t.Fatalf("a circled two matched a one as %q", kind)
	}
}

// cmsy10's \bigcirc is U+20DD in the text layer, alone or after the digit it encircles; the model
// spells the large circle U+25EF.
func TestEnclosingCircleReadsAsLargeCircle(t *testing.T) {
	for _, painted := range []string{"1⃝", "⃝", "Fig. 2.1⃝ x"} {
		model := expect(piece{text: replaceCircle(painted)})
		if got := matchKind(read(line(painted, 0, 1)), model, false); got != "exact" {
			t.Errorf("%q: matchKind = %q, want exact", painted, got)
		}
	}
}

func replaceCircle(text string) string {
	out := []rune(text)
	for index, r := range out {
		if r == 0x20dd {
			out[index] = 0x25ef
		}
	}
	return string(out)
}
