package tablelayout

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A rule shorter than a browser layout unit is no rule of the layout (spite: a \cline of 4.6e-07 of its
// cell failed the export with "rule paint/identity (visible false)"); a real one stays.
func TestAppendRuleDropsAHairline(t *testing.T) {
	l := &Layout{ColumnWidths: []any{int64(20_000_000)}, RowHeights: []any{int64(500_000)}, segments: map[string][]rec.Map{}}
	cell := rec.Map{"id": "c", "row": int64(0), "column": int64(0), "rowspan": int64(1), "colspan": int64(1)}
	l.appendRule(cell, "horizontal", "bottom", 0.0, 4.61e-07, int64(26214), int64(13107))
	if got := len(l.RulesFor(cell)); got != 0 {
		t.Errorf("hairline kept: %d rules", got)
	}
	l.appendRule(cell, "horizontal", "bottom", 0.0, 0.5, int64(26214), int64(13107))
	if got := len(l.RulesFor(cell)); got != 1 {
		t.Errorf("a half-cell rule: %d rules, want 1", got)
	}
}
