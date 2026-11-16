package tablelayout

import (
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
)

// Row header columns (decision 131). The first column is the row header when the author says so,
// when a rule sets it apart from the other columns (as decision 088 reads a ruled head), or when
// every first cell of the body is bold over a body that is not; its cells outside the header rows
// that hold text render as <th scope="row">. The acceptance reads the same rules on its own
// (lib/relatex/table_header_rows.rb).

// How a table's row header column is known (HeaderColumnSource): HeaderAuthor, or a rule or bold
// type in the first column.
const (
	HeaderFirstColumnRule = "first_column_rule"
	HeaderFirstColumnBold = "first_column_bold"
)

// HeaderColumns are the table's row header columns, in order; none when the source marks none.
func (l *Layout) HeaderColumns() []int {
	if l.columns != nil {
		return *l.columns
	}
	columns, source := authorIndices(l.Model.Data, AuthorHeaderColumns)
	if columns == nil {
		columns, source = l.ruledHeaderColumns(), HeaderFirstColumnRule
	}
	if len(columns) == 0 && source != HeaderAuthor {
		columns, source = l.boldHeaderColumns(), HeaderFirstColumnBold
	}
	if len(columns) == 0 && source != HeaderAuthor {
		columns, source = l.spanHeaderColumns(), HeaderFirstColumnSpan
	}
	if len(columns) == 0 && source != HeaderAuthor {
		columns, source = l.inferredHeaderColumns(), HeaderFirstColumnInferred
	}
	if len(columns) == 0 || !l.textual(l.headerColumnCell(columns)) {
		columns, source = []int{}, ""
	}
	l.columns, l.columnSource = &columns, source
	return columns
}

// HeaderColumnSource is how the row header columns are known: HeaderAuthor, HeaderFirstColumnRule
// or HeaderFirstColumnBold; "" when the table has none.
func (l *Layout) HeaderColumnSource() string {
	l.HeaderColumns()
	return l.columnSource
}

// headerColumnCell selects the cells of the given columns outside the header rows.
func (l *Layout) headerColumnCell(columns []int) func(rec.Map) bool {
	rows := l.HeaderRows()
	return func(cell rec.Map) bool {
		return slices.Contains(columns, int(rec.FetchInt(cell, "column"))) && !slices.Contains(rows, int(rec.FetchInt(cell, "row")))
	}
}

// bodyStart is the first row below the leading header rows.
func (l *Layout) bodyStart() int {
	start := 0
	for slices.Contains(l.HeaderRows(), start) {
		start++
	}
	return start
}

// ruledHeaderColumns is [0] when a full rule down the body after the first column sets it apart:
// fewer than half of the other inner column boundaries carry one, or its rule is stronger than each
// of theirs (a double rule, or one at least 1.2 times as thick). A table of two columns has no other
// inner boundary; its rule sets the first column apart only when the outer edges carry none (a
// framed |l|l| grid is ruled throughout).
func (l *Layout) ruledHeaderColumns() []int {
	count, columns := int(rec.FetchInt(l.Model.Data, "row_count")), int(rec.FetchInt(l.Model.Data, "column_count"))
	start := l.bodyStart()
	if count < 2 || columns < 2 || start >= count {
		return nil
	}
	top := rubynum.Float(rubynum.Sum(span(l.RowHeights, 0, start)))
	bottom := rubynum.Float(rubynum.Sum(l.RowHeights))
	boundaries := l.axisRules(verticalAxis)
	full := func(boundary int) bool { return covers(boundaries[boundary], top, bottom) }
	if !full(1) {
		return nil
	}
	if columns == 2 {
		if full(0) || full(2) {
			return nil
		}
		return []int{0}
	}
	others, ruled, weaker := 0, 0, 0
	for boundary := 2; boundary < columns; boundary++ {
		others++
		if !full(boundary) {
			continue
		}
		ruled++
		if stronger(boundaries[1], boundaries[boundary]) {
			weaker++
		}
	}
	if ruled*2 < others || weaker == ruled {
		return []int{0}
	}
	return nil
}

// boldHeaderColumns is [0] when the first column is bold over a body that is not: every first cell
// below the header rows with text sets all of it bold, most of those written cells have text (not
// formulas alone), and fewer than half of the other body cells with text are bold. A cell spanning
// from the first column into others is no first cell and is not counted.
func (l *Layout) boldHeaderColumns() []int {
	if rec.FetchInt(l.Model.Data, "row_count") < 2 || rec.FetchInt(l.Model.Data, "column_count") < 2 {
		return nil
	}
	rows := l.HeaderRows()
	firstText, firstBold, firstWritten := 0, 0, 0
	otherText, otherBold := 0, 0
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		if slices.Contains(rows, int(rec.FetchInt(cell, "row"))) {
			continue
		}
		weight := cellWeight(cell)
		switch {
		case rec.FetchInt(cell, "column") > 0:
			otherText += boolCount(weight != weightNone)
			otherBold += boolCount(weight == weightBold)
		case rec.FetchInt(cell, "colspan") == 1:
			firstWritten += boolCount(!blankCell(cell))
			firstText += boolCount(weight != weightNone)
			firstBold += boolCount(weight == weightBold)
		}
	}
	if firstText == 0 || firstBold != firstText || firstText*2 < firstWritten || otherText == 0 || otherBold*2 >= otherText {
		return nil
	}
	return []int{0}
}
