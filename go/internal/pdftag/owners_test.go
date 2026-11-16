package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// The layout paints a form's glyphs TeX never shipped ("XY" between "ab" and "cd"), and TeX ships a
// glyph the layout lacks ("e"); the matching skips both and pairs the rest by position.
func TestOwnerMatcherSkipsUnsharedGlyphs(t *testing.T) {
	page := glyphowner.Page{Number: 1, X: []float64{10, 20, 30, 40, 50, 60}, Y: []float64{100, 100, 100, 100, 100, 100},
		Text: []string{"a", "b", "c", "d", "e", "f"}, Leaf: []int{1, 1, 1, 1, 1, 1}}
	painted := []paintedGlyph{
		{text: "a", baseline: [2]float64{10, 100}}, {text: "b", baseline: [2]float64{20.2, 100}},
		{text: "X", baseline: [2]float64{300, 50}}, {text: "Y", baseline: [2]float64{310, 50}},
		{text: "c", baseline: [2]float64{30, 100}}, {text: "d", baseline: [2]float64{40, 100}},
		{text: "f", baseline: [2]float64{60, 100}},
	}
	pairs := (&ownerMatcher{page: page, painted: painted, unknown: map[int]bool{}}).match()
	want := map[int]int{0: 0, 1: 1, 4: 2, 5: 3, 6: 5}
	if len(pairs) != len(want) {
		t.Fatalf("pairs %v, want %v", pairs, want)
	}
	for paint, tex := range want {
		if pairs[paint] != tex {
			t.Errorf("paint %d paired with %d, want %d", paint, pairs[paint], tex)
		}
	}
}

// An included figure paints more glyphs of its own than the matching's window (papers5 1438: a
// float's plots before its caption): the matching resumes after it at the caption's first glyph by its
// position, and no TeX glyph is lost to the figure (decision 121).
func TestOwnerMatcherSkipsAFigureLongerThanTheWindow(t *testing.T) {
	page := glyphowner.Page{Number: 1, X: []float64{10, 20, 30, 40}, Y: []float64{500, 500, 500, 500},
		Text: []string{"F", "I", "G", "."}, Leaf: []int{1, 1, 1, 1}}
	var painted []paintedGlyph
	for k := range paintSkip + 200 {
		painted = append(painted, paintedGlyph{text: "0", baseline: [2]float64{float64(100 + k%300), float64(100 + k/300)}})
	}
	for index, x := range page.X {
		painted = append(painted, paintedGlyph{text: page.Text[index], baseline: [2]float64{x + 0.2, 500}})
	}
	pairs := (&ownerMatcher{page: page, painted: painted, unknown: map[int]bool{}}).match()
	if len(pairs) != len(page.X) {
		t.Fatalf("%d of %d TeX glyphs matched: %v", len(pairs), len(page.X), pairs)
	}
	for index := range page.X {
		if pairs[paintSkip+200+index] != index {
			t.Errorf("TeX glyph %d not paired with its painted glyph: %v", index, pairs)
		}
	}
}

// A glyph whose position TeX does not know (inside a literal-drawn picture) is matched by its text.
func TestOwnerMatcherUnknownPositionByText(t *testing.T) {
	page := glyphowner.Page{Number: 1, X: []float64{0, 0}, Y: []float64{0, 0}, Text: []string{"ff", "x"}, Leaf: []int{1, 1}}
	painted := []paintedGlyph{{text: "ﬀ", baseline: [2]float64{12, 40}}, {text: "x", baseline: [2]float64{20, 40}}}
	pairs := (&ownerMatcher{page: page, painted: painted, unknown: map[int]bool{0: true, 1: true}}).match()
	if len(pairs) != 2 || pairs[0] != 0 || pairs[1] != 1 {
		t.Fatalf("pairs %v", pairs)
	}
}

// TeX ships "ffi" as one glyph a virtual font paints as "f" and "fi": the unmatched "fi" between two
// glyphs of one paragraph is the paragraph's; a gap between two leaves stays open.
func TestFillGaps(t *testing.T) {
	leaves := []string{"p", "", "p", "", "q", "", "", "", "", "", "q"}
	decided := []bool{true, false, true, false, true, false, false, false, false, false, true}
	if filled := fillGaps(leaves, decided); filled != 1 || leaves[1] != "p" || leaves[3] != "" || leaves[5] != "" {
		t.Fatalf("filled %d: %v", filled, leaves)
	}
}

// An "ff" TeX shipped as one glyph that the PDF painted as two, the second lying inside a drawn
// picture's box: the owners completed the ligature, so the second f stays the caption's and the
// picture does not take it (papers6 1124).
func TestDrawnPictureLeavesLigatureCompletedGlyphs(t *testing.T) {
	owners := &glyphowner.Owners{Pictures: []glyphowner.Picture{{Node: "fig", Page: 1, Box: [4]float64{100, 662, 150, 692}}}}
	tagger := &tagger{owners: owners,
		builder:      &builder{leaves: map[string]*selem{"fig": {role: "Figure", leaf: "fig"}}, hosts: map[string]string{"fig": "host"}},
		painted:      map[int][]paintedGlyph{1: {{text: "f", baseline: [2]float64{110, 680}}, {text: "f", baseline: [2]float64{112, 680}}}},
		placedOwners: map[glyphRef]string{{1, 0}: "caption"}} // the origin, which the owners matched
	page := glyphowner.Page{Number: 1, X: []float64{110}, Y: []float64{680}, Text: []string{"ff"}}
	matcher := &ownerMatcher{page: page, painted: tagger.painted[1], unknown: map[int]bool{}}
	leaves, decided := []string{"caption", ""}, []bool{true, false}
	filled := completeLigatures(matcher, map[int]int{0: 0}, leaves, decided)
	if len(filled) != 1 || filled[0] != 1 || leaves[1] != "caption" || !decided[1] {
		t.Fatalf("filled %v: %v %v", filled, leaves, decided)
	}
	frame := pageFrame{box: [4]float64{0, 0, 612, 792}}
	prepared := &prepared{number: 1, mapper: &glyphMapper{mapping: []int{0, 1}, frame: frame}}
	pictures := tagger.pagePictures(1, frame)
	if got := tagger.glyphPicture(prepared, pictures, 1); got != "fig" {
		t.Fatalf("the completed glyph before it is recorded: picture %q, want fig", got)
	}
	tagger.placeLigatures(1, leaves, filled)
	for ordinal := range tagger.painted[1] {
		if got := tagger.glyphPicture(prepared, pictures, ordinal); got != "" {
			t.Errorf("glyph %d: picture %q, want none", ordinal, got)
		}
	}
}

// A glyph the ordinary PDF's font did not map reads U+FFFD through the layout; once the tagging
// spelled the font's codes it stands for the model's character, and only then.
func TestSameTextAcceptsSpelledReplacements(t *testing.T) {
	if !sameText("�nonce", "˷nonce", true) || sameText("�nonce", "˷nonce", false) || sameText("�nonc", "˷nonce", true) {
		t.Fatal("replacement characters compare wrongly")
	}
}

func TestOwnerCheckCompare(t *testing.T) {
	check := OwnerCheck{}
	check.compare(glyphRef{1, 0}, "p1", "p1", true)
	check.compare(glyphRef{1, 1}, "p1", "p2", true)
	check.compare(glyphRef{1, 2}, "p1", "", true)
	check.compare(glyphRef{1, 3}, "", "p2", true)
	check.compare(glyphRef{1, 4}, "p2", "p2", false)
	if check.Agree != 1 || check.Disagree != 1 || check.OwnersOnly != 1 || check.AlignmentOnly != 1 || len(check.Samples) != 1 {
		t.Fatalf("check %+v", check)
	}
}

func TestPageSpansKeepTheAlignmentNearTheOwnedGlyphs(t *testing.T) {
	spans := pageSpans{}
	spans.add("p", 6)
	spans.add("p", 7)
	for page, want := range map[int]bool{4: false, 5: true, 6: true, 8: true, 9: false, 15: false} {
		if got := spans.near("p", page); got != want {
			t.Errorf("near(p, %d) = %v, want %v", page, got, want)
		}
	}
	if !spans.near("alignment-only", 11) {
		t.Error("a leaf the owners gave no glyph is the alignment's alone")
	}
}

// An "ff" ending a cell's word: TeX's one glyph matches the first painted f, the second f lies
// between two cells and is the first cell's; a glyph that is not the rest of the ligature stays open.
func TestCompleteLigatures(t *testing.T) {
	page := glyphowner.Page{Number: 1, X: []float64{0, 20, 40}, Y: []float64{9, 9, 9}, Text: []string{"i", "ff", "7"}}
	painted := []paintedGlyph{
		{text: "i", baseline: [2]float64{0, 9}}, {text: "f", baseline: [2]float64{20, 9}},
		{text: "f", baseline: [2]float64{25, 9}}, {text: "7", baseline: [2]float64{40, 9}},
		{text: "x", baseline: [2]float64{50, 9}},
	}
	matcher := &ownerMatcher{page: page, painted: painted, unknown: map[int]bool{}}
	pairs := map[int]int{0: 0, 1: 1, 3: 2}
	leaves := []string{"a", "a", "", "b", ""}
	decided := []bool{true, true, false, true, false}
	if filled := completeLigatures(matcher, pairs, leaves, decided); len(filled) != 1 || filled[0] != 2 || leaves[2] != "a" || !decided[2] || leaves[4] != "" || decided[4] {
		t.Fatalf("filled %v: %v %v", filled, leaves, decided)
	}
}

// A cell whose glyphs the table geometry bound in full takes no glyph from the text alignment: the
// alignment found "5min" in the tail of "27.5 min" as well (papers3 0829).
func TestDropSurplus(t *testing.T) {
	tagger := &tagger{leafText: map[string]expectedText{"full": {text: "5min"}, "short": {text: "5min"}}}
	bound := map[glyphRef]string{{8, 1}: "full", {8, 2}: "short", {8, 3}: "none"}
	fallback := []glyphRef{{8, 1}, {8, 2}, {8, 3}}
	dropped := tagger.dropSurplus(bound, fallback, map[string]int{"full": 4, "short": 2})
	if _, ok := bound[glyphRef{8, 1}]; dropped != 1 || ok || len(bound) != 2 {
		t.Fatalf("dropped %d: %v", dropped, bound)
	}
}
