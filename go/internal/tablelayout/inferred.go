package tablelayout

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Two inferred header rows that follow from WCAG table semantics (decision 167, amendment 3), read
// only when no rule, bold or span cue marked a header: the empty corner over labelled columns, and
// a row of words over a numeric body. Both name the first row only.
const (
	HeaderCorner  = "corner"
	HeaderNumeric = "numeric"
)

// cornerHeaderRows: the top-left cell is empty, every other cell of the first row is written, and
// at least half of the cells below it in the first column are written: the empty corner sits where
// row labels meet column labels, so the first row labels the columns.
func (l *Layout) cornerHeaderRows() []int {
	if rec.FetchInt(l.Model.Data, "row_count") < 2 || rec.FetchInt(l.Model.Data, "column_count") < 3 {
		return nil
	}
	corner, labelled, below, named := false, 0, 0, 0
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		row, column := rec.FetchInt(cell, "row"), rec.FetchInt(cell, "column")
		switch {
		case row == 0 && column == 0:
			corner = blankCell(cell)
		case row == 0 && blankCell(cell):
			return nil
		case row == 0:
			labelled++
		case column == 0:
			below++
			named += boolCount(!blankCell(cell))
		}
	}
	if !corner || labelled < 2 || named*2 < below {
		return nil
	}
	return []int{0}
}

// numericHeaderRows: every cell of the first row is words (text with a letter, no number) spanning one
// row and one column (a spanned head is a multi-row head no rule here reads), two or more of them, and most of the written body cells are numbers (at least two).
func (l *Layout) numericHeaderRows() []int {
	if rec.FetchInt(l.Model.Data, "row_count") < 2 || rec.FetchInt(l.Model.Data, "column_count") < 2 {
		return nil
	}
	words, numbers, written := 0, 0, 0
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		if rec.FetchInt(cell, "row") == 0 {
			if !wordsCell(cell) || rec.FetchInt(cell, "colspan") > 1 || rec.FetchInt(cell, "rowspan") > 1 {
				return nil
			}
			words++
			continue
		}
		written += boolCount(!blankCell(cell))
		numbers += boolCount(numberCell(cell))
	}
	if words < 2 || numbers < 2 || numbers*2 <= written {
		return nil
	}
	return []int{0}
}

// cellText is the cell's text when it holds text spans only; ok is false for any other content.
func cellText(cell rec.Map) (text string, ok bool) {
	var builder strings.Builder
	for _, value := range rec.ListOf(cell["content"]) {
		span := rec.AsMap(value)
		if span["type"] != "text" {
			return "", false
		}
		part, _ := span["text"].(string)
		builder.WriteString(part)
	}
	return strings.TrimSpace(builder.String()), true
}

var (
	letterPattern = regexp.MustCompile(`\p{L}`)
	numberPattern = regexp.MustCompile(`^[+\-−–±∓]?\s*[$€£]?\s*\d[\d.,\s]*(\s*[±∓]\s*\d[\d.,\s]*)?\s*%?[*†‡]*$`)
)

// wordsCell: the cell is text with a letter in it and is no number.
func wordsCell(cell rec.Map) bool {
	text, ok := cellText(cell)
	return ok && letterPattern.MatchString(text) && !numberPattern.MatchString(text)
}

// numberCell: the cell's text is one number (sign, currency, decimals, a plus-minus, a percent).
func numberCell(cell rec.Map) bool {
	text, ok := cellText(cell)
	return ok && numberPattern.MatchString(text)
}
