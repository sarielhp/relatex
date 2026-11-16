package tablelayout

import "github.com/sarielhp/relatex/internal/rec"

// Header rows decision 131 reads beside decisions 088 and 111: booktabs' rules and \multicolumn
// group headings. Each is a rule of typesetting, never a guess at content; the acceptance reads the
// same rules on its own (lib/relatex/table_header_rows.rb).

// maxBooktabsHead and maxGroupRows bound the head these rules read: a \midrule below the fourth
// row, or more than two rows of group headings, is no head relatex recognises.
const (
	maxBooktabsHead = 3
	maxGroupRows    = 2
)

// booktabsHeaderRows: booktabs' \toprule … \midrule … \bottomrule. The table's top and bottom
// boundaries carry a full rule, and the first full rule below the top (after row 1, 2 or 3, with a
// body row below it) is thinner than both: the rows above it are the header rows, whatever rules
// the body holds (a \midrule between body rows is no evidence against the head).
func (l *Layout) booktabsHeaderRows() []int {
	count := int(rec.FetchInt(l.Model.Data, "row_count"))
	if count < 2 || rec.FetchInt(l.Model.Data, "column_count") < 2 {
		return nil
	}
	boundaries := l.boundaryRules()
	if !l.fullRule(boundaries[0]) || !l.fullRule(boundaries[count]) {
		return nil
	}
	for head := 1; head <= maxBooktabsHead && head < count; head++ {
		if !l.fullRule(boundaries[head]) {
			continue
		}
		middle := ruleThickness(boundaries[head])
		if middle >= ruleThickness(boundaries[0]) || middle >= ruleThickness(boundaries[count]) {
			return nil
		}
		return leading(head)
	}
	return nil
}

// spanHeaderRows: \multicolumn group headings over a row of column labels. The leading rows (one
// or two) each hold a written cell spanning two or more columns but not all of them; the next row
// spans no columns and has a written cell starting in it under every column the headings span; no
// later row holds a group span, and a body row follows. The heading rows and the label row are the
// header rows.
func (l *Layout) spanHeaderRows() []int {
	count := int(rec.FetchInt(l.Model.Data, "row_count"))
	if count < 3 || rec.FetchInt(l.Model.Data, "column_count") < 3 {
		return nil
	}
	groups := 0
	for groups < maxGroupRows && groups < count && l.groupRow(groups) {
		groups++
	}
	if groups == 0 || groups+1 >= count || l.spanningRow(groups) || !l.labelsUnder(groups) {
		return nil
	}
	for row := groups + 1; row < count; row++ {
		if l.groupRow(row) {
			return nil
		}
	}
	return leading(groups + 1)
}

// groupRow: a cell starting in the row spans two or more columns, not all, and holds text.
func (l *Layout) groupRow(row int) bool {
	columns := rec.FetchInt(l.Model.Data, "column_count")
	for _, cell := range l.rowCells(row) {
		if span := rec.FetchInt(cell, "colspan"); span >= 2 && span < columns && textCell(cell) {
			return true
		}
	}
	return false
}

// spanningRow: a cell starting in the row spans two or more columns.
func (l *Layout) spanningRow(row int) bool {
	for _, cell := range l.rowCells(row) {
		if rec.FetchInt(cell, "colspan") >= 2 {
			return true
		}
	}
	return false
}

// labelsUnder: under every column a group heading above spans, the label row has a cell of its own
// that holds text.
func (l *Layout) labelsUnder(labels int) bool {
	for row := range labels {
		for _, group := range l.rowCells(row) {
			if rec.FetchInt(group, "colspan") < 2 {
				continue
			}
			first := int(rec.FetchInt(group, "column"))
			for column := first; column < first+int(rec.FetchInt(group, "colspan")); column++ {
				label := l.cell(l.Model.Occupancy[labels][column])
				if rec.FetchInt(label, "row") != int64(labels) || !textCell(label) {
					return false
				}
			}
		}
	}
	return true
}

// rowCells are the cells that start in a row.
func (l *Layout) rowCells(row int) []rec.Map {
	cells := []rec.Map{}
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		if cell := rec.AsMap(value); rec.FetchInt(cell, "row") == int64(row) {
			cells = append(cells, cell)
		}
	}
	return cells
}

// leading are the indices 0 … count-1.
func leading(count int) []int {
	indices := make([]int, count)
	for index := range indices {
		indices[index] = index
	}
	return indices
}
