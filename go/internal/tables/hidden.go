package tables

import (
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// hiddenColumnWidth is the widest a column may be and still be one the class hid: column
// boundaries are kept strictly increasing, so a column with no width at all measures one scaled point.
const hiddenColumnWidth = 2

// dropHiddenColumns removes the columns a class hid (aastex's splitdeluxetable typesets every
// column of the table in each part and swallows the other parts' cells at zero width): a column
// that has no width and no cell of its own with content has no area in the print or the
// browser. Its cells go, the columns after them move up, a span across it narrows by it, and the
// boundaries close up. A declared column no cell fills that a note row spans is one too
// (deluxetable {cccccccc} over seven filled columns; fifth cohort 1210).
func dropHiddenColumns(cells []rec.Map, boundaries []int64) ([]rec.Map, []int64) {
	columns := int64(len(boundaries) - 1)
	hidden := hiddenColumns(cells, boundaries)
	shift := make([]int64, columns+1)
	kept := []int64{boundaries[0]}
	for column := range columns {
		shift[column+1] = shift[column]
		if hidden[column] {
			shift[column+1]++
		} else {
			// Hidden columns have no width of their own to add to the next one.
			kept = append(kept, kept[len(kept)-1]+boundaries[column+1]-boundaries[column])
		}
	}
	if shift[columns] == 0 {
		return cells, boundaries
	}
	result := []rec.Map{}
	for _, cell := range cells {
		column, span := fetchInt(cell, "column"), fetchInt(cell, "colspan")
		end := min(column+span, columns)
		first := column
		for first < end && hidden[first] {
			first++
		}
		if first == end {
			continue
		}
		cell["column"] = first - shift[first]
		if visible := (end - shift[end]) - (first - shift[first]); visible != span {
			cell["colspan"] = visible
		}
		result = append(result, cell)
	}
	return result, kept
}

// hiddenColumns marks the columns with no width that no cell needs: a cell with content keeps
// its column, and a span keeps its columns when all of them are narrow, so it never vanishes.
func hiddenColumns(cells []rec.Map, boundaries []int64) []bool {
	columns := int64(len(boundaries) - 1)
	hidden := make([]bool, columns)
	for column := range hidden {
		hidden[column] = boundaries[column+1]-boundaries[column] <= hiddenColumnWidth
	}
	narrow := append([]bool(nil), hidden...)
	for _, cell := range cells {
		column, span := fetchInt(cell, "column"), fetchInt(cell, "colspan")
		end := min(column+span, columns)
		if span == 1 && emptyContent(cell) || span > 1 && !allNarrow(narrow[column:end]) {
			continue
		}
		for x := column; x < end; x++ {
			hidden[x] = false
		}
	}
	return hidden
}

func allNarrow(columns []bool) bool {
	for _, narrow := range columns {
		if !narrow {
			return false
		}
	}
	return true
}

func emptyContent(cell rec.Map) bool {
	for _, span := range maps(tablemodel.FetchList(cell, "content")) {
		if span["type"] != "text" || rec.Strip(rec.FetchString(span, "text")) != "" {
			return false
		}
	}
	return true
}
