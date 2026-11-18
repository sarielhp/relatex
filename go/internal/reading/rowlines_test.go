package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

func rowEvidence(indent int64) (*Evidence, rec.Map, rec.Map, rec.Map) {
	e := &Evidence{scopes: newScopes(nil), origins: map[uintptr]rec.Map{}, rowIndents: map[ruleKey]int64{},
		lastBlocks: map[partition]rec.Map{}, lastKeys: map[partition]blockKey{}}
	e.activeTable, e.activeCell = int64(1), int64(5)
	owner := rec.Map{"owner": int64(7), "entity": int64(0), "role": "paragraph"}
	text := rec.Map{"type": "text", "text": "violate"}
	e.origins[rec.Identity(text)] = e.tableOrigin()
	block := rec.Map{"id": "b1", "type": "paragraph", "content": []any{text}}
	e.lastBlocks[partition{}] = block
	e.rowIndents[ruleKey{int64(7), int64(3)}] = indent
	return e, owner, block, rec.Map{"type": "text", "table": int64(1), "table_cell": int64(5)}
}

// A row of a box outside every table (an \fbox around a minipage of tabbing, walk Figure 9.1) ends
// the line of its paragraph as well.
func TestRowOfABoxOutsideATableBreaksTheLine(t *testing.T) {
	e, owner, block, _ := rowEvidence(1966080)
	e.activeTable, e.activeCell = nil, nil
	e.origins = map[uintptr]rec.Map{}
	e.breakRow(owner, 3)
	if got := len(rec.FetchList(block, "content")); got != 2 {
		t.Errorf("a row outside a table: block has %d spans, want 2", got)
	}
}

// A tabbing row after another in a table cell ends the line before it (yolk: "forI=1 tom doif
// violate" was one overflowing run); the break keeps the row's indent in its table origin.
func TestRowOfABoxInATableCellBreaksTheLine(t *testing.T) {
	e, owner, block, _ := rowEvidence(1966080)
	e.breakRow(owner, 3)
	content := rec.FetchList(block, "content")
	if len(content) != 2 {
		t.Fatalf("a break joins the block, got %v", content)
	}
	brk := rec.AsMap(content[1])
	if brk["line_break"] != true || brk["text"] != " " {
		t.Errorf("the break is one space marked line_break, got %v", brk)
	}
	if got := e.origins[rec.Identity(brk)][tablemodel.LineIndentKey]; got != int64(1966080) {
		t.Errorf("indent = %v, want 1966080", got)
	}
	if !rec.Equal(e.origins[rec.Identity(brk)]["cell"], int64(5)) {
		t.Errorf("the break belongs to the item's cell, got %v", e.origins[rec.Identity(brk)])
	}
}

// No row opens at the slot, the item sits in no table, a formula is open, the last line is a break
// already or belongs to another cell: no break.
func TestNoRowBreakWithoutARowInTheCell(t *testing.T) {
	cases := map[string]func(*Evidence, rec.Map, rec.Map) (int64, rec.Map){
		"slot opens no row": func(*Evidence, rec.Map, rec.Map) (int64, rec.Map) { return 4, nil },
		"table line, item outside": func(e *Evidence, _, _ rec.Map) (int64, rec.Map) {
			e.activeTable, e.activeCell = nil, nil
			return 3, nil
		},
		"formula open": func(e *Evidence, _, _ rec.Map) (int64, rec.Map) {
			e.pending = &pendingMath{}
			return 3, nil
		},
		"another cell": func(e *Evidence, _, _ rec.Map) (int64, rec.Map) { e.activeCell = int64(6); return 3, nil },
	}
	for name, setup := range cases {
		e, owner, block, item := rowEvidence(0)
		slot, _ := setup(e, owner, item)
		e.breakRow(owner, slot)
		if got := len(rec.FetchList(block, "content")); got != 1 {
			t.Errorf("%s: block has %d spans, want 1", name, got)
		}
	}
	e, owner, block, _ := rowEvidence(0)
	e.breakRow(owner, 3)
	e.breakRow(owner, 3)
	if got := len(rec.FetchList(block, "content")); got != 2 {
		t.Errorf("a break after a break: block has %d spans, want 2", got)
	}
}
