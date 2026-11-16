package tablelayout

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stylemodel"
)

// A bold first row (decision 111, the author's decision 2): a table without a declared head or a
// rule that sets one apart, whose first row is visually its header: every cell of it with text
// sets all of its text bold, most of its written cells have text (not formulas alone), and fewer
// than half of the body's text cells are bold. The row is then the header row, an inference the
// accessibility report records (HeaderBold). The acceptance reads the same rule on its own
// (lib/relatex/table_header_rows.rb).

// boldHeaderRows is [0] when the first row is a bold header over a body that is not bold.
func (l *Layout) boldHeaderRows() []int {
	if rec.FetchInt(l.Model.Data, "row_count") < 2 || rec.FetchInt(l.Model.Data, "column_count") < 2 {
		return nil
	}
	headText, headBold, headWritten := 0, 0, 0
	bodyText, bodyBold := 0, 0
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		weight := cellWeight(cell)
		if rec.FetchInt(cell, "row") != 0 {
			bodyText += boolCount(weight != weightNone)
			bodyBold += boolCount(weight == weightBold)
			continue
		}
		headWritten += boolCount(!blankCell(cell))
		headText += boolCount(weight != weightNone)
		headBold += boolCount(weight == weightBold)
	}
	if headText == 0 || headBold != headText || headText*2 < headWritten || bodyText == 0 || bodyBold*2 >= bodyText {
		return nil
	}
	return []int{0}
}

// The weight of a cell's text: none (no written text), bold (all of it bold) or regular.
const (
	weightNone = iota
	weightBold
	weightRegular
)

func cellWeight(cell rec.Map) int {
	weight := weightNone
	for _, value := range rec.ListOf(cell["content"]) {
		span := rec.AsMap(value)
		text, _ := span["text"].(string)
		if span["type"] != "text" || strings.TrimSpace(text) == "" {
			continue
		}
		if !boldSeries(span) {
			return weightRegular
		}
		weight = weightBold
	}
	return weight
}

// boldWeight is the least weight that reads as bold (semibold, NFSS sb).
const boldWeight = 600

// boldSeries: the span's font series is a bold one (sb, b, bx, eb, ub and their widths).
func boldSeries(span rec.Map) bool {
	style, _ := span["style"].(rec.Map)
	series, _ := style["series"].(string)
	return stylemodel.Weights[series] >= boldWeight
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
