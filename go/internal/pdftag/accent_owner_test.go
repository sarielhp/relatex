package pdftag

import (
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// TeX's \accent paints the accent, then its base over it, while the owners give the composed letter
// alone at the accent's origin. The painted base must take the accent's owner too, or it is left
// unbound and set as an artifact, and the accent cannot be composed (papers 0069, "Smolčić").
func TestAccentBaseTakesTheAccentsOwner(t *testing.T) {
	page := glyphowner.Page{Number: 1, X: []float64{10, 20}, Y: []float64{100, 100},
		Text: []string{"č", "x"}, Leaf: []int{1, 2}}
	painted := []paintedGlyph{
		{text: "ˇ", baseline: [2]float64{10, 100}, advance: 3.6},
		{text: "c", baseline: [2]float64{9.6, 100}, advance: 4.8},
		{text: "x", baseline: [2]float64{20, 100}, advance: 4.8},
	}
	matcher := &ownerMatcher{page: page, painted: painted, unknown: map[int]bool{}}
	pairs := matcher.match()
	if pairs[0] != 0 || pairs[1] != 0 || pairs[2] != 1 {
		t.Fatalf("pairs %v, want the accent and its base owned by glyph 0", pairs)
	}
	owners := &glyphowner.Owners{Leaves: []string{"p", "q"}, Pages: []glyphowner.Page{page}}
	builder := newBuilder(&document{})
	builder.leaves["p"], builder.leaves["q"] = &selem{role: "P"}, &selem{role: "P"}
	tagger := &tagger{builder: builder}
	check := OwnerCheck{}
	leaves := make([]string, len(painted))
	for index := range painted {
		leaves[index], _, _ = tagger.ownerLeaf(owners, page, pairs, index, &check)
	}
	if leaves[0] != "p" || leaves[1] != "p" || leaves[2] != "q" {
		t.Fatalf("leaves %v, want the accent and its base in p, the next glyph in q", leaves)
	}
	if kind := matchKind(read(painted), expect(piece{text: "čx"}), false); kind != "accents" {
		t.Fatalf("the accented name reads as %q", kind)
	}
	// The owner's base must be the glyph painted over the accent: "č" over an "i" pairs nothing.
	painted[1].text = "i"
	if pairs := matcher.match(); len(pairs) != 2 {
		t.Fatalf("pairs %v, want the base left open", pairs)
	}
}
