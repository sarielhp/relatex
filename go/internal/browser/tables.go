package browser

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// The attributes PaperTableBrowser and PaperRichTableBrowser expect.
var (
	cellAttributes     = strings2any([]string{"data-depth-sp", "data-font-size-sp", "data-height-sp", "data-width-sp", "style"})
	textAttributes     = strings2any([]string{"data-cell-text", "style"})
	ruleAttributes     = []string{"aria-hidden", "data-depth", "data-height", "data-rule", "data-width", "style"}
	richCellAttributes = strings2any([]string{"colspan", "data-cell-meta", "data-rich-cell", "rowspan", "style"})
	columnLetter       = regexp.MustCompile(`[lcr]`)
	alignments         = map[string]string{"l": "left", "c": "center", "r": "right", "left": "left", "center": "center", "right": "right"}
)

// defaultTextColor is the page's text color for an unpainted table.
// defaultTextColor is the light theme's ink (the checks force the light theme), which every page uses.
const defaultTextColor = "rgb(0, 0, 0)"

// tolerance is PaperRichTableBrowser::TOLERANCE_PX (and the classic checks' 1/64 px).
const tolerance = 1.0 / 64

// browserSpan is RichTableLayout.browser_span: a grid span as the browser inventory reads it
// back, a rich grid by its layout.
func browserSpan(span rec.Map) rec.Map {
	grid, _ := span["grid"].(rec.Map)
	if span["type"] != "table_grid" || grid["schema_version"] != tablemodel.Version {
		return span
	}
	if classic := tablelayout.ClassicProjection(grid); classic != nil {
		return rec.Merge(span, rec.Map{"grid": classic})
	}
	layout := tablelayout.NewLayout(tablelayout.NewTable(grid, nil))
	cells := []any{}
	for _, row := range layout.Model.Rows {
		for _, cell := range row {
			content := []any{}
			for _, inner := range rec.FetchList(cell, "content") {
				content = append(content, browserSpan(rec.AsMap(inner)))
			}
			cells = append(cells, rec.Merge(layout.BrowserCell(cell), rec.Map{"content": content, "rules": maps(layout.RulesFor(cell))}))
		}
	}
	return rec.Merge(span, rec.Map{"grid": rec.Merge(layout.BrowserGrid(), rec.Map{"cells": cells})})
}

// validateTables is PaperTableBrowser#validate! on the document's tables, each matched to the
// page's table of the same id: a parsed document holds its nodes in key order, not the order
// the page lists them in. It returns their ids.
func validateTables(inventory, document rec.Map) []string {
	spans, byID := tableSpans(document)
	names := htmlkit.TableNames(rec.FetchMap(document, "nodes"))
	observed := rec.FetchList(inventory, "tables")
	if len(observed) != len(spans) || len(byID) != len(spans) {
		rec.Fail("Browser changed table count")
	}
	color := defaultTextColor
	if truthy(document["text_paint"]) {
		color = textPaintColor
	}
	paint, _ := document["table_paint"].(rec.Map)
	ids := []string{}
	for _, value := range observed {
		table := rec.AsMap(value)
		span, found := byID[rec.ToS(table["id"])]
		if !found {
			rec.Fail("Browser changed native grid markup")
		}
		check := &classicCheck{color: paintColor(paint, span, color), name: names[rec.ToS(rec.Fetch(span, "id"))]}
		check.validate(table, span)
		ids = append(ids, rec.ToS(rec.Fetch(span, "id")))
	}
	return ids
}

// tableSpans are the document's table spans and the same spans by id.
func tableSpans(document rec.Map) ([]rec.Map, map[string]rec.Map) {
	spans := []rec.Map{}
	byID := map[string]rec.Map{}
	for _, node := range nodeValues(document) {
		content, _ := node["content"].([]any)
		for _, span := range inline.Deep(content) {
			if span["type"] == "table_grid" {
				spans = append(spans, span)
				byID[rec.ToS(rec.Fetch(span, "id"))] = span
			}
		}
	}
	return spans, byID
}

func paintColor(paint, span rec.Map, fallback string) string {
	if paint == nil {
		return fallback
	}
	for _, value := range rec.FetchList(paint, "tables") {
		if row := rec.AsMap(value); rec.Equal(rec.Fetch(row, "id"), rec.Fetch(span, "id")) {
			return rec.FetchString(row, "color")
		}
	}
	return fallback
}

// classicCheck is one PaperTableBrowser table check; headers are the grid's column header rows.
type classicCheck struct {
	color   string
	name    string // the caption id the float names the table by, "" when unnamed
	headers []int  // the column header rows
	columns []int  // the row header columns (decision 131)
}

func (c *classicCheck) validate(table, span rec.Map) {
	grid := rec.FetchMap(span, "grid")
	layout := tablelayout.NewLayout(tablelayout.NewTable(grid, nil))
	c.headers, c.columns = layout.HeaderRows(), layout.HeaderColumns()
	expected := []string{"data-kind", "data-preamble", "data-width-sp", "id", "style"}
	if tablelayout.Presentational(grid, nil) {
		expected = append(expected, "role")
	}
	if c.name != "" {
		expected = append(expected, "aria-labelledby")
	}
	sortStrings(expected)
	attributes := strings2any(expected)
	if grid["schema_version"] == tablemodel.Version {
		classic := tablelayout.ClassicProjection(grid)
		if classic == nil {
			(&richCheck{color: c.color}).validate(table, span, c.name)
			return
		}
		grid = classic
	}
	if !rec.Equal([]any{table["id"], table["kind"], table["attributes"], table["name"], table["visible"]},
		[]any{rec.Fetch(span, "id"), "table_grid", attributes, nameValue(c.name), true}) ||
		!rec.Equal(rec.Fetch(table, "shape"), []any{rec.Map{"tag": "TBODY", "attributes": []any{}}}) {
		rec.Fail("Browser changed native grid markup")
	}
	text := ""
	for _, row := range rec.FetchList(grid, "rows") {
		for _, cell := range rec.AsList(row) {
			text += rec.FetchString(rec.AsMap(cell), "text")
		}
	}
	if !rec.Equal([]any{table["text"], table["parent_text"]}, []any{text, text}) {
		rec.Fail("Browser introduced unmodeled table text")
	}
	measure(table, "width", rec.Fetch(grid, "width_sp"), "table")
	height := 0.0
	for index, row := range rec.FetchList(grid, "rows") {
		height += rowHeight(rec.AsList(row)) + rowGap(grid, index)
	}
	measure(table, "height", height, "table")
	c.verifyRows(rec.FetchList(table, "rows"), grid)
}

// nameValue is the page's aria-labelledby value: the caption id, or nil when the table is unnamed.
func nameValue(name string) any {
	if name == "" {
		return nil
	}
	return name
}

func number(value any) float64 {
	result, ok := rec.Number(value)
	if !ok {
		rec.Fail("undefined method for %T", value)
	}
	return result
}

func rowHeight(row []any) float64 {
	first := rec.AsMap(row[0])
	return number(rec.Fetch(first, "height_sp")) + number(rec.Fetch(first, "depth_sp"))
}

// measure is PaperTableBrowser#measure!: the element's size in px, the grid's in sp.
func measure(element rec.Map, key string, expected any, subject string) {
	target := rec.Round(pixels(expected), 6)
	if math.Abs(number(rec.Fetch(element, key))-target) >= tolerance {
		rec.Fail("Browser changed %s %s", subject, key)
	}
}

func rowGap(grid rec.Map, index int) float64 {
	gap := 0.0
	for _, value := range rec.FetchList(grid, "horizontal_borders") {
		border := rec.AsMap(value)
		if rec.Equal(rec.Fetch(border, "after_row"), int64(index+1)) {
			gap += number(rec.Fetch(border, "height"))
		}
	}
	return gap
}

func (c *classicCheck) verifyRows(observed []any, grid rec.Map) {
	rows := rec.FetchList(grid, "rows")
	if len(observed) != len(rows) {
		rec.Fail("Browser changed table rows")
	}
	for index, value := range observed {
		row, cells := rec.AsMap(value), rec.AsList(rows[index])
		shape := make([]any, len(cells))
		for column, cell := range cells {
			shape[column], _ = cellMarkup(c.headerScope(rec.AsMap(cell), index, column) != "", cellAttributes)
		}
		if len(rec.FetchList(row, "attributes")) > 0 || !rec.Equal(rec.Fetch(row, "shape"), shape) ||
			len(rec.FetchList(row, "cells")) != len(cells) {
			rec.Fail("Browser changed table cells")
		}
		for column, cell := range rec.FetchList(row, "cells") {
			c.verifyCell(rec.AsMap(cell), rec.AsMap(cells[column]), grid, index, column)
		}
	}
}

func (c *classicCheck) verifyCell(cell, source, grid rec.Map, row, column int) {
	height := number(rec.Fetch(source, "height_sp")) + number(rec.Fetch(source, "depth_sp"))
	scope := c.headerScope(source, row, column)
	tag, attributes := cellMarkup(scope != "", cellAttributes)
	if !rec.Equal([]any{cell["tag"], cell["attributes"], cell["visible"]}, []any{tag, attributes, true}) || scope != "" && cell["scope"] != scope {
		rec.Fail("Browser changed table cell markup")
	}
	rows := rec.FetchList(grid, "rows")
	left := 0.0
	for _, entry := range rec.AsList(rows[row])[:column] {
		left += number(rec.Fetch(rec.AsMap(entry), "width_sp"))
	}
	top := 0.0
	for index, entries := range rows[:row] {
		top += rowHeight(rec.AsList(entries)) + rowGap(grid, index)
	}
	if !rec.Equal(rec.Fetch(cell, "text_content"), rec.Fetch(source, "text")) {
		rec.Fail("Browser introduced unmodeled cell text")
	}
	measure(cell, "left", left, "table")
	measure(cell, "top", top, "table")
	measure(cell, "width", rec.Fetch(source, "width_sp"), "table")
	measure(cell, "height", height+rowGap(grid, row), "table")
	c.verifyText(cell, source, grid, column, height)
	c.verifyRules(cell, source, grid, row, column)
}

// headerScope: a classic cell holding text is a column header in a header row (decision 088), else
// a row header in a header column (decision 131); "" for a data cell.
func (c *classicCheck) headerScope(cell rec.Map, row, column int) string {
	switch {
	case strings.TrimFunc(rec.FetchString(cell, "text"), unicode.IsSpace) == "":
		return ""
	case slices.Contains(c.headers, row):
		return tablelayout.ScopeColumn
	case slices.Contains(c.columns, column):
		return tablelayout.ScopeRow
	}
	return ""
}

func (c *classicCheck) verifyText(cell, source, grid rec.Map, column int, height float64) {
	text, _ := cell["text"].(rec.Map)
	runs, _ := source["runs"].([]any)
	if text == nil || !rec.Equal([]any{text["attributes"], text["children"], text["visible"]}, []any{textAttributes, int64(len(runs)), true}) {
		rec.Fail("Browser changed table text markup")
	}
	letters := columnLetter.FindAllString(rec.FetchString(grid, "preamble"), -1)
	if column >= len(letters) {
		panic(rec.Failure{Message: fmt.Sprintf("index %d outside of array bounds: %d...%d", column, -len(letters), len(letters)), Lookup: true})
	}
	colors := make([]any, len(runs))
	for index := range colors {
		colors[index] = c.color
	}
	if !rec.Equal([]any{text["alignment"], text["color"], text["run_colors"]}, []any{alignments[letters[column]], c.color, colors}) {
		rec.Fail("Browser changed table text presentation")
	}
	measure(text, "height", height, "table")
	measure(text, "line_height", height, "table")
	measure(text, "font_size", rec.Fetch(source, "font_size_sp"), "table")
}

type border struct {
	kind  string
	value rec.Map
}

func (c *classicCheck) verifyRules(cell, source, grid rec.Map, row, column int) {
	rules := []border{}
	for _, value := range rec.FetchList(source, "borders") {
		rules = append(rules, border{"vertical", rec.AsMap(value)})
	}
	if column == 0 {
		for _, value := range rec.FetchList(grid, "horizontal_borders") {
			after := rec.Fetch(rec.AsMap(value), "after_row")
			if (rec.Equal(after, int64(0)) && row == 0) || rec.Equal(after, int64(row+1)) {
				rules = append(rules, border{"horizontal", rec.AsMap(value)})
			}
		}
	}
	shape := []any{"text"}
	for _, rule := range rules {
		shape = append(shape, rule.kind)
	}
	observed := rec.FetchList(cell, "rules")
	if !rec.Equal(rec.Fetch(cell, "shape"), shape) || len(observed) != len(rules) {
		rec.Fail("Browser changed table border placement")
	}
	for index, rule := range rules {
		c.verifyRule(rec.AsMap(observed[index]), rule, source)
	}
}

func (c *classicCheck) verifyRule(observed rec.Map, rule border, cell rec.Map) {
	attributes := append([]string{}, ruleAttributes...)
	if rule.kind == "vertical" {
		attributes = append(attributes, "data-x")
	} else {
		attributes = append(attributes, "data-after-row")
	}
	sortStrings(attributes)
	wanted := []any{rule.kind, strings2any(attributes), int64(0), "", "true", true, "absolute"}
	seen := []any{observed["kind"], observed["attributes"], observed["children"], observed["text"], observed["hidden"],
		observed["visible"], observed["position"]}
	if !rec.Equal(seen, wanted) || !rec.Equal([]any{observed["background"], observed["color"]}, []any{c.color, c.color}) {
		rec.Fail("Browser changed visible table border")
	}
	height := number(rec.Fetch(cell, "height_sp")) + number(rec.Fetch(cell, "depth_sp"))
	left, top, size := 0.0, 0.0, number(rec.Fetch(rule.value, "height"))
	if rule.kind == "vertical" {
		left, size = number(rec.Fetch(rule.value, "x")), height
	} else if rec.Equal(rec.Fetch(rule.value, "after_row"), int64(0)) {
		top = -number(rec.Fetch(rule.value, "height"))
	} else {
		top = height
	}
	measure(observed, "left", left, "table")
	measure(observed, "top", top, "table")
	measure(observed, "width", rec.Fetch(rule.value, "width"), "table")
	measure(observed, "height", size, "table")
}
