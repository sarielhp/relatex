package pdftag

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// Glyphs shown in rendering mode 2 are filled and then stroked when MuPDF flushes its text buffer
// (here at the mode change and at ET): the trace has a paint for each stroke, after the fills.
func TestInterpreterNumbersTracePaints(t *testing.T) {
	file := &pdfFile{objects: map[string]any{"obj:5 0 R": map[string]any{"value": map[string]any{"/Subtype": "/Type1"}}}}
	resources := map[string]any{"/Font": map[string]any{"/F1": "5 0 R"}}
	operations, err := parseContent([]byte("BT /F1 10 Tf 2 Tr 1 0 0 1 10 20 Tm (ab) Tj 0 Tr (c) Tj ET " +
		"BT 2 Tr 1 0 0 1 10 40 Tm (d) Tj 0 Tr ET BT 1 0 0 1 10 60 Tm (e) Tj ET q 2 Tr BT (f) Tj ET Q"))
	if err != nil {
		t.Fatal(err)
	}
	in := newInterpreter(file, resources)
	if _, err := in.run(operations); err != nil {
		t.Fatal(err)
	}
	// a b (fills), a b (strokes), c, d, d (stroke), e, f, f (stroke).
	if want := []int{0, 1, 4, 5, 7, 8}; !slices.Equal(in.trace.of, want) || in.trace.total != 10 || in.trace.trailing() != 1 {
		t.Fatalf("paints %v of %d, want %v of 10", in.trace.of, in.trace.total, want)
	}
}

// The layout numbers the trace's paints and binds no glyph to a stroke: a page whose glyphs are
// stroked too maps by paint index, checks every placed operator and may end before the last stroke.
func TestMapPaintsSkipsStrokes(t *testing.T) {
	box := [4]float64{0, 0, 100, 100}
	items := []item{
		{kind: kindShow, first: 0, count: 1, placed: true, origin: [2]float64{10, 90}},
		{kind: kindShow, first: 1, count: 1, placed: true, origin: [2]float64{20, 90}},
		{kind: kindShow, first: 2, count: 1, placed: true, origin: [2]float64{30, 90}},
	}
	painted := []paintedGlyph{{text: "G", baseline: [2]float64{10, 10}}, {unknown: true},
		{text: "G", baseline: [2]float64{20, 10}}, {unknown: true}, {text: "x", baseline: [2]float64{30, 10}}}
	paints := tracePaints{of: []int{0, 2, 4}, total: 6}
	m, err := mapPaints(items, painted, pageFrame{box: box}, paints)
	if err != nil || !slices.Equal(m.mapping, []int{0, 2, 4}) {
		t.Fatalf("mapping %v: %v", m.mapping, err)
	}
	if _, err := mapGlyphs(items, painted, box, 3); err == nil {
		t.Fatal("glyphs counted once each were mapped onto stroke paints")
	}
}

// A form's glyphs follow the page's in the trace, with their own strokes.
func TestTracePaintsAppendAndSpan(t *testing.T) {
	page := identityPaints(2)
	page.append(tracePaints{of: []int{0, 2}, total: 4})
	if !slices.Equal(page.of, []int{0, 1, 2, 4}) || page.total != 6 || page.span(2, 4) != 4 || page.span(0, 2) != 2 {
		t.Fatalf("paints %+v", page)
	}
}

// A picture's labels TeX placed without a known position ("r2") face paint indices the layout
// skipped: both runs are passed together, and the label's text does not pair with the caption's
// "r2" below (papers2 0236's figure 1).
func TestOwnerMatcherPassesSkippedRuns(t *testing.T) {
	page := glyphowner.Page{Number: 1, X: []float64{0, 0, 10, 20, 30}, Y: []float64{0, 0, 50, 50, 50},
		Text: []string{"r", "2", "F", "r", "2"}, Leaf: []int{1, 1, 2, 2, 2}}
	painted := []paintedGlyph{{unknown: true}, {unknown: true}, {text: "F", baseline: [2]float64{10, 50}},
		{text: "r", baseline: [2]float64{20, 50}}, {text: "2", baseline: [2]float64{30, 50}}}
	pairs := (&ownerMatcher{page: page, painted: painted, unknown: map[int]bool{0: true, 1: true}}).match()
	if len(pairs) != 3 || pairs[2] != 2 || pairs[3] != 3 || pairs[4] != 4 {
		t.Fatalf("pairs %v", pairs)
	}
}

// Where TeX does not know the position, the text decides; the layout spells a delimiter's size
// variant with a variation selector (cmex's parenleftbig), TeX the plain character.
func TestOwnerMatcherIgnoresVariationSelectors(t *testing.T) {
	page := glyphowner.Page{Number: 1, X: []float64{0, 0, 0}, Y: []float64{0, 0, 0}, Text: []string{"(", "x", ")"}, Leaf: []int{1, 1, 1}}
	painted := []paintedGlyph{{text: "(︁", baseline: [2]float64{12, 40}}, {text: "x", baseline: [2]float64{20, 40}},
		{text: ")︁", baseline: [2]float64{28, 40}}}
	pairs := (&ownerMatcher{page: page, painted: painted, unknown: map[int]bool{0: true, 1: true, 2: true}}).match()
	if len(pairs) != 3 || pairs[0] != 0 || pairs[2] != 2 {
		t.Fatalf("pairs %v", pairs)
	}
}

// identityPaints are the paints of glyphs painted once each.
func identityPaints(glyphs int) tracePaints {
	of := make([]int, glyphs)
	for index := range of {
		of[index] = index
	}
	return tracePaints{of: of, total: glyphs}
}
