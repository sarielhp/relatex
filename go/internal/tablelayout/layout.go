// Package tablelayout is the document model's table layout (RichTable as the views read it and
// RichTableLayout, decision 102): the rule segments attached to cell edges, display rows and the
// column header rows (decision 088), shared by the page writer, the Typst writer, the browser
// checks and the accessibility report.
package tablelayout

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// crash is a Ruby error that is no Relatex::Failure (a NoMethodError): nothing that
// rescues Failures catches it.
func crash(message string) { panic(errors.New(message)) }

// rubyInspect is String#inspect for the identifiers in lookup messages.
func rubyInspect(text string) string {
	encoded, err := canonjson.Marshal(text)
	if err != nil {
		return text
	}
	return string(encoded)
}

// Table is RichTable's model as the views read it: the grid (legacy grids normalized), its
// rows and its occupancy. The grid is validated by tablemodel.NewWith.
type Table struct {
	Data      rec.Map
	Rows      [][]rec.Map
	Occupancy [][]string
}

var cellLetters = regexp.MustCompile(`[lcr]`)

// NewTable is RichTable.new(grid, nodes:): nodes may be nil.
func NewTable(grid, nodes rec.Map) *Table {
	tablemodel.NewWith(grid, nodes)
	data := grid
	if grid["schema_version"] != tablemodel.Version {
		data = legacyGrid(grid)
	}
	t := &Table{Data: data}
	t.Rows = tablemodel.New(data).Rows()
	rows, columns := int(rec.FetchInt(data, "row_count")), int(rec.FetchInt(data, "column_count"))
	t.Occupancy = make([][]string, rows)
	for index := range t.Occupancy {
		t.Occupancy[index] = make([]string, columns)
	}
	for _, value := range rec.FetchList(data, "cells") {
		cell := rec.AsMap(value)
		row, column := int(rec.FetchInt(cell, "row")), int(rec.FetchInt(cell, "column"))
		for y := row; y < row+int(rec.FetchInt(cell, "rowspan")); y++ {
			for x := column; x < column+int(rec.FetchInt(cell, "colspan")); x++ {
				t.Occupancy[y][x] = rec.FetchString(cell, "id")
			}
		}
	}
	return t
}

// legacyGrid is RichTable#legacy_grid.
func legacyGrid(grid rec.Map) rec.Map {
	tablemodel.ValidateShape(grid)
	alignment := cellLetters.FindAllString(rec.FetchString(grid, "preamble"), -1)
	cells := []any{}
	for y, rowValue := range rec.FetchList(grid, "rows") {
		for x, value := range rec.AsList(rowValue) {
			cell := rec.AsMap(value)
			content := tablemodel.LegacyCellContent(cell)
			copied := rec.Map{}
			for key, item := range cell {
				if key != "text" && key != "runs" {
					copied[key] = item
				}
			}
			cells = append(cells, rec.Merge(copied, rec.Map{"id": fmt.Sprintf("r%dc%d", y, x), "row": int64(y), "column": int64(x),
				"rowspan": int64(1), "colspan": int64(1), "alignment": alignment[x], "vertical_align": "top", "content": content}))
		}
	}
	return rec.Map{"schema_version": tablemodel.Version, "row_count": int64(len(rec.FetchList(grid, "rows"))),
		"column_count": int64(len(alignment)), "width_sp": rec.Fetch(grid, "width_sp"), "cells": cells,
		"horizontal_borders": rec.Fetch(grid, "horizontal_borders"), "source_version": "legacy-grid"}
}

// Layout is RichTableLayout: shared rule geometry. Reading renderers attach each segment to
// a semantic cell edge, so wrapped text can grow without detaching the table borders.
type Layout struct {
	Model        *Table
	ColumnWidths []any
	RowHeights   []any
	segments     map[string][]rec.Map
	cells        map[string]rec.Map
	headers      *[]int // the column header rows, once read (HeaderRows)
	columns      *[]int // the row header columns, once read (HeaderColumns)
	columnSource string // how they are known (HeaderColumnSource)
	headerSource string // how they are known (HeaderSource)
}

// cellFields is RichTableLayout::CELL_FIELDS.
var cellFields = []string{"id", "row", "column", "rowspan", "colspan", "alignment", "vertical_align", "padding_sp", "wrap", "role"}

// NewLayout is RichTableLayout.new(model).
func NewLayout(model *Table) *Layout {
	grid := model.Data
	l := &Layout{Model: model, segments: map[string][]rec.Map{}, cells: map[string]rec.Map{}}
	for _, value := range rec.FetchList(grid, "cells") {
		cell := rec.AsMap(value)
		l.cells[rec.FetchString(cell, "id")] = cell
	}
	l.ColumnWidths = columnWidths(grid)
	l.RowHeights = rowHeights(model)
	for _, value := range rec.ListOf(grid["native_fragments"]) {
		l.nativeRules(rec.AsMap(value))
	}
	if grid["source_version"] == "legacy-grid" {
		l.legacyRules()
	}
	l.identifySegments()
	return l
}

func columnWidths(grid rec.Map) []any {
	if widths, ok := grid["column_widths_sp"]; ok {
		return slices.Clone(rec.AsList(widths))
	}
	count := rec.FetchInt(grid, "column_count")
	width := rec.FetchOr(grid, "width_sp", 3_276_800*count)
	result := []any{}
	for range count {
		result = append(result, rubynum.Fdiv(width, count))
	}
	return result
}

// rowHeights: each row's tallest cell (a spanning cell's share), a native row's own height,
// 786432sp (12pt) for an empty row.
func rowHeights(model *Table) []any {
	heights := make([]any, rec.FetchInt(model.Data, "row_count"))
	for row := range heights {
		heights[row] = int64(0)
	}
	for row, cells := range model.Rows {
		var height any
		for _, cell := range cells {
			value := rubynum.Fdiv(rubynum.Add(rec.FetchOr(cell, "height_sp", int64(0)), rec.FetchOr(cell, "depth_sp", int64(0))), rec.Fetch(cell, "rowspan"))
			if height == nil || rubynum.Less(height, value) {
				height = value
			}
		}
		if height != nil {
			heights[row] = height
		}
	}
	for _, value := range rec.ListOf(model.Data["native_fragments"]) {
		for _, row := range rec.ListOf(rec.AsMap(value)["rows"]) {
			native := rec.AsMap(row)
			heights[rec.FetchInt(native, "row")] = rubynum.Add(rec.Fetch(native, "height"), rec.Fetch(native, "depth"))
		}
	}
	for index, value := range heights {
		if !rubynum.Positive(value) {
			heights[index] = int64(786_432)
		}
	}
	return heights
}

// identifySegments drops repeated rules of a cell and names each by its content.
func (l *Layout) identifySegments() {
	for id, rules := range l.segments {
		unique := []rec.Map{}
		for _, rule := range rules {
			if !slices.ContainsFunc(unique, func(seen rec.Map) bool { return reflect.DeepEqual(seen, rule) }) {
				unique = append(unique, rule)
			}
		}
		for _, rule := range unique {
			digest, err := canonjson.Digest(rule)
			if err != nil {
				rec.Fail("%v", err)
			}
			rule["id"] = digest
		}
		l.segments[id] = unique
	}
}

func span(values []any, from, count int) []any {
	if from > len(values) {
		return nil
	}
	return values[from:min(from+count, len(values))]
}

// CellRect is RichTableLayout#cell_rect.
func (l *Layout) CellRect(cell rec.Map) [4]any {
	column, row := int(rec.FetchInt(cell, "column")), int(rec.FetchInt(cell, "row"))
	x := rubynum.Sum(span(l.ColumnWidths, 0, column))
	y := rubynum.Sum(span(l.RowHeights, 0, row))
	width := rubynum.Sum(span(l.ColumnWidths, column, int(rec.FetchInt(cell, "colspan"))))
	height := rubynum.Sum(span(l.RowHeights, row, int(rec.FetchInt(cell, "rowspan"))))
	return [4]any{x, y, rubynum.Add(x, width), rubynum.Add(y, height)}
}

// RulesFor is RichTableLayout#rules_for.
func (l *Layout) RulesFor(cell rec.Map) []rec.Map { return l.segments[rec.FetchString(cell, "id")] }

// BrowserGrid is RichTableLayout#browser_grid without its cells.
func (l *Layout) BrowserGrid() rec.Map {
	data := l.Model.Data
	result := rec.Map{"schema_version": tablemodel.Version, "row_count": rec.Fetch(data, "row_count"),
		"column_count": rec.Fetch(data, "column_count"), "width_sp": rubynum.Sum(l.ColumnWidths),
		"column_widths_sp": l.ColumnWidths, "row_heights_sp": l.RowHeights}
	for _, key := range []string{"sections", "body_rows", "header_rows", "footer_rows"} {
		if value, ok := data[key]; ok {
			result[key] = value
		}
	}
	return result
}

// BrowserCell is RichTableLayout#browser_cell without its content and rules.
func (l *Layout) BrowserCell(cell rec.Map) rec.Map {
	result := rec.Pick(cell, cellFields...)
	result["alignment"] = rec.FetchOr(cell, "alignment", "l")
	result["vertical_align"] = rec.FetchOr(cell, "vertical_align", "top")
	result["padding_sp"] = rec.FetchOr(cell, "padding_sp", int64(0))
	result["wrap"] = rec.FetchOr(cell, "wrap", true)
	return result
}

// DisplayRow is one row the reading displays: a logical row on a page.
type DisplayRow struct {
	Row        int
	Page       any
	Occurrence int
	Role       any
	Cells      []rec.Map
}

func rowRole(cells []rec.Map) any {
	if len(cells) == 0 {
		return "body"
	}
	role := rec.FetchOr(cells[0], "role", "body")
	if !truthy(role) {
		return "body"
	}
	return role
}

// DisplayRows is RichTableLayout#display_rows.
func (l *Layout) DisplayRows() []DisplayRow {
	fragments := rec.ListOf(l.Model.Data["native_fragments"])
	result := []DisplayRow{}
	if len(fragments) == 0 {
		for row, cells := range l.Model.Rows {
			result = append(result, DisplayRow{Row: row, Page: int64(0), Occurrence: 0, Role: rowRole(cells), Cells: cells})
		}
		return result
	}
	counts := map[int]int{}
	for _, value := range fragments {
		fragment := rec.AsMap(value)
		for _, item := range rec.ListOf(fragment["rows"]) {
			row := int(rec.FetchInt(rec.AsMap(item), "row"))
			if row < 0 || row >= len(l.Model.Rows) {
				panic(rec.Failure{Message: fmt.Sprintf("index %d outside of array bounds", row), Lookup: true})
			}
			cells := l.Model.Rows[row]
			result = append(result, DisplayRow{Row: row, Page: rec.Fetch(fragment, "page"), Occurrence: counts[row], Role: rowRole(cells), Cells: cells})
			counts[row]++
		}
	}
	return result
}

func (l *Layout) nativeRules(fragment rec.Map) {
	for _, value := range rec.ListOf(fragment["rules"]) {
		rule := rec.AsMap(value)
		if truthy(rec.FetchOr(rule, "math", false)) || tablemodel.AreaFill(rule) {
			continue
		}
		rect := rec.FetchList(rule, "rect_sp")
		x, y, width, height := rect[0], rect[1], rect[2], rect[3]
		if !rubynum.Less(width, height) {
			l.horizontal(fragment, x, y, width, height)
		} else {
			l.vertical(fragment, x, y, width, height)
		}
	}
}

// rowLeft is the left edge of a native row's first grid column. A row's leftmost cell entry
// stands for column 0 unless that column holds no entry: an empty cell under a \multirow
// prints no tagged node, so the row is measured from its first cell that is in the grid,
// less that cell's grid column offset.
func (l *Layout) rowLeft(native rec.Map) any {
	var left, start any
	column := int64(-1)
	for _, value := range rec.FetchList(native, "cells") {
		entry := rec.AsMap(value)
		item := rec.Fetch(entry, "left")
		if left == nil || rubynum.Less(item, left) {
			left = item
		}
		if cell := tablemodel.SourceCell(l.cells, entry["cell"]); cell != nil && (column < 0 || rec.FetchInt(cell, "column") < column) {
			column, start = rec.FetchInt(cell, "column"), rubynum.Sub(item, l.CellRect(cell)[0])
		}
	}
	if start != nil {
		left = start
	}
	origin := rec.FetchList(native, "origin_sp")
	return rubynum.Add(origin[0], left)
}

type edgeChoice struct {
	native rec.Map
	side   string
	edge   any
}

func (l *Layout) horizontal(fragment rec.Map, ruleX, ruleY, width, height any) {
	center := rubynum.Add(ruleY, rubynum.Fdiv(height, 2.0))
	var chosen *edgeChoice
	var best float64
	for _, value := range rec.ListOf(fragment["rows"]) {
		native := rec.AsMap(value)
		origin := rec.FetchList(native, "origin_sp")
		top := origin[len(origin)-1]
		bottom := rubynum.Add(rubynum.Add(top, rec.Fetch(native, "height")), rec.Fetch(native, "depth"))
		for _, choice := range []edgeChoice{{native, "top", top}, {native, "bottom", bottom}} {
			distance := rubynum.Float(rubynum.Abs(rubynum.Sub(choice.edge, center)))
			if chosen == nil || distance < best {
				copied := choice
				chosen, best = &copied, distance
			}
		}
	}
	if chosen == nil {
		crash("undefined method for nil")
	}
	left := l.rowLeft(chosen.native)
	from, to := rubynum.Sub(ruleX, left), rubynum.Sub(rubynum.Add(ruleX, width), left)
	covered := any(int64(0))
	for _, cell := range l.ownersForNativeEdge(int(rec.FetchInt(chosen.native, "row")), chosen.side) {
		rect := l.CellRect(cell)
		first, last := rubynum.Maximum(from, rect[0]), rubynum.Minimum(to, rect[2])
		if !rubynum.Less(first, last) {
			continue
		}
		covered = rubynum.Add(covered, rubynum.Sub(last, first))
		extent := rubynum.Sub(rect[2], rect[0])
		l.appendRule(cell, "horizontal", chosen.side, rubynum.Div(rubynum.Sub(first, rect[0]), extent), rubynum.Div(rubynum.Sub(last, rect[0]), extent),
			height, rubynum.Sub(center, chosen.edge))
	}
	expected := rubynum.Sub(rubynum.Minimum(to, rubynum.Sum(l.ColumnWidths)), rubynum.Maximum(from, int64(0)))
	if rubynum.Less(int64(2), rubynum.Abs(rubynum.Sub(covered, rubynum.Maximum(expected, int64(0))))) {
		rec.Fail("Native horizontal rule crosses a spanned cell interior")
	}
}

func (l *Layout) ownersForNativeEdge(row int, side string) []rec.Map {
	result := []rec.Map{}
	seen := map[string]bool{}
	for _, id := range l.Model.Occupancy[row] {
		if seen[id] {
			continue
		}
		seen[id] = true
		cell := l.cell(id)
		start := rec.FetchInt(cell, "row")
		atEdge := start == int64(row)
		if side != "top" {
			atEdge = start+rec.FetchInt(cell, "rowspan") == int64(row+1)
		}
		if atEdge {
			result = append(result, cell)
		}
	}
	return result
}

func (l *Layout) cell(id string) rec.Map {
	cell, ok := l.cells[id]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rubyInspect(id), Lookup: true})
	}
	return cell
}

type ownerSide struct {
	cell rec.Map
	side string
}

func (l *Layout) ownersAtHorizontal(boundary int) []ownerSide {
	result := []ownerSide{}
	rowCount := int(rec.FetchInt(l.Model.Data, "row_count"))
	for column := range int(rec.FetchInt(l.Model.Data, "column_count")) {
		var found *ownerSide
		if boundary < rowCount {
			if below := l.cell(l.Model.Occupancy[boundary][column]); rec.FetchInt(below, "row") == int64(boundary) {
				found = &ownerSide{below, "top"}
			}
		}
		if found == nil && boundary > 0 {
			above := l.cell(l.Model.Occupancy[boundary-1][column])
			if rec.FetchInt(above, "row")+rec.FetchInt(above, "rowspan") == int64(boundary) {
				found = &ownerSide{above, "bottom"}
			}
		}
		if found != nil && !slices.ContainsFunc(result, func(seen ownerSide) bool {
			return rec.Identity(seen.cell) == rec.Identity(found.cell) && seen.side == found.side
		}) {
			result = append(result, *found)
		}
	}
	return result
}

func (l *Layout) vertical(fragment rec.Map, ruleX, ruleY, width, height any) {
	for _, value := range rec.ListOf(fragment["rows"]) {
		native := rec.AsMap(value)
		origin := rec.FetchList(native, "origin_sp")
		rowTop := origin[len(origin)-1]
		rowBottom := rubynum.Add(rubynum.Add(rowTop, rec.Fetch(native, "height")), rec.Fetch(native, "depth"))
		from, to := rubynum.Maximum(ruleY, rowTop), rubynum.Minimum(rubynum.Add(ruleY, height), rowBottom)
		// \cline pulls the next row up by a rule width: a sliver of overlap is not a crossing.
		if !rubynum.Less(rubynum.Minimum(int64(65_536), rubynum.Div(height, int64(2))), rubynum.Sub(to, from)) {
			continue
		}
		l.verticalSegment(native, ruleX, width, rowTop, from, to)
	}
}

func (l *Layout) verticalSegment(native rec.Map, ruleX, width, rowTop, from, to any) {
	left := l.rowLeft(native)
	positions := []any{int64(0)}
	running := any(nil)
	for _, value := range l.ColumnWidths {
		if running == nil {
			running = rubynum.Add(int64(0), value)
		} else {
			running = rubynum.Add(running, value)
		}
		positions = append(positions, running)
	}
	boundary, best := 0, math.Inf(1)
	for index, position := range positions {
		distance := math.Abs(rubynum.Float(rubynum.Sub(rubynum.Sub(rubynum.Add(position, left), ruleX), rubynum.Fdiv(width, 2.0))))
		if distance < best {
			boundary, best = index, distance
		}
	}
	column := max(boundary-1, 0)
	row := int(rec.FetchInt(native, "row"))
	cell := l.cell(l.Model.Occupancy[row][column])
	side := "right"
	expected := rec.FetchInt(cell, "column") + rec.FetchInt(cell, "colspan")
	if boundary == 0 {
		side, expected = "left", rec.FetchInt(cell, "column")
	}
	if int64(boundary) != expected {
		rec.Fail("Native vertical rule cuts a spanned cell")
	}
	rect := l.CellRect(cell)
	logicalTop := rubynum.Sum(span(l.RowHeights, 0, row))
	extent := rubynum.Sub(rect[3], rect[1])
	start := rubynum.Div(rubynum.Sub(rubynum.Sub(rubynum.Add(logicalTop, from), rowTop), rect[1]), extent)
	finish := rubynum.Div(rubynum.Sub(rubynum.Sub(rubynum.Add(logicalTop, to), rowTop), rect[1]), extent)
	offset := rubynum.Sub(rubynum.Sub(rubynum.Add(ruleX, rubynum.Fdiv(width, 2.0)), left), positions[boundary])
	l.appendRule(cell, "vertical", side, start, finish, width, offset)
}

func (l *Layout) appendRule(cell rec.Map, axis, side string, start, finish, thickness, offset any) {
	first, last := rubynum.RoundDigits(start, 9), rubynum.RoundDigits(finish, 9)
	if !rubynum.Less(first, last) {
		return
	}
	id := rec.FetchString(cell, "id")
	l.segments[id] = append(l.segments[id], rec.Map{"axis": axis, "side": side, "start": first, "end": last,
		"thickness_sp": thickness, "offset_sp": offset})
}

func (l *Layout) legacyRules() {
	for _, cells := range l.Model.Rows {
		for _, cell := range cells {
			for _, value := range rec.ListOf(cell["borders"]) {
				rule := rec.AsMap(value)
				side := "right"
				if rubynum.Less(rec.Fetch(rule, "x"), rubynum.Div(rec.Fetch(cell, "width_sp"), int64(2))) {
					side = "left"
				}
				offset := rubynum.Add(rec.Fetch(rule, "x"), rubynum.Fdiv(rec.Fetch(rule, "width"), 2.0))
				if side == "right" {
					offset = rubynum.Sub(offset, rec.Fetch(cell, "width_sp"))
				} else {
					offset = rubynum.Sub(offset, int64(0))
				}
				l.appendRule(cell, "vertical", side, 0.0, 1.0, rec.Fetch(rule, "width"), offset)
			}
		}
	}
	for _, value := range rec.ListOf(l.Model.Data["horizontal_borders"]) {
		rule := rec.AsMap(value)
		for _, owner := range l.ownersAtHorizontal(int(rec.FetchInt(rule, "after_row"))) {
			l.appendRule(owner.cell, "horizontal", owner.side, 0.0, 1.0, rec.Fetch(rule, "height"), int64(0))
		}
	}
}
