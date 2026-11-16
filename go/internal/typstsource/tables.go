package typstsource

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

func intField(cell rec.Map, name string) int {
	number, ok := rec.Number(rec.Fetch(cell, name))
	if !ok {
		rec.Fail("expected a number, got %T", cell[name])
	}
	return int(number)
}

func (e *exporter) table(grid rec.Map) string {
	columns := intField(grid, "column_count")
	cells := maps(rec.Fetch(grid, "cells"))
	sort.SliceStable(cells, func(a, b int) bool {
		if intField(cells[a], "row") != intField(cells[b], "row") {
			return intField(cells[a], "row") < intField(cells[b], "row")
		}
		return intField(cells[a], "column") < intField(cells[b], "column")
	})
	header := len(list(grid["header_rows"]))
	rows := e.tableRows(cells, columns, header)
	return fmt.Sprintf("#table(\n  columns: %d, align: (%s,), stroke: none,\n  table.hline(),\n  %s\n  table.hline(),\n)", columns,
		strings.Join(tableAlignments(cells, columns), ", "), strings.Join(rows, "\n  "))
}

func tableAlignments(cells []rec.Map, columns int) []string {
	result := make([]string, columns)
	for column := range columns {
		var cell rec.Map
		for _, item := range cells {
			if intField(item, "column") == column && intField(item, "colspan") == 1 && item["role"] != "header" {
				cell = item
				break
			}
		}
		if cell == nil {
			for _, item := range cells {
				if intField(item, "column") == column {
					cell = item
					break
				}
			}
		}
		alignment := prefix(toS(cell["alignment"]), 1)
		if cell == nil {
			alignment = ""
		}
		found, ok := cellAlignments[alignment]
		if !ok {
			found = "left"
		}
		result[column] = found
	}
	return result
}

// tableRows: a short row is padded: Typst would flow the next row's cells into it.
func (e *exporter) tableRows(cells []rec.Map, columns, header int) []string {
	covered := map[int]int{}
	groups := map[int][]rec.Map{}
	order := []int{}
	for _, cell := range cells {
		row := intField(cell, "row")
		if _, ok := groups[row]; !ok {
			order = append(order, row)
		}
		groups[row] = append(groups[row], cell)
	}
	sort.Ints(order)
	lines := []string{}
	for _, row := range order {
		used := covered[row]
		for _, cell := range groups[row] {
			used += intField(cell, "colspan")
		}
		for _, cell := range groups[row] {
			for offset := 1; offset < intField(cell, "rowspan"); offset++ {
				covered[row+offset] += intField(cell, "colspan")
			}
		}
		parts := []string{}
		for _, cell := range groups[row] {
			parts = append(parts, e.tableCell(cell))
		}
		for range max(columns-used, 0) {
			parts = append(parts, "[]")
		}
		line := strings.Join(parts, ", ")
		if row == header && header > 0 {
			lines = append(lines, "table.hline(),\n  "+line+",")
		} else {
			lines = append(lines, line+",")
		}
	}
	return lines
}

func (e *exporter) tableCell(cell rec.Map) string {
	text := e.nested(func() string { return rec.Strip(e.inline(content(cell), cell, 0, false)) })
	spans := []string{}
	if colspan := intField(cell, "colspan"); colspan > 1 {
		spans = append(spans, fmt.Sprintf("colspan: %d", colspan))
	}
	if rowspan := intField(cell, "rowspan"); rowspan > 1 {
		spans = append(spans, fmt.Sprintf("rowspan: %d", rowspan))
	}
	if len(spans) == 0 {
		return "[" + text + "]"
	}
	return "table.cell(" + strings.Join(spans, ", ") + ")[" + text + "]"
}
