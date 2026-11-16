package tablelayout

import (
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
)

// HeaderFirstColumnSpan is the source of a row header column read from a \multirow first column (decision 186),
// beside withLabelRow, a head a rule ends early: two readings of what the typesetting shows, neither a guess at content.
const HeaderFirstColumnSpan = "first_column_span"

// withLabelRow extends head rows read from a rule (booktabs' \midrule or a ruled head) to the row of
// column labels under their group headings: the last head row holds a written cell spanning two or more
// columns but not all, the next row has a cell of its own under every spanned column (a single column,
// text that is no number), no later row holds a group span, and a body row follows. A \cmidrule under the
// group headings ends the head there for the rule readers; the labels belong to it all the same. The head
// stays within maxBooktabsHead rows.
func (l *Layout) withLabelRow(rows []int) []int {
	count := int(rec.FetchInt(l.Model.Data, "row_count"))
	last := rows[len(rows)-1]
	labels := last + 1
	if len(rows) >= maxBooktabsHead || labels+1 >= count || !l.groupRow(last) || !l.labelsUnder(labels) || !l.plainLabels(labels) {
		return rows
	}
	for row := labels + 1; row < count; row++ {
		if l.groupRow(row) {
			return rows
		}
	}
	return append(slices.Clone(rows), labels)
}

// plainLabels: under every column a group heading above spans, the label cell spans one column and
// holds words, not a number.
func (l *Layout) plainLabels(labels int) bool {
	for row := range labels {
		for _, group := range l.rowCells(row) {
			span := int(rec.FetchInt(group, "colspan"))
			if span < 2 {
				continue
			}
			first := int(rec.FetchInt(group, "column"))
			for column := first; column < first+span; column++ {
				label := l.cell(l.Model.Occupancy[labels][column])
				if rec.FetchInt(label, "colspan") != 1 || numberCell(label) {
					return false
				}
			}
		}
	}
	return true
}

// spanHeaderColumns is [0] when the first column below the header rows is a column of row-group labels:
// at least one of its cells spans two or more rows (and one column) and holds words, and none holds a
// number (a numeric first column is data).
func (l *Layout) spanHeaderColumns() []int {
	count := int(rec.FetchInt(l.Model.Data, "row_count"))
	// A cell spanning down from the first row heads columns (a head no reader found), it names no group.
	start := max(l.bodyStart(), 1)
	if count-start < 2 || rec.FetchInt(l.Model.Data, "column_count") < 2 {
		return nil
	}
	groups := 0
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		if rec.FetchInt(cell, "column") != 0 || int(rec.FetchInt(cell, "row")) < start {
			continue
		}
		if numberCell(cell) {
			return nil
		}
		groups += boolCount(rec.FetchInt(cell, "rowspan") > 1 && rec.FetchInt(cell, "colspan") == 1 && wordsCell(cell))
	}
	if groups == 0 {
		return nil
	}
	return []int{0}
}
