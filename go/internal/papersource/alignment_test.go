package papersource

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/reading"
	"github.com/sarielhp/relatex/internal/rec"
)

// Lines end at the line-end flag; a formula widens its line without owning it; a glyph under a
// transform and a glyph without metrics are left out.
func TestLineReaderGathersLinesByParagraph(t *testing.T) {
	node := func(kind, key string) rec.Map {
		return rec.Map{"type": kind, "provenance": rec.Map{"inspection_key": key}}
	}
	document := rec.Map{"nodes": rec.Map{"n-par": node("paragraph", "b1"), "n-math": node("math", "m1")}}
	retention := rec.Map{"occurrences": []any{rec.Map{"id": "m1", "formula": int64(7), "pages": []any{int64(1)}}}}
	slots := map[reading.SlotKey]string{{Owner: 3, Slot: 1, Page: 1}: "b1"}
	reader := &lineReader{index: newOwnerIndex(document, retention, slots), lines: map[string][]reading.Extent{}}
	const pt = 65536
	page := glyphMapPage{Page: 1, Metrics: map[string][]int64{"1:65": {10 * pt, 0, 0}}, Matrices: map[string]any{"5": []any{}}}
	g := &page.Glyphs
	g.Text = []string{"a", "x", "a", "a", "a"}
	g.DX = []int64{100 * pt, 20 * pt, 10 * pt, 10 * pt, 10 * pt}
	g.DY, g.Font, g.Char = make([]int64, 5), []int64{1, 1, 1, 1, 1}, []int64{65, 65, 65, 65, 65}
	g.Formula, g.Owner, g.Slot = []int64{0, 7, 0, 0, 0}, []int64{3, 0, 3, 3, 3}, []int64{1, 0, 1, 1, 1}
	g.Flags = []int64{0, 4 + 16, 0, 4 + 64, 0}
	reader.page(page)
	got := reader.lines["n-par"]
	want := []reading.Extent{{Left: 99.63, Right: 129.51}, {Left: 129.51, Right: 149.44}}
	if !slices.Equal(got, want) {
		t.Fatalf("lines %+v", reader.lines)
	}
	// The skips of each line come from the flags of its last glyph (16 centred, 64 plain).
	if skips := reader.skips["n-par"]; !slices.Equal(skips, []int{reading.SkipCentre, reading.SkipPlain}) {
		t.Fatalf("skips %v", skips)
	}
}
