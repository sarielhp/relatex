package tables

import (
	"math"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// Rows of a box in a cell (tabbing's rows): the reading put a line break span before each later
// row (tablemodel.LineIndentKey in its origin holds the row's indent). A cell that wraps its lines
// (a p, m or b column) reads the rows as the words of its paragraph, as it always did: the
// breaks go and the text they split joins again. A cell that does not wrap keeps them, and
// each row keeps its indent as the em spaces it is wide (a \quad tab stop is one em).

// defaultFontSize is a 10 pt font's size in sp, the indent unit of a cell whose text carries no style.
const defaultFontSize = 655360

// emSpace is one em of indent.
const emSpace = " "

// rowLines is the cell's content with its row breaks read for the cell's kind.
func (e *rich) rowLines(cell any, content []rec.Map) []rec.Map {
	marked := slices.ContainsFunc(content, func(span rec.Map) bool { return e.rowBreak(span) })
	if !marked {
		return content
	}
	if slices.Contains([]string{"p", "m", "b"}, rec.FetchString(e.cell(cell), "kind")) {
		return e.unbroken(content)
	}
	size := fontSize(content)
	result := make([]rec.Map, 0, len(content)+4)
	indent, broken := int64(0), false
	for _, span := range content {
		if e.rowBreak(span) {
			if len(result) > 0 {
				result = append(result, span)
				indent, broken = rec.AsInt(e.origin(span)[tablemodel.LineIndentKey]), true
			}
			continue
		}
		if broken {
			result = indented(result, span, indent, size)
		} else {
			result = append(result, span)
		}
		indent, broken = 0, false
	}
	return result
}

// rowBreak: the span is a row break of the reading.
func (e *rich) rowBreak(span rec.Map) bool {
	_, ok := e.origin(span)[tablemodel.LineIndentKey]
	return ok && span["line_break"] == true
}

// indented appends span after the em spaces that make up its row's indent.
func indented(result []rec.Map, span rec.Map, indent int64, size float64) []rec.Map {
	count := int(math.Round(float64(indent) / size))
	if span["type"] == "text" {
		span = copyMap(span)
		span["text"] = strings.TrimLeft(rec.FetchString(span, "text"), " ")
	}
	if count > 0 {
		result = append(result, rec.Map{"type": "text", "text": strings.Repeat(emSpace, count)})
	}
	return append(result, span)
}

// fontSize is the size of the first styled text of a cell, in sp.
func fontSize(content []rec.Map) float64 {
	for _, span := range content {
		style, _ := span["style"].(rec.Map)
		if size, ok := rec.Number(style["size_sp"]); ok && size > 0 {
			return size
		}
	}
	return defaultFontSize
}

// unbroken is the content of a wrapping cell without its row breaks: the text a break split
// joins again, as the reading joins the glyph runs of one line.
func (e *rich) unbroken(content []rec.Map) []rec.Map {
	result := []rec.Map{}
	split := false
	for _, span := range content {
		if e.rowBreak(span) {
			split = true
			continue
		}
		if last := len(result) - 1; split && last >= 0 && joinable(result[last], span) {
			joined := copyMap(result[last])
			joined["text"] = rec.FetchString(joined, "text") + rec.FetchString(span, "text")
			result[last] = joined
		} else {
			result = append(result, span)
		}
		split = false
	}
	return result
}

func joinable(left, right rec.Map) bool {
	if left["type"] != "text" || right["type"] != "text" {
		return false
	}
	return rec.Equal(except(left, "text"), except(right, "text")) && tablemodel.Inspect(left["line_break"]) == tablemodel.Inspect(right["line_break"])
}
