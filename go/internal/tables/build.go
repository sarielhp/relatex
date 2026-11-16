package tables

import (
	"fmt"
	"math"
	"slices"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// buildGrid is the rich grid of a table: its executed cells in logical rows, with the
// columns its cells were observed to start and end at.
func (e *rich) buildGrid(table any, rows []rec.Map, content map[string][]rec.Map, order []string) rec.Map {
	logical := e.logicalRows(rows)
	entries := []rec.Map{}
	for _, row := range rows {
		entries = append(entries, maps(tablemodel.FetchList(row, "cells"))...)
	}
	extents := []any{}
	anchors := []rec.Map{}
	anchorIDs := []any{}
	for _, entry := range entries {
		cell := e.cell(rec.Fetch(entry, "cell"))
		extents = append(extents, add(sub(rec.Fetch(cell, "column"), int64(1)), rec.Fetch(cell, "colspan")))
		if !containsEql(anchorIDs, entry["cell"]) {
			anchorIDs = append(anchorIDs, entry["cell"])
			anchors = append(anchors, entry)
		}
	}
	columnCount := maximum(extents)
	// Content of a cell outside the captured rows would be dropped.
	for _, id := range order {
		if !slices.ContainsFunc(anchorIDs, func(anchor any) bool { return key(anchor) == id }) {
			rec.Fail("Rich table content outside captured rows")
		}
	}
	e.checkRows(rows)
	cells := []rec.Map{}
	fills := e.cellFills(table)
	for _, entry := range anchors {
		cells = append(cells, withFill(e.gridCell(entry, logical, content), fills))
	}
	// A \multirow may reach past the table's last row; it spans the rows that exist.
	for _, cell := range cells {
		room := sub(int64(len(logical)), rec.Fetch(cell, "row"))
		if number(rec.Fetch(cell, "rowspan")) > number(room) {
			cell["rowspan"] = room
			e.clamped = append(e.clamped, table)
		}
	}
	boundaries := e.columnBoundaries(anchors, rec.AsInt(columnCount))
	fragments := e.fragments(table, rows, logical)
	cutAtInterruptions(cells, e.interruptions(rows, logical))
	cutAtOccupiedRows(cells)
	cutAtCrossingRules(cells, fragments, boundaries)
	cells = collapseCoveredCells(cells)
	// A cell followed by stretched column glue spans that glue as part of its column.
	for _, cell := range cells {
		column, colspan := fetchInt(cell, "column"), fetchInt(cell, "colspan")
		spanned := boundaries[column+colspan] - boundaries[column]
		if number(sub(spanned, rec.Fetch(cell, "width_sp"))) > 4 {
			cell["width_sp"] = spanned
		}
	}
	cells = append(cells, missingCells(cells, int64(len(logical)), boundaries)...)
	cells, boundaries = dropHiddenColumns(cells, boundaries)
	columnCount = int64(len(boundaries) - 1)
	sections := e.sections(rows, logical)
	widths := []any{}
	for index := 1; index < len(boundaries); index++ {
		widths = append(widths, boundaries[index]-boundaries[index-1])
	}
	section := func(name string) []any { return rec.AsList(fetchDefault(sections, name, []any{})) }
	grid := rec.Map{"schema_version": tablemodel.Version, "row_count": int64(len(logical)), "column_count": columnCount,
		"cells": anyList(cells), "column_widths_sp": widths, "width_sp": boundaries[len(boundaries)-1] - boundaries[0],
		"native_fragments": fragments, "native_table": table, "source_version": "tables-0.2",
		"sections": sections, "native_preamble": rec.Fetch(rows[0], "preamble"), "body_rows": section("body"),
		"header_rows": tablemodel.SortValues(append(slices.Clone(section("first_head")), section("repeat_head")...)),
		"footer_rows": tablemodel.SortValues(append(slices.Clone(section("repeat_foot")), section("last_foot")...))}
	if declared := e.declaredHeaderRows(table, len(logical)); len(declared) > 0 {
		grid["declared_header_rows"] = declared
	}
	return grid
}

// interruptions are the logical body rows after which the printed table breaks off for rows that
// are not body rows (a longtable's foot and repeated head at a page break): the last body row
// before each such interruption, as an index into logical.
func (e *rich) interruptions(rows []rec.Map, logical []any) []int64 {
	cuts := []int64{}
	previous, interrupted := int64(-1), false
	for _, row := range rows {
		index := int64(slices.IndexFunc(logical, func(number any) bool { return rec.Equal(number, rec.Fetch(row, "logical_row")) }))
		if e.role(e.firstCell(rows, logical[index])) != "body" {
			interrupted = interrupted || previous >= 0
			continue
		}
		if interrupted && index > previous {
			cuts = append(cuts, previous)
		}
		previous, interrupted = index, false
	}
	return cuts
}

// cutAtInterruptions ends a \multirow cell at the first interruption inside its span: a table
// cannot hold its rows across the foot and head a page break prints between them, and the PDF
// prints a multirow's text once, on its first page. The rows it no longer spans keep their own
// (empty) cells.
func cutAtInterruptions(cells []rec.Map, cuts []int64) {
	for _, cell := range cells {
		row, span := fetchInt(cell, "row"), fetchInt(cell, "rowspan")
		for _, cut := range cuts {
			if row <= cut && cut < row+span-1 {
				cell["rowspan"] = cut - row + 1
				break
			}
		}
	}
}

// cutAtOccupiedRows ends a \multirow cell before the first row inside its span whose own cell
// in those columns holds content. \multirow reserves nothing: its text is lowered into the rows
// below, and a later row that fills the column (a \multirow{2}{*}{} header above a body row, a
// \multirow{3} whose section has two rows) prints its own text there as well. The grid keeps
// both by letting the multirow span only the rows it leaves free.
func cutAtOccupiedRows(cells []rec.Map) {
	for _, cell := range cells {
		row, span := fetchInt(cell, "row"), fetchInt(cell, "rowspan")
		first, last := fetchInt(cell, "column"), fetchInt(cell, "column")+fetchInt(cell, "colspan")
		for _, other := range cells {
			y, x := fetchInt(other, "row"), fetchInt(other, "column")
			if y <= row || y >= row+span || x+fetchInt(other, "colspan") <= first || x >= last || !hasContent(other) {
				continue
			}
			span = y - row
		}
		cell["rowspan"] = span
	}
}

// hasContent: a cell holds something besides blank text.
func hasContent(cell rec.Map) bool {
	for _, span := range maps(tablemodel.FetchList(cell, "content")) {
		if span["type"] != "text" || rec.Strip(rec.FetchString(span, "text")) != "" {
			return true
		}
	}
	return false
}

// logicalRows is the table's logical row numbers, in section order then by number.
func (e *rich) logicalRows(rows []rec.Map) []any {
	logical := []any{}
	for _, row := range rows {
		if number := rec.Fetch(row, "logical_row"); !containsEql(logical, number) {
			logical = append(logical, number)
		}
	}
	ranks := map[string]int{}
	for _, number := range logical {
		ranks[key(number)] = slices.Index(tablemodel.SectionRoles, e.role(e.firstCell(rows, number)))
	}
	slices.SortStableFunc(logical, func(a, b any) int {
		if order := ranks[key(a)] - ranks[key(b)]; order != 0 {
			return order
		}
		return compareNumbers(a, b)
	})
	return logical
}

// firstCell is the cell id of the first entry of the first native row of a logical row.
func (e *rich) firstCell(rows []rec.Map, number any) any {
	for _, row := range rows {
		if rec.Equal(rec.Fetch(row, "logical_row"), number) {
			return rec.Fetch(rec.AsMap(rec.Index(tablemodel.FetchList(row, "cells"), 0)), "cell")
		}
	}
	panic(fmt.Sprintf("tables: undefined method 'fetch' for nil (logical row %s)", tablemodel.Inspect(number)))
}

func (e *rich) checkRows(rows []rec.Map) {
	for _, row := range rows {
		roles := []string{}
		for _, entry := range maps(tablemodel.FetchList(row, "cells")) {
			if !rec.Equal(rec.Fetch(e.cell(rec.Fetch(entry, "cell")), "row"), rec.Fetch(row, "logical_row")) {
				rec.Fail("Native row differs from executed cell row")
			}
		}
		for _, entry := range maps(tablemodel.FetchList(row, "cells")) {
			if role := e.role(rec.Fetch(entry, "cell")); !slices.Contains(roles, role) {
				roles = append(roles, role)
			}
		}
		if len(roles) != 1 {
			rec.Fail("Native row mixes table section roles")
		}
	}
}

func (e *rich) gridCell(entry rec.Map, logical []any, content map[string][]rec.Map) rec.Map {
	declaration := e.cell(rec.Fetch(entry, "cell"))
	kind := rec.FetchString(declaration, "kind")
	id := rec.Fetch(declaration, "id")
	vertical := map[string]any{"p": "top", "m": "middle", "b": "bottom"}[kind]
	if vertical == nil {
		vertical = rec.Fetch(declaration, "vertical_align")
	}
	var row any
	for index, number := range logical {
		if rec.Equal(number, rec.Fetch(declaration, "row")) {
			row = int64(index)
			break
		}
	}
	cellContent := []any{}
	if spans, ok := content[key(id)]; ok {
		cellContent = anyList(spans)
	}
	return rec.Map{"id": "cell-" + tablemodel.Inspect(id), "row": row, "column": sub(rec.Fetch(declaration, "column"), int64(1)),
		"rowspan": rec.Fetch(declaration, "rowspan"), "colspan": rec.Fetch(declaration, "colspan"), "alignment": alignments[kind],
		"vertical_align": vertical, "width_sp": wholeSP(rec.Fetch(entry, "width")), "height_sp": wholeSP(rec.Fetch(entry, "height")),
		"depth_sp": wholeSP(rec.Fetch(entry, "depth")), "source_cell": id, "wrap": kind == "p" || kind == "m" || kind == "b",
		"padding_sp": rec.Fetch(declaration, "padding_sp"), "role": e.role(id), "content": cellContent}
}

// wholeSP is a measured size as whole scaled points: stretched glue (tabularx, \extracolsep{\fill})
// leaves a cell a fractional width, and the grid schema holds integers.
func wholeSP(value any) any {
	if fraction, ok := value.(canonjson.Float); ok {
		return int64(math.Round(float64(fraction)))
	}
	return value
}

// sections is each section role's logical row indices, sorted.
func (e *rich) sections(rows []rec.Map, logical []any) rec.Map {
	result := rec.Map{}
	for index, number := range logical {
		role := e.role(e.firstCell(rows, number))
		list, _ := result[role].([]any)
		result[role] = append(list, int64(index))
	}
	for role, indices := range result {
		result[role] = tablemodel.SortValues(rec.AsList(indices))
	}
	return result
}

// collapseCoveredCells drops the placeholder cells inside another cell's span.
func collapseCoveredCells(cells []rec.Map) []rec.Map {
	sorted := slices.Clone(cells)
	slices.SortStableFunc(sorted, func(a, b rec.Map) int {
		return compareLists(values(a, "row", "column"), values(b, "row", "column"))
	})
	occupied := map[string]bool{}
	result := []rec.Map{}
	for _, cell := range sorted {
		if occupied[key(cell["row"], cell["column"])] {
			if hasContent(cell) {
				rec.Fail("Spanned placeholder contains independent content")
			}
			continue
		}
		row, column := fetchInt(cell, "row"), fetchInt(cell, "column")
		for y := row; y < row+fetchInt(cell, "rowspan"); y++ {
			for x := column; x < column+fetchInt(cell, "colspan"); x++ {
				if occupied[key(y, x)] {
					rec.Fail("Overlapping executed rich table spans")
				}
				occupied[key(y, x)] = true
			}
		}
		result = append(result, cell)
	}
	return result
}

// missingCells: a row may end before the last column (a & b \\ in a three-column table);
// its remaining positions are empty cells with no native counterpart.
func missingCells(cells []rec.Map, rowCount int64, boundaries []int64) []rec.Map {
	columns := int64(len(boundaries) - 1)
	occupied := map[string]bool{}
	for _, cell := range cells {
		row, column := fetchInt(cell, "row"), fetchInt(cell, "column")
		for y := row; y < row+fetchInt(cell, "rowspan"); y++ {
			for x := column; x < column+fetchInt(cell, "colspan"); x++ {
				occupied[key(y, x)] = true
			}
		}
	}
	result := []rec.Map{}
	for y := range rowCount {
		for x := range columns {
			if occupied[key(y, x)] {
				continue
			}
			result = append(result, emptyCell(cells, y, x, boundaries))
		}
	}
	return result
}

func emptyCell(cells []rec.Map, y, x int64, boundaries []int64) rec.Map {
	row := []rec.Map{}
	for _, cell := range cells {
		if rec.Equal(cell["row"], y) && rec.Equal(cell["rowspan"], int64(1)) {
			row = append(row, cell)
		}
	}
	role := any("body")
	if len(row) > 0 {
		if value := rec.Fetch(row[0], "role"); truthy(value) {
			role = value
		}
	}
	heights, depths := []any{}, []any{}
	for _, cell := range row {
		heights = append(heights, rec.Fetch(cell, "height_sp"))
		depths = append(depths, rec.Fetch(cell, "depth_sp"))
	}
	orZero := func(value any) any {
		if value == nil {
			return int64(0)
		}
		return value
	}
	return rec.Map{"id": fmt.Sprintf("cell-empty-%d-%d", y, x), "row": y, "column": x, "rowspan": int64(1), "colspan": int64(1),
		"alignment": "l", "vertical_align": "top", "width_sp": max(boundaries[x+1]-boundaries[x], 1),
		"height_sp": orZero(maximum(heights)), "depth_sp": orZero(maximum(depths)),
		"wrap": false, "padding_sp": int64(0), "role": role, "content": []any{}}
}

// columnBoundaries: a boundary is where the cells of a column start; stretched glue between
// columns (tabular* with \extracolsep{\fill}) belongs to the column before it. The last
// boundary, or one no cell starts at, is where cells end.
func (e *rich) columnBoundaries(anchors []rec.Map, columns int64) []int64 {
	starts := map[int64][]any{}
	ends := map[int64][]any{}
	for _, entry := range anchors {
		cell := e.cell(rec.Fetch(entry, "cell"))
		column := fetchInt(cell, "column") - 1
		starts[column] = append(starts[column], rec.Fetch(entry, "left"))
		end := column + fetchInt(cell, "colspan")
		ends[end] = append(ends[end], add(rec.Fetch(entry, "left"), rec.Fetch(entry, "width")))
	}
	result := make([]int64, 0, columns+1)
	for column := int64(0); column <= columns; column++ {
		observed, started := starts[column]
		if column >= columns || !started {
			observed = ends[column]
		}
		if len(observed) == 0 {
			rec.Fail("Unobserved native column boundary")
		}
		if number(maximum(observed))-number(minimum(observed)) > 2 {
			rec.Fail("Inconsistent native column boundary")
		}
		result = append(result, fdivRound(tablemodel.Sum(observed), len(observed)))
	}
	return orderedBoundaries(result, starts)
}

// orderedBoundaries keeps the boundaries strictly increasing. A declared column no cell starts
// in, crossed only by wider spans, can have no width at all (deluxetable's {cccccccc} over seven
// filled columns; fifth cohort 1210): it measures one scaled point, as dropHiddenColumns expects.
func orderedBoundaries(result []int64, starts map[int64][]any) []int64 {
	for index := 1; index < len(result); index++ {
		_, started := starts[int64(index-1)]
		if result[index-1] == result[index] && !started {
			result[index]++
		}
		if result[index-1] >= result[index] {
			rec.Fail("Invalid native column order")
		}
	}
	return result
}

// fragments is the table's native rows, glyphs and rules, page by page.
func (e *rich) fragments(table any, rows []rec.Map, logical []any) []any {
	byPage := map[string][]rec.Map{}
	pages := []any{}
	for _, row := range rows {
		page := rec.Fetch(row, "page")
		if _, ok := byPage[key(page)]; !ok {
			pages = append(pages, page)
		}
		byPage[key(page)] = append(byPage[key(page)], row)
	}
	result := []any{}
	for _, page := range pages {
		glyphs, rules := []any{}, []any{}
		for _, event := range e.events {
			if !rec.Equal(values(event, "table", "page"), []any{table, page}) {
				continue
			}
			switch event["code"] {
			case "rich_table_rule":
				rules = append(rules, except(event, "seq", "kind", "code", "source", "table", "page", "cell", "color"))
			case "rich_table_glyphs":
				glyphs = append(glyphs, tablemodel.FetchList(event, "glyphs")...)
			}
		}
		entries := []any{}
		for _, entry := range byPage[key(page)] {
			merged := copyMap(entry)
			merged["row"] = nil
			for index, number := range logical {
				if rec.Equal(number, rec.Fetch(entry, "logical_row")) {
					merged["row"] = int64(index)
					break
				}
			}
			entries = append(entries, merged)
		}
		result = append(result, rec.Map{"page": page, "rows": entries, "glyphs": glyphs, "rules": rules})
	}
	return result
}
