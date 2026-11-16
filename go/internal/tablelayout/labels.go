package tablelayout

import (
	"regexp"
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
)

// HeaderInferred and HeaderFirstColumnInferred are the sources of a header row, and of a row header
// column, read from the kinds of content alone (decision 186, amendment 1): words or formulas over a body that is
// mostly numbers, and the same turned sideways. Nothing in the typesetting marks either; the author's hint
// and every cue above them win.
const (
	HeaderInferred            = "inferred"
	HeaderFirstColumnInferred = "first_column_inferred"
)

// citationPattern is a bracketed citation ("[5]", "[3, 7]"): a label with no letter, a word here.
var citationPattern = regexp.MustCompile(`^\[[\d,\s–-]+\]$`)

// headLabelCell: the cell is a word or a formula: text with a letter that is no number (or a bracketed citation), or
// content with a formula in it and no image or nested table, spanning one row and one column.
func headLabelCell(cell rec.Map) bool {
	if rec.FetchInt(cell, "colspan") > 1 || rec.FetchInt(cell, "rowspan") > 1 {
		return false
	}
	if text, ok := cellText(cell); ok && citationPattern.MatchString(text) || wordsCell(cell) {
		return true
	}
	formula := false
	for _, value := range rec.ListOf(cell["content"]) {
		switch rec.AsMap(value)["type"] {
		case "math":
			formula = true
		case "text":
		default:
			return false
		}
	}
	return formula
}

// inferredHeaderRows is [0] when the first row is all words or formulas (two cells at least) and the
// body below it is mostly numbers: at least two, and at least half of the written body cells (a table
// of a label column and numbers has exactly half). A body of words, or of formulas, is no such table: a
// glossary, a list of definitions or a layout of equal cells has no header row to infer.
func (l *Layout) inferredHeaderRows() []int {
	if rec.FetchInt(l.Model.Data, "row_count") < 2 || rec.FetchInt(l.Model.Data, "column_count") < 2 {
		return nil
	}
	labels, numbers, written := 0, 0, 0
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		if rec.FetchInt(cell, "row") == 0 {
			if !headLabelCell(cell) {
				return nil
			}
			labels++
			continue
		}
		written += boolCount(!blankCell(cell))
		numbers += boolCount(numberCell(cell))
	}
	if labels < 2 || numbers < 2 || numbers*2 < written {
		return nil
	}
	return []int{0}
}

// inferredHeaderColumns is [0] when the first column below the header rows is all words or formulas
// (two cells at least, none spanning) and the other columns of the body are mostly numbers: at least
// two, and more than half of their written cells.
func (l *Layout) inferredHeaderColumns() []int {
	if rec.FetchInt(l.Model.Data, "row_count") < 2 || rec.FetchInt(l.Model.Data, "column_count") < 2 {
		return nil
	}
	rows := l.HeaderRows()
	labels, numbers, written := 0, 0, 0
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		if slices.Contains(rows, int(rec.FetchInt(cell, "row"))) {
			continue
		}
		if rec.FetchInt(cell, "column") == 0 {
			if !headLabelCell(cell) {
				return nil
			}
			labels++
			continue
		}
		written += boolCount(!blankCell(cell))
		numbers += boolCount(numberCell(cell))
	}
	if labels < 2 || numbers < 2 || numbers*2 <= written {
		return nil
	}
	return []int{0}
}
