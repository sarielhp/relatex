package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func hiddenTestCell(row, column int64, text string) rec.Map {
	content := []any{}
	if text != "" {
		content = append(content, rec.Map{"type": "text", "text": text})
	}
	return rec.Map{"row": row, "column": column, "colspan": int64(1), "content": content}
}

// A column the class hid (aastex splitdeluxetable's other parts) has no width and only empty
// cells: it leaves the grid and the columns after it move up (paper 372 of the second cohort).
func TestHiddenEmptyColumnsAreDropped(t *testing.T) {
	cells := []rec.Map{hiddenTestCell(0, 0, ""), hiddenTestCell(0, 1, "a"), hiddenTestCell(1, 0, ""), hiddenTestCell(1, 1, "b")}
	kept, boundaries := dropHiddenColumns(cells, []int64{0, 1, 500})
	if len(kept) != 2 || len(boundaries) != 2 || boundaries[1] != 499 {
		t.Fatalf("kept %d cells, boundaries %v", len(kept), boundaries)
	}
	if got := fetchInt(kept[0], "column"); got != 0 {
		t.Fatalf("column %d, want 0", got)
	}
}

// A declared column no cell starts in and only wider spans cross has no width: it measures one
// scaled point (deluxetable {cccccccc} over seven filled columns; fifth cohort 1210). A column
// cells start in may not collapse.
func TestEmptyDeclaredColumnMeasuresOneScaledPoint(t *testing.T) {
	starts := map[int64][]any{0: {int64(0)}, 1: {int64(100)}}
	got := orderedBoundaries([]int64{0, 100, 200, 200}, starts)
	if got[3] != 201 {
		t.Fatalf("boundaries %v, want the last at 201", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a started column of no width was accepted")
		}
	}()
	orderedBoundaries([]int64{0, 100, 100}, starts)
}

// That column leaves the grid and the note row spanning it narrows by one (fifth cohort 1210);
// a span over narrow columns only keeps them.
func TestSpannedHiddenColumnIsDropped(t *testing.T) {
	note := hiddenTestCell(1, 0, "note")
	note["colspan"] = int64(3)
	cells := []rec.Map{hiddenTestCell(0, 0, "a"), hiddenTestCell(0, 1, "b"), hiddenTestCell(0, 2, ""), note}
	kept, boundaries := dropHiddenColumns(cells, []int64{0, 100, 200, 201})
	if len(kept) != 3 || len(boundaries) != 3 || fetchInt(note, "colspan") != 2 || fetchInt(note, "column") != 0 {
		t.Fatalf("kept %d cells, boundaries %v, note %v", len(kept), boundaries, note)
	}
	narrow := hiddenTestCell(0, 1, "")
	narrow["colspan"] = int64(2)
	kept, boundaries = dropHiddenColumns([]rec.Map{hiddenTestCell(0, 0, "a"), narrow}, []int64{0, 100, 101, 102})
	if len(kept) != 2 || len(boundaries) != 4 {
		t.Fatalf("kept %d cells, boundaries %v", len(kept), boundaries)
	}
}

func TestNarrowColumnWithContentIsKept(t *testing.T) {
	cells := []rec.Map{hiddenTestCell(0, 0, "x"), hiddenTestCell(0, 1, "a")}
	kept, boundaries := dropHiddenColumns(cells, []int64{0, 1, 500})
	if len(kept) != 2 || len(boundaries) != 3 {
		t.Fatalf("kept %d cells, boundaries %v", len(kept), boundaries)
	}
}

// A kept column is as wide as its cells: the hidden columns around it add nothing to it.
func TestHiddenColumnsAddNoWidthToKeptOnes(t *testing.T) {
	cells := []rec.Map{hiddenTestCell(0, 0, "a"), hiddenTestCell(0, 1, ""), hiddenTestCell(0, 2, ""), hiddenTestCell(0, 3, "b")}
	_, boundaries := dropHiddenColumns(cells, []int64{0, 500, 502, 504, 904})
	if len(boundaries) != 3 || boundaries[1] != 500 || boundaries[2] != 900 {
		t.Fatalf("boundaries %v, want [0 500 900]", boundaries)
	}
}
