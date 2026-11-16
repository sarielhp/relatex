package pdftag

import "testing"

// TestGlyphPictureBodyOnPictureEdge: a glyph belongs to a drawn picture when its body does, not its
// baseline origin. TeX sets the text after an inline picture with its origin exactly on the box's
// right edge (papers 0191's "," and ")"), and a glyph starting there is the text's.
func TestGlyphPictureBodyOnPictureEdge(t *testing.T) {
	// The frame maps layout y down onto this 100x20 box; x is unchanged, so these glyphs sit in
	// device space at the positions below.
	frame := pageFrame{box: [4]float64{0, 0, 100, 20}}
	tagger := &tagger{painted: map[int][]paintedGlyph{1: {
		{text: ",", baseline: [2]float64{100, 10}, advance: 3}, // body starts at the box's right edge
		{text: "a", baseline: [2]float64{50, 10}, advance: 5},  // well inside
		{text: "b", baseline: [2]float64{0, 10}, advance: 5},   // body straddles the left edge
	}}, builder: &builder{leaves: map[string]*selem{"fig": {role: "Figure", leaf: "fig"}},
		hosts: map[string]string{"fig": "host"}}}
	page := &prepared{number: 1, mapper: &glyphMapper{mapping: []int{0, 1, 2}, frame: frame}}
	pictures := []picture{{leaf: "fig", box: bounds{0, 0, 100, 20}}}
	for ordinal, want := range []string{"", "fig", "fig"} {
		if got := tagger.glyphPicture(page, pictures, ordinal); got != want {
			t.Errorf("glyph %d: picture %q, want %q", ordinal, got, want)
		}
	}
}
