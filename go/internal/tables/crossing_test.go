package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func crossingRow(index int64, top float64) rec.Map {
	return rec.Map{"row": index, "origin_sp": []any{100.0, top}, "height": 700.0, "depth": 300.0}
}

// A \multirow{3} over two rows before an \hline: the full rule printed between its second and
// third rows ends it there, while a \cline over the other columns leaves it whole (fourth cohort
// 1159).
func TestMultirowEndsAtARuleAcrossItsColumns(t *testing.T) {
	rows := []any{crossingRow(0, 0), crossingRow(1, 1000), crossingRow(2, 2000)}
	cline := rec.Map{"rect_sp": []any{150.0, 995.0, 250.0, 10.0}, "in_row": false}
	hline := rec.Map{"rect_sp": []any{100.0, 1995.0, 300.0, 10.0}, "in_row": false}
	fragments := []any{rec.Map{"rows": rows, "rules": []any{cline, hline}}}
	boundaries := []int64{0, 50, 150, 300}
	group := rec.Map{"row": int64(0), "column": int64(0), "colspan": int64(1), "rowspan": int64(3)}
	right := rec.Map{"row": int64(0), "column": int64(1), "colspan": int64(2), "rowspan": int64(3)}
	cutAtCrossingRules([]rec.Map{group, right}, fragments, boundaries)
	if got := fetchInt(group, "rowspan"); got != 2 {
		t.Fatalf("group rowspan %d, want 2 (ended by the full rule)", got)
	}
	if got := fetchInt(right, "rowspan"); got != 1 {
		t.Fatalf("right rowspan %d, want 1 (the cline crosses it after the first row)", got)
	}
}
