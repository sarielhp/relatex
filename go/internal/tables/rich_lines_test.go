package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

func rowCell(kind string, indent int64) (*rich, []rec.Map) {
	style := rec.Map{"size_sp": int64(655360)}
	text := func(value string) rec.Map { return rec.Map{"type": "text", "text": value, "style": style} }
	brk := rec.Map{"type": "text", "text": " ", "line_break": true}
	spans := []rec.Map{text("for i do"), brk, text(" if v then")}
	e := &rich{origins: map[uintptr]rec.Map{}, cells: map[string]rec.Map{key(int64(1)): {"kind": kind}}}
	e.origins[rec.Identity(brk)] = rec.Map{"table": int64(1), "cell": int64(1), tablemodel.LineIndentKey: indent}
	return e, spans
}

// A cell that does not wrap keeps the break between its rows, and the next row's indent as the em
// spaces it is wide (a tabbing \quad is one em); the break's leading space goes.
func TestRowsOfAnUnwrappedCellKeepTheirBreaksAndIndents(t *testing.T) {
	e, spans := rowCell("l", 1310720)
	got := e.rowLines(int64(1), spans)
	want := []string{"for i do", " ", "  ", "if v then"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d spans", got, len(want))
	}
	for index, span := range got {
		if span["text"] != want[index] {
			t.Errorf("span %d = %q, want %q", index, span["text"], want[index])
		}
	}
	if got[1]["line_break"] != true {
		t.Errorf("the break is kept, got %v", got[1])
	}
}

// A row of a box at the cell's left edge has no indent spaces, only the break.
func TestUnindentedRowKeepsNoSpaces(t *testing.T) {
	e, spans := rowCell("c", 0)
	got := e.rowLines(int64(1), spans)
	if len(got) != 3 || got[2]["text"] != "if v then" {
		t.Errorf("got %v, want the break and the row's text", got)
	}
}

// A cell that wraps its lines (p, m, b) reads its rows as the words of its paragraph, as before:
// the breaks go and the text they split joins again.
func TestRowsOfAWrappingCellJoinAgain(t *testing.T) {
	for _, kind := range []string{"p", "m", "b"} {
		e, spans := rowCell(kind, 1310720)
		got := e.rowLines(int64(1), spans)
		if len(got) != 1 || got[0]["text"] != "for i do if v then" {
			t.Errorf("%s cell: got %v, want one joined span", kind, got)
		}
	}
}

// A cell with no break is left alone.
func TestCellWithoutBreaksIsUnchanged(t *testing.T) {
	e, spans := rowCell("l", 0)
	plain := []rec.Map{spans[0], spans[2]}
	got := e.rowLines(int64(1), plain)
	if len(got) != 2 || got[0]["text"] != "for i do" || got[1]["text"] != " if v then" {
		t.Errorf("got %v", got)
	}
}
