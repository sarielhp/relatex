package layout

import (
	"testing"

	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// A three-piece bar after "dudy" (papers/0204 page 15): MuPDF places the marks at the pen after "y",
// drops the last piece, whose origin lies within a tenth of the size of that pen, and flags the
// kept mark bold. The paint trace keeps the three pieces.
func TestDroppedStackedPieceIsRecovered(t *testing.T) {
	mark := func(flags string) string {
		return `<char c="&#x20d3;" quad="488.596 739.9 488.596 739.9 488.596 747.6 488.596 747.6" x="488.596" y="740.414" flags="` +
			flags + `"/>`
	}
	structured := func(flags ...string) *nativetrace.Node {
		chars := `<font name="CMMI12" size="11.955"><char c="y" quad="482.893 732.1 488.596 732.1 482.893 742.7 488.596 742.7" ` +
			`x="482.893" y="740.414" flags="16"/></font><font name="CMEX10" size="11.955">`
		for _, flag := range flags {
			chars += mark(flag)
		}
		return nativetrace.Parse([]byte(`<document><page><block><line>` + chars + `</font></line></block></page></document>`)).
			Descendants("page")[0]
	}
	piece := func(index int64, x, y float64) rec.Map {
		return rec.Map{"index": index, "text": "⃓", "glyph": "vextendsingle", "font": "BUWAMM+CMEX10",
			"baseline": []any{x, y}, "advance": []any{3.98, 0.0}}
	}
	ypaint := rec.Map{"index": int64(0), "text": "y", "glyph": "y", "font": "ABCDEF+CMMI12", "baseline": []any{482.893, 740.414},
		"advance": []any{5.703, 0.0}}
	near := []rec.Map{ypaint, piece(1, 489.029, 726.665), piece(2, 489.029, 733.838), piece(3, 489.029, 741.011)}
	recovered := func(page *nativetrace.Node, painted []rec.Map) []rec.Map {
		return recoverStackedMarks(glyphsFor(page, 15), charFlags(page), painted, 15)
	}
	glyphs := recovered(structured("24", "16"), near)
	if len(glyphs) != 4 || glyphs[3]["text"] != "⃓" || glyphs[3]["id"] != "p15-g4" || glyphs[2]["id"] != "p15-g3" {
		t.Fatalf("recovered %d glyphs: %v", len(glyphs), glyphs)
	}
	// No bold flag: MuPDF merged nothing. A column far from the pen (the bar after "≤" on the same
	// page, 3.3 pt away): nothing near it to merge. A whole column: nothing missing.
	far := []rec.Map{ypaint, piece(1, 491.9, 726.665), piece(2, 491.9, 733.838), piece(3, 491.9, 741.011)}
	for name, count := range map[string]int{
		"unflagged": len(recovered(structured("16", "16"), near)),
		"far":       len(recovered(structured("24", "16"), far)),
		"whole":     len(recovered(structured("24", "16", "16"), near)),
	} {
		want := 3
		if name == "whole" {
			want = 4
		}
		if count != want {
			t.Errorf("%s: %d glyphs, want %d", name, count, want)
		}
	}
}
