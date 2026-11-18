package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// glyphEvent is a rich_table_glyphs event: one glyph of the given cell per baseline.
func glyphEvent(cell, size int64, baselines ...int64) rec.Map {
	glyphs := []any{}
	for _, y := range baselines {
		glyphs = append(glyphs, rec.Map{"cell": cell, "size_sp": size, "baseline_sp": []any{int64(0), y}})
	}
	return rec.Map{"code": "rich_table_glyphs", "table": int64(1), "glyphs": glyphs}
}

// A minipage cell whose text TeX broke over lines wraps in the browser (good_pairs Figure 1.3,
// max_level Figure 1.1); a one-line cell keeps its width-exact nowrap, and a script's shifted baseline
// is not a second line.
func TestWrapsInTeXReadsLinesFromGlyphBaselines(t *testing.T) {
	declaration := rec.Map{"table": int64(1), "id": int64(7)}
	size := int64(717619)
	cases := map[string]struct {
		events []rec.Map
		want   bool
	}{
		"two lines":  {[]rec.Map{glyphEvent(7, size, 1000000, 1000000, 1900000)}, true},
		"one line":   {[]rec.Map{glyphEvent(7, size, 1000000, 1000000)}, false},
		"script":     {[]rec.Map{glyphEvent(7, size, 1000000, 1200000), glyphEvent(7, size/2, 1000000)}, false},
		"other cell": {[]rec.Map{glyphEvent(8, size, 1000000, 1900000)}, false},
		"no glyphs":  {nil, false},
	}
	for name, c := range cases {
		e := &rich{events: c.events}
		if got := e.wrapsInTeX(declaration); got != c.want {
			t.Errorf("%s: got %v", name, got)
		}
	}
}
