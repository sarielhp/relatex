package tablemodel

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
	"github.com/sarielhp/relatex/internal/stylemodel"
)

// Version is RichTable::VERSION.
const Version = "table-2"

// inlineTypes: a rendered_graphic (a TikZ picture in a cell) becomes an image once the
// figures are rendered from the PDF.
var inlineTypes = []string{"text", "math", "reference", "note_reference", "symbol", "image", "rendered_graphic"}

// SectionRoles is TableSections::ROLES (and RichTableEvidence::ROLES), in table order.
var SectionRoles = []string{"caption", "first_head", "repeat_head", "body", "repeat_foot", "last_foot"}

var columnLetter = regexp.MustCompile(`[lcr]`)

// Rich is RichTable: one anchored cell owns each rich sequence; covered slots
// reference its id. A legacy grid normalizes here.
type Rich struct {
	Data      rec.Map
	Occupancy [][]any
	// nodes are the paper nodes cell math must name (nil: not checked).
	nodes rec.Map
}

// New is RichTable.new(grid) (without math nodes).
func New(grid rec.Map) *Rich { return NewWith(grid, nil) }

// NewWith is RichTable.new(grid, nodes:). A cell may hold nested rich tables, to any
// depth (decision 060).
func NewWith(grid rec.Map, nodes rec.Map) *Rich {
	t := &Rich{Data: grid, nodes: nodes}
	if grid["schema_version"] != Version {
		t.Data = legacyGrid(grid)
	}
	t.validate()
	return t
}

// Rows is RichTable#rows: the cells of each row in column order, every row present.
func (t *Rich) Rows() [][]rec.Map {
	count := int(fetchInt(t.Data, "row_count"))
	rows := map[int64][]rec.Map{}
	order := []int64{}
	for _, value := range FetchList(t.Data, "cells") {
		cell := rec.AsMap(value)
		row := fetchInt(cell, "row")
		if _, ok := rows[row]; !ok {
			order = append(order, row)
		}
		rows[row] = append(rows[row], cell)
	}
	for row := range int64(count) {
		if _, ok := rows[row]; !ok {
			order = append(order, row)
			rows[row] = []rec.Map{}
		}
	}
	slices.Sort(order)
	result := make([][]rec.Map, len(order))
	for index, row := range order {
		cells := slices.Clone(rows[row])
		slices.SortStableFunc(cells, func(a, b rec.Map) int { return compareNumbers(rec.Fetch(a, "column"), rec.Fetch(b, "column")) })
		result[index] = cells
	}
	return result
}

func (t *Rich) validate() {
	schema.Validate(t.Data, Contract("rich-table.schema.json"), "$")
	if rec.Fetch(t.Data, "schema_version") != Version {
		rec.Fail("Unknown table schema")
	}
	rows := t.dimension("row_count", 4096)
	columns := t.dimension("column_count", 64)
	if rows*columns > 65_536 {
		rec.Fail("Table dimensions exceed occupancy limit")
	}
	t.Occupancy = make([][]any, rows)
	for row := range t.Occupancy {
		t.Occupancy[row] = make([]any, columns)
	}
	ids := map[string]bool{}
	for _, value := range FetchList(t.Data, "cells") {
		cell := rec.AsMap(value)
		id, ok := rec.Fetch(cell, "id").(string)
		if !ok || id == "" {
			rec.Fail("Invalid table cell identity")
		}
		if ids[id] {
			rec.Fail("Duplicate table cell identity")
		}
		ids[id] = true
		t.occupy(cell, id, rows, columns)
		t.validateContent(cell)
	}
	for _, row := range t.Occupancy {
		if slices.Contains(row, nil) {
			rec.Fail("Table occupancy contains undeclared empty cells")
		}
	}
	t.validateGeometry()
	t.validateSections()
}

func (t *Rich) dimension(name string, limit int64) int64 {
	value, ok := rec.Fetch(t.Data, name).(int64)
	if !ok || value < 1 || value > limit {
		rec.Fail("Invalid table dimensions")
	}
	return value
}

func (t *Rich) occupy(cell rec.Map, id string, rows, columns int64) {
	row, rowOK := cell["row"].(int64)
	column, columnOK := cell["column"].(int64)
	rowspan, rowspanOK := cell["rowspan"].(int64)
	colspan, colspanOK := cell["colspan"].(int64)
	if !rowOK || !columnOK || !rowspanOK || !colspanOK || row < 0 || column < 0 || rowspan < 1 || colspan < 1 ||
		row+rowspan > rows || column+colspan > columns {
		rec.Fail("Invalid table span: %s", id)
	}
	for y := row; y < row+rowspan; y++ {
		for x := column; x < column+colspan; x++ {
			if prior := t.Occupancy[y][x]; prior != nil {
				rec.Fail("Overlapping table cells %s and %s at %d,%d", prior, id, y, x)
			}
			t.Occupancy[y][x] = id
		}
	}
}

func (t *Rich) validateContent(cell rec.Map) {
	content, ok := rec.Fetch(cell, "content").([]any)
	if !ok {
		rec.Fail("Rich cell content must be an array")
	}
	for _, value := range content {
		span := rec.AsMap(value)
		kind := rec.Fetch(span, "type")
		if kind == "table_grid" {
			t.validateNested(span)
			continue
		}
		if name, isString := kind.(string); !isString || !slices.Contains(inlineTypes, name) {
			rec.Fail("Unsupported rich cell inline type")
		}
		if kind == "text" {
			if _, isString := rec.Fetch(span, "text").(string); !isString {
				rec.Fail("Cell text is not a string")
			}
		}
		if style, ok := span["style"]; ok {
			stylemodel.Validate(style)
		}
		if kind == "math" && t.nodes != nil {
			t.validateMath(cell, span)
		}
	}
}

func (t *Rich) validateMath(cell, span rec.Map) {
	node, ok := t.nodes[rec.FetchString(span, "node")].(rec.Map)
	if !ok {
		rec.Fail("Dangling math node in cell %v", rec.Fetch(cell, "id"))
	}
	if rec.Fetch(node, "type") != "math" {
		rec.Fail("Cell math reference names a non-math node")
	}
}

func (t *Rich) validateNested(span rec.Map) {
	if dig(span["grid"], "schema_version") != Version {
		rec.Fail("Nested table must be a rich table")
	}
	NewWith(fetchMap(span, "grid"), t.nodes)
}

func (t *Rich) validateGeometry() {
	widths, hasWidths := t.Data["column_widths_sp"].([]any)
	if raw, ok := t.Data["column_widths_sp"]; ok && raw != nil && !hasWidths {
		rec.Fail("undefined method 'size' for %s", Inspect(raw))
	}
	if hasWidths && (int64(len(widths)) != fetchInt(t.Data, "column_count") || !rec.Equal(Sum(widths), rec.Fetch(t.Data, "width_sp"))) {
		rec.Fail("Rich table column geometry differs from width")
	}
	// Column boundaries average observed cell edges (2sp tolerance each), so a cell spans its
	// columns' total width up to two boundary tolerances.
	if hasWidths {
		for _, value := range FetchList(t.Data, "cells") {
			cell := rec.AsMap(value)
			column, colspan := fetchInt(cell, "column"), fetchInt(cell, "colspan")
			spanned := Sum(widths[column:min(column+colspan, int64(len(widths)))])
			if abs(number(sub(rec.Fetch(cell, "width_sp"), spanned))) > 4 {
				rec.Fail("Rich table cell width differs from its spanned columns")
			}
		}
	}
	rowCount, columnCount := fetchInt(t.Data, "row_count"), fetchInt(t.Data, "column_count")
	for _, name := range []string{"header_rows", "footer_rows", "declared_header_rows", "author_header_rows", "author_header_columns"} {
		indices := rec.AsList(fetchDefault(t.Data, name, []any{}))
		limit := rowCount
		if name == "author_header_columns" {
			limit = columnCount
		}
		valid := rec.Equal(indices, uniqueValues(SortValues(indices)))
		for _, index := range indices {
			valid = valid && number(index) >= 0 && number(index) <= float64(limit-1)
		}
		if !valid {
			rec.Fail("Invalid rich table header/footer rows")
		}
	}
	headers := rec.AsList(fetchDefault(t.Data, "header_rows", []any{}))
	for _, index := range rec.AsList(fetchDefault(t.Data, "footer_rows", []any{})) {
		if containsEql(headers, index) {
			rec.Fail("Rich table header and footer rows overlap")
		}
	}
	for _, fragment := range maps(rec.AsList(fetchDefault(t.Data, "native_fragments", []any{}))) {
		validateFragment(fragment, rowCount)
	}
}

func validateFragment(fragment rec.Map, rowCount int64) {
	if !rec.Positive(rec.Fetch(fragment, "page")) {
		rec.Fail("Invalid native table page")
	}
	for _, row := range maps(FetchList(fragment, "rows")) {
		index, ok := rec.Fetch(row, "row").(int64)
		if !ok || index < 0 || index > rowCount-1 {
			rec.Fail("Native table row has no logical owner")
		}
		geometryNumbers(rec.Fetch(row, "origin_sp"), 2)
	}
	for _, rule := range maps(FetchList(fragment, "rules")) {
		rect := geometryNumbers(rec.Fetch(rule, "rect_sp"), 4)
		if !(rect[2] > 0) || !(rect[3] > 0) {
			rec.Fail("Invalid native table rule extent")
		}
	}
}

// geometryNumbers is Geometry.numbers on any JSON value.
func geometryNumbers(value any, count int) []float64 {
	list, ok := value.([]any)
	if !ok {
		rec.Fail("Invalid geometry vector")
	}
	return geometry.Numbers(list, count)
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func (t *Rich) validateSections() {
	raw, ok := t.Data["sections"]
	if !ok {
		return
	}
	sections := rec.AsMap(raw)
	all := []any{}
	for name, indices := range sections {
		if !slices.Contains(SectionRoles, name) {
			rec.Fail("Unknown rich table section")
		}
		all = append(all, rec.AsList(indices)...)
	}
	expected := make([]any, fetchInt(t.Data, "row_count"))
	for index := range expected {
		expected[index] = int64(index)
	}
	if !rec.Equal(SortValues(all), expected) {
		rec.Fail("Rich table sections overlap or omit rows")
	}
	for _, indices := range sections {
		if !rec.Equal(uniqueValues(SortValues(rec.AsList(indices))), indices) {
			rec.Fail("Unordered rich table section rows")
		}
	}
	body := fetchDefault(sections, "body", []any{})
	if !rec.Equal(rec.Fetch(t.Data, "body_rows"), body) {
		rec.Fail("Rich table body index differs")
	}
	fragments := rec.AsList(fetchDefault(t.Data, "native_fragments", []any{}))
	if len(fragments) == 0 {
		return
	}
	counts := map[string]int{}
	for _, fragment := range maps(fragments) {
		for _, row := range maps(FetchList(fragment, "rows")) {
			counts[key(rec.Fetch(row, "row"))]++
		}
	}
	for _, row := range rec.AsList(body) {
		if counts[key(row)] != 1 {
			rec.Fail("Native table body rows are missing or repeated")
		}
	}
}

// legacyGrid is RichTable#legacy_grid: a classic grid as a rich table.
func legacyGrid(grid rec.Map) rec.Map {
	ValidateShape(grid)
	alignment := columnLetter.FindAllString(rec.FetchString(grid, "preamble"), -1)
	cells := []any{}
	for y, row := range FetchList(grid, "rows") {
		for x, value := range rec.AsList(row) {
			cell := rec.AsMap(value)
			runs := fetchDefault(cell, "runs", []any{rec.Map{"text": rec.Fetch(cell, "text")}})
			content := []any{}
			for _, run := range rec.AsList(runs) {
				merged := copyMap(rec.AsMap(run))
				merged["type"] = "text"
				content = append(content, merged)
			}
			if x >= len(alignment) {
				panic(indexError{fmt.Sprintf("index %d outside of array bounds: %d...%d", x, -len(alignment), len(alignment))})
			}
			converted := except(cell, "text", "runs")
			for name, field := range (rec.Map{"id": fmt.Sprintf("r%dc%d", y, x), "row": int64(y), "column": int64(x), "rowspan": int64(1),
				"colspan": int64(1), "alignment": alignment[x], "vertical_align": "top", "content": content}) {
				converted[name] = field
			}
			cells = append(cells, converted)
		}
	}
	return rec.Map{"schema_version": Version, "row_count": int64(len(FetchList(grid, "rows"))),
		"column_count": int64(len(alignment)), "width_sp": rec.Fetch(grid, "width_sp"), "cells": cells,
		"horizontal_borders": rec.Fetch(grid, "horizontal_borders"), "source_version": "legacy-grid"}
}

// ValidateShape is TableGridShape.validate!: the published grid, independently of the
// transient native node identities.
func ValidateShape(grid rec.Map) rec.Map {
	if grid["schema_version"] == Version {
		return New(grid).Data
	}
	schema.Validate(grid, Contract("table-grid.schema.json"), "$")
	preamble := rec.FetchString(grid, "preamble")
	if !ClassicPreamble.MatchString(preamble) {
		rec.Fail("Unsupported published grid preamble")
	}
	columns := strings.Count(preamble, "l") + strings.Count(preamble, "c") + strings.Count(preamble, "r")
	rows := FetchList(grid, "rows")
	if columns < 1 || columns > 16 || len(rows) < 1 || len(rows) > 64 {
		rec.Fail("Invalid published grid size")
	}
	first := maps(rec.AsList(rows[0]))
	widths := make([]any, len(first))
	for index, cell := range first {
		widths[index] = rec.Fetch(cell, "width_sp")
	}
	if len(widths) != columns || !rec.Equal(Sum(widths), rec.Fetch(grid, "width_sp")) {
		rec.Fail("Published grid width mismatch")
	}
	var separate bool
	if firstBorders := FetchList(first[0], "borders"); len(firstBorders) > 0 {
		separate = isZero(rec.Fetch(rec.AsMap(firstBorders[0]), "x"))
	}
	for _, value := range rows {
		validateShapeRow(maps(rec.AsList(value)), widths, separate)
	}
	validateHorizontal(grid)
	return grid
}

func validateShapeRow(row []rec.Map, widths []any, separate bool) {
	rowWidths := make([]any, len(row))
	extents := []any{}
	for index, cell := range row {
		rowWidths[index] = rec.Fetch(cell, "width_sp")
		extent := fetchValues(cell, "height_sp", "depth_sp")
		if !contains(extents, extent) {
			extents = append(extents, extent)
		}
	}
	if !rec.Equal(rowWidths, widths) {
		rec.Fail("Published grid column mismatch")
	}
	if len(extents) != 1 {
		rec.Fail("Published grid row extent mismatch")
	}
	for column, cell := range row {
		validateShapeCell(cell, column, separate)
	}
}

func validateShapeCell(cell rec.Map, column int, separate bool) {
	if _, ok := cell["runs"]; ok {
		validateTextRuns(cell)
	}
	borders := maps(FetchList(cell, "borders"))
	if len(borders) != map[bool]int{true: 2, false: 1}[column == 0] {
		rec.Fail("Published cell border mismatch")
	}
	for index, border := range borders {
		width := rec.Fetch(border, "width")
		x := BorderX(rec.Fetch(cell, "width_sp"), column, index, width, separate)
		if !positive(width) || !Even(width) || !rec.Equal(rec.Fetch(border, "x"), x) ||
			!rec.Equal(values(border, "height", "depth"), values(cell, "height_sp", "depth_sp")) {
			rec.Fail("Published cell border geometry mismatch")
		}
	}
}

func validateTextRuns(cell rec.Map) {
	runs := maps(FetchList(cell, "runs"))
	var text strings.Builder
	for _, run := range runs {
		text.WriteString(rec.FetchString(run, "text"))
	}
	if text.String() != rec.Fetch(cell, "text") {
		rec.Fail("Styled cell text mismatch")
	}
	for _, run := range runs {
		if style, ok := run["style"]; ok {
			stylemodel.Validate(style)
		}
	}
}

func validateHorizontal(grid rec.Map) {
	borders := maps(FetchList(grid, "horizontal_borders"))
	rows := int64(len(FetchList(grid, "rows")))
	boundaries := make([]any, len(borders))
	for index, border := range borders {
		boundaries[index] = rec.Fetch(border, "after_row")
	}
	valid := rec.Equal(boundaries, uniqueValues(SortValues(boundaries)))
	for _, boundary := range boundaries {
		valid = valid && number(boundary) >= 0 && number(boundary) <= float64(rows)
	}
	if !valid {
		rec.Fail("Published grid row boundary mismatch")
	}
	for _, border := range borders {
		if !rec.Equal(rec.Fetch(border, "width"), rec.Fetch(grid, "width_sp")) || !positive(rec.Fetch(border, "height")) ||
			!isZero(rec.Fetch(border, "depth")) {
			rec.Fail("Published horizontal border mismatch")
		}
	}
}

// ClassicPreamble is the preamble of a classic (published) grid: TableGridShape::PREAMBLE.
var ClassicPreamble = regexp.MustCompile(`\A\|[lcr](?:\|[lcr])*\|\z`)

// Even is Integer#even?; a Float has no even? (NoMethodError), reported as a type failure.
func Even(value any) bool {
	integer, ok := value.(int64)
	if !ok {
		rec.Fail("undefined method 'even?' for %s", Inspect(value))
	}
	return integer%2 == 0
}

// BorderX is where a cell's border starts: TableGrid#border_x and TableGridShape.border_x.
func BorderX(cellWidth any, column, borderIndex int, width any, separate bool) any {
	if column == 0 && borderIndex == 0 {
		if separate {
			return int64(0)
		}
		return half(negate(width))
	}
	if separate {
		return sub(cellWidth, width)
	}
	return sub(cellWidth, half(width))
}

// uniqueValues is Array#uniq (eql?).
func uniqueValues(list []any) []any {
	result := []any{}
	for _, value := range list {
		if !containsEql(result, value) {
			result = append(result, value)
		}
	}
	return result
}
