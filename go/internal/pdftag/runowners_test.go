package pdftag

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// A box with a literal (a Beamer block's background) flags all its glyphs as of unknown position,
// though nothing moved them: a glyph the font spells otherwise than TeX (slides 0027's ℕ, painted
// as U+FFFD) is still TeX's glyph where it lies at TeX's position (a11y-102). One elsewhere with
// another text is not.
func TestOwnerMatcherUnknownPositionAtTeXsPosition(t *testing.T) {
	page := glyphowner.Page{Number: 1, X: []float64{10, 20, 30}, Y: []float64{40, 40, 40}, Text: []string{"t", "ℕ", "("},
		Leaf: []int{1, 2, 1}}
	painted := []paintedGlyph{{text: "t", baseline: [2]float64{10, 40}}, {text: "�", baseline: [2]float64{20.2, 40}},
		{text: "(", baseline: [2]float64{30, 40}}}
	unknown := map[int]bool{0: true, 1: true, 2: true}
	pairs := (&ownerMatcher{page: page, painted: painted, unknown: unknown}).match()
	if len(pairs) != 3 || pairs[1] != 1 {
		t.Fatalf("pairs %v", pairs)
	}
	moved := &ownerMatcher{page: page, painted: []paintedGlyph{{text: "�", baseline: [2]float64{80, 90}}}, unknown: unknown}
	if moved.matches(1, 0) {
		t.Fatal("a glyph away from TeX's position with another text matched")
	}
}

// A drawn picture's box takes the glyphs painted in it, but not one TeX set outside every
// literal-drawn box that the owners give another leaf than the picture's host: papers 0097's caption,
// pulled up into its TikZ picture's box by \vspace{-20pt}, read back as part of the Figure (a11y-106).
func TestDrawnPictureLeavesTheGlyphsOwnedElsewhere(t *testing.T) {
	owners := &glyphowner.Owners{Pictures: []glyphowner.Picture{{Node: "fig", Page: 1, Box: [4]float64{100, 662, 150, 692}}}}
	inside := [2]float64{110, 680}
	tagger := &tagger{owners: owners,
		builder:      &builder{leaves: map[string]*selem{"fig": {role: "Figure", leaf: "fig"}}, hosts: map[string]string{"fig": "host"}},
		painted:      map[int][]paintedGlyph{1: {{text: "a", baseline: inside}, {text: "b", baseline: inside}, {text: "c", baseline: inside}}},
		placedOwners: map[glyphRef]string{{1, 1}: "caption", {1, 2}: "host"}}
	frame := pageFrame{box: [4]float64{0, 0, 612, 792}}
	pictures := tagger.pagePictures(1, frame)
	page := &prepared{number: 1, mapper: &glyphMapper{mapping: []int{0, 1, 2}, frame: frame}}
	for ordinal, want := range []string{"fig", "", "fig"} {
		if got := tagger.glyphPicture(page, pictures, ordinal); got != want {
			t.Errorf("glyph %d: picture %q, want %q", ordinal, got, want)
		}
	}
}

// The owners name a block cut at its tables, not its runs (papers5 1333's panel labels between four
// tables): the glyphs no cell took are shared out to the text runs in TeX's order, each to the run it
// continues; a hyphen continues the current run, and a glyph that continues none stays open.
func TestShareRunsInTeXOrder(t *testing.T) {
	tagger := &tagger{builder: &builder{textRuns: map[string][]string{"b": {"b#r0", "b#r2"}}},
		leafText: map[string]expectedText{"b#r0": {text: "(a)Ov"}, "b#r2": {text: "(b)Pa"}}}
	texts := []string{"(", "b", ")", "P", "a", "(", "a", ")", "O", "v", "-", "Z", "1"}
	painted := make([]paintedGlyph, len(texts))
	pairs := map[int]int{}
	blocks := make([]string, len(texts))
	for index, text := range texts {
		painted[index] = paintedGlyph{text: text}
		blocks[index] = "b"
		// The second label is painted first; TeX shipped it second.
		pairs[index] = (index + 6) % 11
	}
	pairs[11], pairs[12] = 11, 12
	blocks[12] = ""
	leaves, decided := make([]string, len(texts)), make([]bool, len(texts))
	leaves[12], decided[12] = "cell", true
	tagger.shareRuns(map[string]*runCursor{}, painted, pairs, blocks, leaves, decided)
	want := []string{"b#r2", "b#r2", "b#r2", "b#r2", "b#r2", "b#r0", "b#r0", "b#r0", "b#r0", "b#r0", "b#r0", "", "cell"}
	if !slices.Equal(leaves, want) {
		t.Fatalf("leaves %v, want %v", leaves, want)
	}
	if decided[11] {
		t.Fatal("a glyph that continues no run was decided")
	}
}
