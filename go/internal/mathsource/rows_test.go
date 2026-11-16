package mathsource

import (
	"testing"
)

func TestRowRangesSplitAtRowBreaksOutsideBracesAndEnvironments(t *testing.T) {
	body := "\nu &= v + w \\\\\nf(t) &= \\int_0^1 g(s)\\,ds\n"
	var rows []string
	for _, r := range RowRanges(body) {
		rows = append(rows, body[r[0]:r[1]])
	}
	want := []string{"u &= v + w", "f(t) &= \\int_0^1 g(s)\\,ds"}
	if len(rows) != 2 || rows[0] != want[0] || rows[1] != want[1] {
		t.Errorf("rows = %q, want %q", rows, want)
	}
	nested := "a &= \\begin{array}{c} 1 \\\\ 2 \\end{array} \\\\[2pt] b % note \\\\ c\n"
	rows = nil
	for _, r := range RowRanges(nested) {
		rows = append(rows, nested[r[0]:r[1]])
	}
	if len(rows) != 2 || rows[0] != "a &= \\begin{array}{c} 1 \\\\ 2 \\end{array}" || rows[1] != "b % note \\\\ c" {
		t.Errorf("nested rows = %q", rows)
	}
	if got := RowRanges(" \\\\ \n"); len(got) != 0 {
		t.Errorf("blank rows were kept: %v", got)
	}
}

// RowRanges and Rows agree on how many rows a body has.
func TestRowRangesAgreeWithRows(t *testing.T) {
	for _, body := range []string{"a \\\\ b \\\\ c", "x", "a & b \\\\[3pt] c & d \\\\", "\\begin{cases} 1 \\\\ 2 \\end{cases} \\\\ z"} {
		if len(RowRanges(body)) != len(Rows(body)) {
			t.Errorf("%q: %d ranges, %d rows", body, len(RowRanges(body)), len(Rows(body)))
		}
	}
}
