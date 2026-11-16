// Package tablefixture builds small rich table grids for tests: cells of text, regular or bold,
// with spans, and native rules placed where TeX would draw them (\toprule, \midrule, \hline, a
// vertical rule of the preamble). The grids pass the rich table contract and read through
// tablelayout as a converted table does, so header rules can be tested on tables made for them.
package tablefixture

import (
	"fmt"

	"github.com/sarielhp/relatex/internal/rec"
)

// Cell is one cell as a row lists it: its text (none for an empty cell), whether it is bold, and
// its column and row spans (0 means 1).
type Cell struct {
	Text    string
	Math    bool // the cell holds a formula (a math span), not Text
	Bold    bool
	Colspan int
	Rowspan int
}

// Rule is a native rule. A horizontal rule lies at row boundary At (0 above the first row) across
// the table; a vertical one at column boundary At (0 left of the first column) down the rows
// From … To-1 (To 0 means to the last row).
type Rule struct {
	Vertical  bool
	At        int
	Thickness int64
	From, To  int
}

// Rule thicknesses in sp: booktabs' heavy (\toprule, \bottomrule, 0.08em) and light (\midrule,
// 0.05em) rules and \arrayrulewidth (0.4pt).
const (
	Heavy int64 = 52_429
	Light int64 = 32_768
	Array int64 = 26_214
)

// Geometry of every fixture: column width, row height and depth, the table's origin on the page.
const (
	columnWidth int64 = 2_000_000
	rowHeight   int64 = 500_000
	rowDepth    int64 = 200_000
	originX     int64 = 1_000_000
	originY     int64 = 10_000_000
)

// Grid builds a rich grid of rows of cells (a cell under a row span above is left out of its row, as
// in LaTeX) with the rules given. It panics on a malformed layout.
func Grid(rows [][]Cell, rules []Rule) rec.Map {
	columns := width(rows)
	occupied := make([][]bool, len(rows))
	for index := range occupied {
		occupied[index] = make([]bool, columns)
	}
	cells, natives := []any{}, []any{}
	for row, list := range rows {
		column, entries := 0, []any{}
		for _, spec := range list {
			for column < columns && occupied[row][column] {
				column++
			}
			cell := gridCell(spec, row, column, len(cells)+1)
			occupy(occupied, cell)
			cells = append(cells, cell)
			entries = append(entries, rec.Map{"cell": cell["source_cell"], "left": int64(column) * columnWidth})
			column += int(rec.AsInt(cell["colspan"]))
		}
		natives = append(natives, rec.Map{"row": int64(row), "origin_sp": []any{float64(originX), float64(top(row))},
			"height": rowHeight, "depth": rowDepth, "cells": entries})
	}
	widths := make([]any, columns)
	for index := range widths {
		widths[index] = columnWidth
	}
	return rec.Map{"schema_version": "table-2", "row_count": int64(len(rows)), "column_count": int64(columns),
		"column_widths_sp": widths, "width_sp": int64(columns) * columnWidth, "cells": cells,
		"native_fragments": []any{rec.Map{"page": int64(1), "rows": natives, "rules": nativeRules(rules, len(rows), columns)}}}
}

// Span is a table_grid span holding the grid, as a paragraph's content lists it.
func Span(id string, grid rec.Map) rec.Map {
	return rec.Map{"type": "table_grid", "id": id, "grid": grid}
}

func width(rows [][]Cell) int {
	covered := map[int]int{}
	columns := 0
	for row, list := range rows {
		count := covered[row]
		for _, spec := range list {
			span := max(spec.Colspan, 1)
			count += span
			for below := row + 1; below < row+max(spec.Rowspan, 1); below++ {
				covered[below] += span
			}
		}
		columns = max(columns, count)
	}
	return columns
}

func occupy(occupied [][]bool, cell rec.Map) {
	row, column := int(rec.AsInt(cell["row"])), int(rec.AsInt(cell["column"]))
	for y := row; y < row+int(rec.AsInt(cell["rowspan"])); y++ {
		for x := column; x < column+int(rec.AsInt(cell["colspan"])); x++ {
			if y >= len(occupied) || x >= len(occupied[y]) || occupied[y][x] {
				panic(fmt.Sprintf("tablefixture: cell at %d,%d overlaps or leaves the table", y, x))
			}
			occupied[y][x] = true
		}
	}
}

func gridCell(spec Cell, row, column, source int) rec.Map {
	colspan, rowspan := int64(max(spec.Colspan, 1)), int64(max(spec.Rowspan, 1))
	content := []any{}
	if spec.Math {
		content = append(content, rec.Map{"type": "math", "node": fmt.Sprintf("n-fixture-%d-%d", row, column)})
	} else if spec.Text != "" {
		content = append(content, rec.Map{"type": "text", "text": spec.Text, "style": style(spec.Bold)})
	}
	return rec.Map{"id": fmt.Sprintf("c%d-%d", row, column), "row": int64(row), "column": int64(column),
		"rowspan": rowspan, "colspan": colspan, "content": content, "role": "body", "alignment": "l",
		"vertical_align": "top", "padding_sp": int64(0), "wrap": false, "source_cell": int64(source),
		"width_sp": colspan * columnWidth, "height_sp": rowHeight, "depth_sp": rowDepth}
}

// style is Computer Modern's roman, medium or bold extended.
func style(bold bool) rec.Map {
	font, series := "cmr10", "m"
	if bold {
		font, series = "cmbx10", "bx"
	}
	return rec.Map{"source": "nfss-font-id", "font_id": int64(1), "font_name": font, "size_sp": int64(655_360),
		"encoding": "OT1", "family": "cmr", "series": series, "shape": "n", "generic": "serif"}
}

func top(row int) int64 { return originY + int64(row)*(rowHeight+rowDepth) }

func nativeRules(rules []Rule, rows, columns int) []any {
	result := []any{}
	for _, rule := range rules {
		var rect []any
		if rule.Vertical {
			last := rule.To
			if last == 0 {
				last = rows
			}
			x := originX + int64(rule.At)*columnWidth - rule.Thickness/2
			rect = []any{float64(x), float64(top(rule.From)), rule.Thickness, top(last) - top(rule.From)}
		} else {
			y := top(rule.At) - rule.Thickness/2
			rect = []any{float64(originX), float64(y), int64(columns) * columnWidth, rule.Thickness}
		}
		result = append(result, rec.Map{"rect_sp": rect, "math": false, "in_row": false})
	}
	return result
}
