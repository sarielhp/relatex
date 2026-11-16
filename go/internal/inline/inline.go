// Package inline is the Go port of Relatex::PaperInline: a single semantic inline walk for
// ordinary blocks and owned rich table cells.
package inline

import (
	"slices"
	"strconv"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// Flat is PaperInline.flat: the spans of content with each table grid replaced by its
// cells' content, in row and column order (presentation: in display order, a repeated
// header row once per page it prints on).
func Flat(content []any, presentation bool) []rec.Map {
	result := []rec.Map{}
	for _, value := range content {
		span := rec.AsMap(value)
		if span["type"] != "table_grid" {
			result = append(result, span)
			continue
		}
		for _, cell := range Cells(span, presentation) {
			result = append(result, Flat(rec.FetchList(cell, "content"), presentation)...)
		}
	}
	return result
}

// Cells are a table grid span's cells, as Flat walks them: RichTable#rows flattened, or
// with presentation and a rich grid RichTableLayout#display_rows' cells.
func Cells(span rec.Map, presentation bool) []rec.Map {
	grid := rec.FetchMap(span, "grid")
	rows := tablemodel.New(grid).Rows()
	if presentation && grid["schema_version"] == tablemodel.Version {
		rows = displayRows(grid, rows)
	}
	result := []rec.Map{}
	for _, row := range rows {
		result = append(result, row...)
	}
	return result
}

// displayRows is the cells of RichTableLayout#display_rows: each native row a fragment
// prints, in fragment order, or every row when the grid has no native fragments.
func displayRows(grid rec.Map, rows [][]rec.Map) [][]rec.Map {
	fragments, _ := grid["native_fragments"].([]any)
	if len(fragments) == 0 {
		return rows
	}
	result := [][]rec.Map{}
	for _, fragment := range fragments {
		for _, native := range rec.FetchList(rec.AsMap(fragment), "rows") {
			row := rec.FetchInt(rec.AsMap(native), "row")
			if row < 0 || row >= int64(len(rows)) {
				panic(rec.Failure{Message: "index " + itoa(row) + " outside of array bounds: " + itoa(-int64(len(rows))) + "..." +
					itoa(int64(len(rows))), Lookup: true})
			}
			result = append(result, rows[row])
		}
	}
	return result
}

// OfType are the spans of one type in a content list, those in the cells of its table grids too, in
// the order the lists hold them (cells as the grid lists them, nested tables included).
func OfType(content []any, kind string) []rec.Map {
	return appendOfType(nil, content, kind)
}

func appendOfType(spans []rec.Map, content []any, kind string) []rec.Map {
	for _, value := range content {
		span := rec.AsMap(value)
		switch span["type"] {
		case kind:
			spans = append(spans, span)
		case "table_grid":
			grid, _ := span["grid"].(rec.Map)
			cells, _ := grid["cells"].([]any)
			for _, cell := range cells {
				nested, _ := rec.AsMap(cell)["content"].([]any)
				spans = appendOfType(spans, nested, kind)
			}
		}
	}
	return spans
}

// Deep is PaperInline.deep: every span, table grids included, then the content of each
// rich cell in row and column order, nested tables' cells too. The image an author's QED mark shows (decision 176)
// follows its symbol.
func Deep(content []any) []rec.Map {
	result := []rec.Map{}
	for _, value := range content {
		span := rec.AsMap(value)
		result = append(result, span)
		if mark, ok := span["image"].(rec.Map); ok && span["type"] == "symbol" {
			result = append(result, mark)
		}
		if span["type"] != "table_grid" {
			continue
		}
		grid, _ := span["grid"].(rec.Map)
		if grid["schema_version"] != tablemodel.Version {
			continue
		}
		cells := []rec.Map{}
		for _, cell := range rec.FetchList(grid, "cells") {
			cells = append(cells, rec.AsMap(cell))
		}
		slices.SortStableFunc(cells, compareCells)
		for _, cell := range cells {
			inner, _ := cell["content"].([]any)
			result = append(result, Deep(inner)...)
		}
	}
	return result
}

func compareCells(a, b rec.Map) int {
	for _, key := range []string{"row", "column"} {
		left, _ := rec.Number(a[key])
		right, _ := rec.Number(b[key])
		if left != right {
			if left < right {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Contents is the content list of spans.
func Contents(spans []rec.Map) []any {
	result := make([]any, len(spans))
	for index, span := range spans {
		result[index] = span
	}
	return result
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }
