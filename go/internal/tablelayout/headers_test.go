package tablelayout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// tableFixture is a table_grid span of the HTML writer's testdata (a cohort paper's table).
func tableFixture(t *testing.T, name string) rec.Map {
	t.Helper()
	tablemodel.Root = filepath.Join("..", "..", "..") // the schemas the grid is validated by
	file, err := os.Open(filepath.Join("..", "html", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	span, err := canonjson.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return rec.AsMap(span)
}

func TestBlankCellIsNoHeader(t *testing.T) {
	if !blankCell(rec.Map{"content": []any{rec.Map{"type": "text", "text": "  "}}}) ||
		blankCell(rec.Map{"content": []any{rec.Map{"type": "math", "node": "m"}}}) {
		t.Fatal("blank cells")
	}
}

// A header cell holding only a formula is <th> too (decision 095, amendment 2), as is one with text beside it.
func TestFormulaOnlyCellIsHeaderCell(t *testing.T) {
	span := tableFixture(t, "ruled-table.json")
	grid := rec.FetchMap(span, "grid")
	formula := rec.Map{"type": "math", "node": "m"}
	changed := false
	for _, value := range rec.FetchList(grid, "cells") {
		if cell := rec.AsMap(value); rec.FetchInt(cell, "row") == 0 && !changed {
			cell["content"], changed = []any{formula}, true
		}
	}
	layout := NewLayout(NewTable(grid, nil))
	headers := 0
	for _, value := range rec.FetchList(grid, "cells") {
		if layout.HeaderCell(rec.AsMap(value)) {
			headers++
		}
	}
	if headers != 2 {
		t.Fatalf("header cells %d, want 2", headers)
	}
	if !textCell(rec.Map{"content": []any{formula, rec.Map{"type": "text", "text": " (s)"}}}) {
		t.Fatal("a formula with text is no header cell")
	}
}

func TestStrongerRule(t *testing.T) {
	single := []boundaryRule{{side: "top", centre: 0, thickness: 26214}}
	double := []boundaryRule{{side: "top", centre: 0, thickness: 26214}, {side: "bottom", centre: 1, thickness: 26214}}
	thick := []boundaryRule{{side: "top", centre: 0, thickness: 52428}}
	if !stronger(double, single) || !stronger(thick, single) || stronger(single, single) {
		t.Fatal("a double or thicker rule is stronger, an equal one is not")
	}
}

// boldRows sets the text of the grid's cells bold in the rows where bold says so.
func boldRows(grid rec.Map, bold func(row int64) bool) {
	for _, value := range rec.FetchList(grid, "cells") {
		cell := rec.AsMap(value)
		if !bold(rec.FetchInt(cell, "row")) {
			continue
		}
		for _, span := range rec.ListOf(cell["content"]) {
			if text := rec.AsMap(span); text["type"] == "text" {
				text["style"] = rec.Map{"source": "nfss-font-id", "font_id": int64(1), "font_name": "cmbx10", "size_sp": int64(655360),
					"encoding": "OT1", "family": "cmr", "series": "bx", "shape": "n", "generic": "serif"}
			}
		}
	}
}

// A fully ruled grid has no header rows by its rules, only the numeric inference (text over numbers,
// decision 167 amendment 3); a bold first row over a regular body is the header (decision 111), and a
// body as bold as the first row is not.
func TestBoldFirstRowIsTheHeader(t *testing.T) {
	grid := rec.FetchMap(tableFixture(t, "grid-table.json"), "grid")
	if layout := NewLayout(NewTable(grid, nil)); layout.HeaderSource() != HeaderNumeric {
		t.Fatalf("plain grid headers %v %q", layout.HeaderRows(), layout.HeaderSource())
	}
	boldRows(grid, func(row int64) bool { return row == 0 })
	layout := NewLayout(NewTable(grid, nil))
	if rows := layout.HeaderRows(); len(rows) != 1 || rows[0] != 0 || layout.HeaderSource() != HeaderBold {
		t.Fatalf("bold first row: %v %q", rows, layout.HeaderSource())
	}
	boldRows(grid, func(int64) bool { return true })
	if layout := NewLayout(NewTable(grid, nil)); layout.HeaderSource() != HeaderNumeric {
		t.Fatalf("all bold: %v %q", layout.HeaderRows(), layout.HeaderSource())
	}
}

// panelExtra is the content the kinds of a panel cell add to its picture ("p") and text ("t"): "a" an
// arrow glyph, "m" a formula alone, "L" a label "(a)", "x" a panel letter "(A)", "W" a word label, "C" a
// short capitalised word ("Cat"), "B" a bracketed word "(Cat)", "n" a number, "w" a long sentence, "g" a
// nested table.
func panelExtra(kind string) []any {
	content := []any{}
	for _, char := range kind {
		switch char {
		case 'a':
			content = append(content, rec.Map{"type": "text", "text": " → "})
		case 'm':
			content = append(content, rec.Map{"type": "math", "node": "n"})
		case 'L':
			content = append(content, rec.Map{"type": "text", "text": "(a)"})
		case 'x':
			content = append(content, rec.Map{"type": "text", "text": "(A)"})
		case 'W':
			content = append(content, rec.Map{"type": "text", "text": "Input"})
		case 'C':
			content = append(content, rec.Map{"type": "text", "text": "Cat"})
		case 'B':
			content = append(content, rec.Map{"type": "text", "text": "(Cat)"})
		case 'n':
			content = append(content, rec.Map{"type": "text", "text": "1"})
		case 'w':
			content = append(content, rec.Map{"type": "text", "text": "A sentence far too long to be a row label."})
		}
	}
	return content
}

// panelCells builds a grid from rows of cell kinds: "p" an image, "t" text, "" nothing, "pt" both, "a" an
// arrow glyph, "m" a formula alone, "L" a label "(a)", "x" a panel letter beside the cell's picture, "W" a
// word label, "n" a number, "w" a long sentence, "g" a nested table.
func panelCells(rows ...[]string) rec.Map {
	cells := []any{}
	for row, kinds := range rows {
		for column, kind := range kinds {
			content := []any{}
			if strings.Contains(kind, "p") {
				content = append(content, rec.Map{"type": "image"})
			}
			if strings.Contains(kind, "g") {
				content = append(content, rec.Map{"type": "table_grid",
					"grid": rec.Map{"row_count": int64(1), "column_count": int64(1), "cells": []any{}}})
			}
			if strings.Contains(kind, "t") {
				content = append(content, rec.Map{"type": "text", "text": "Caption"})
			}
			content = append(content, panelExtra(kind)...)
			cells = append(cells, rec.Map{"row": int64(row), "column": int64(column), "content": content})
		}
	}
	return rec.Map{"row_count": int64(len(rows)), "column_count": int64(len(rows[0])), "cells": cells}
}

func TestPresumedLayoutReadsFigurePanels(t *testing.T) {
	layouts := map[string]rec.Map{
		"pictures over captions, twice": panelCells([]string{"p", "p"}, []string{"t", "t"}, []string{"p", ""}, []string{"t", ""}),
		"a grid of pictures":            panelCells([]string{"p", "p"}, []string{"p", "p"}),
		"an empty spacer row":           panelCells([]string{"p", "p"}, []string{"", ""}, []string{"t", "t"}),
		"arrows between pictures":       panelCells([]string{"p", "a", "p"}, []string{"", "m", ""}, []string{"p", "m", "p"}),
		"row labels before pictures":    panelCells([]string{"L", "p", "p"}, []string{"t", "p", "p"}),
		"a label after pictures":        panelCells([]string{"p", "p", "L"}, []string{"p", "p", "p"}),
		// books/0003 figure 16.6: a panel letter beside its picture in the same cell.
		"a letter in the picture's cell": panelCells([]string{"px", "px"}, []string{"px", "px"}),
		// books/0003 figure 7.4: a minipage holding only the panel letter beside one picture per row.
		"a letter beside one picture":    panelCells([]string{"L", "p"}, []string{"L", "p"}),
		"a letter beside a nested table": panelCells([]string{"L", "g"}, []string{"L", "g"}),
		// books/0003 figure 4.2: a formula typeset as a panel beside the row's pictures.
		"a formula beside pictures": panelCells([]string{"p", "p", "m"}, []string{"p", "p", "p"}),
	}
	for name, grid := range layouts {
		if !PresumedLayout(grid) {
			t.Errorf("%s: not a layout", name)
		}
	}
	tables := map[string]rec.Map{
		"text first":                   panelCells([]string{"t", "t"}, []string{"p", "p"}),
		"two text rows":                panelCells([]string{"p", "p"}, []string{"t", "t"}, []string{"t", "t"}),
		"a picture beside text":        panelCells([]string{"p", "t"}, []string{"p", "t"}),
		"a cell with both":             panelCells([]string{"pt", "p"}, []string{"t", "t"}),
		"no pictures at all":           panelCells([]string{"t", "t"}, []string{"t", "t"}),
		"an arrow outside":             panelCells([]string{"p", "p", "a"}, []string{"p", "p", "p"}),
		"a word between":               panelCells([]string{"p", "w", "p"}, []string{"p", "p", "p"}),
		"a long label":                 panelCells([]string{"w", "p", "p"}, []string{"p", "p", "p"}),
		"one word label per picture":   panelCells([]string{"W", "p"}, []string{"W", "p"}),
		"a short word per picture":     panelCells([]string{"C", "p"}, []string{"C", "p"}),
		"a bracketed word per picture": panelCells([]string{"B", "p"}, []string{"B", "p"}),
		"a picture with a word":        panelCells([]string{"pW", "p"}, []string{"p", "p"}),
		"a picture beside numbers":     panelCells([]string{"p", "n"}, []string{"n", "n"}, []string{"n", "n"}),
		"a number beside a picture":    panelCells([]string{"n", "p"}, []string{"n", "p"}),
		"a letter between pictures":    panelCells([]string{"p", "L", "p"}, []string{"p", "p", "p"}),
		"a formula beside one picture": panelCells([]string{"p", "m"}, []string{"p", "p"}),
	}
	for name, grid := range tables {
		if PresumedLayout(grid) {
			t.Errorf("%s: presumed a layout", name)
		}
	}
}
