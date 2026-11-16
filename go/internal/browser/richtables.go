package browser

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
	"github.com/sarielhp/relatex/internal/tablelayout"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

func sortStrings(values []string) { sort.Strings(values) }

// shrunkCells are the cells the browser set shorter than their native height in this check.
var shrunkCells []any

// richCheck is one PaperRichTableBrowser table check.
type richCheck struct {
	table  string
	layout *tablelayout.Layout
	color  string
}

func (r *richCheck) validate(table, span rec.Map, name string) {
	r.table = rec.ToS(span["id"])
	r.layout = tablelayout.NewLayout(tablelayout.NewTable(rec.FetchMap(span, "grid"), nil))
	expected := []string{"data-grid-meta", "data-kind", "data-schema", "id", "style"}
	if tablelayout.Presentational(rec.FetchMap(span, "grid"), nil) {
		expected = append(expected, "role")
	}
	if name != "" {
		expected = append(expected, "aria-labelledby")
	}
	sortStrings(expected)
	if !rec.Equal([]any{table["id"], table["kind"], table["schema"], table["attributes"], table["name"], table["visible"]},
		[]any{rec.Fetch(span, "id"), "table_grid", tablemodel.Version, strings2any(expected), nameValue(name), true}) {
		rec.Fail("Browser changed rich table markup")
	}
	shape := []any{rec.Map{"tag": "COLGROUP", "attributes": []any{}}, rec.Map{"tag": "TBODY", "attributes": []any{}}}
	if !rec.Equal(rec.Fetch(table, "shape"), shape) {
		rec.Fail("Browser changed rich table structure")
	}
	richMeasure(table, "width", pixels(rubynum.Sum(r.layout.ColumnWidths)), tolerance)
	r.verifyColumns(rec.FetchList(table, "columns"))
	rows := rec.FetchList(table, "rows")
	display := r.layout.DisplayRows()
	if len(rows) != len(display) {
		rec.Fail("Browser changed rich table row count")
	}
	for index, value := range rows {
		r.verifyRow(rec.AsMap(value), display[index])
	}
}

func (r *richCheck) verifyRow(row rec.Map, entry tablelayout.DisplayRow) {
	rich := rec.Map{"row": int64(entry.Row), "page": entry.Page, "occurrence": int64(entry.Occurrence), "role": entry.Role}
	if !rec.Equal(row["rich_row"], rich) {
		rec.Fail("Browser changed logical/physical table row ownership")
	}
	expected := []string{"data-occurrence", "data-page", "data-role", "data-row", "style"}
	headers, _ := r.layout.Model.Data["header_rows"].([]any)
	for _, header := range headers {
		if rec.Equal(header, int64(entry.Row)) {
			expected = append(expected, "data-header-row")
			break
		}
	}
	sortStrings(expected)
	shape := make([]any, len(entry.Cells))
	for index, cell := range entry.Cells {
		shape[index], _ = cellMarkup(r.layout.HeaderCell(cell), richCellMarkup(cell))
	}
	if !rec.Equal(rec.Fetch(row, "attributes"), strings2any(expected)) || !rec.Equal(rec.Fetch(row, "shape"), shape) {
		rec.Fail("Browser changed rich table row markup")
	}
	cells := rec.FetchList(row, "cells")
	if len(cells) != len(entry.Cells) {
		rec.Fail("Browser changed rich table cell count")
	}
	for index, cell := range cells {
		r.verifyCell(rec.AsMap(cell), entry.Cells[index])
	}
}

// richMeasure is PaperRichTableBrowser#measure!.
func richMeasure(element rec.Map, key string, expected, allowed float64) {
	observed := number(rec.Fetch(element, key))
	if math.Abs(observed-expected) <= allowed {
		return
	}
	rec.Fail("Browser changed rich table %s (%s px for %s)", key, rounded(rec.Fetch(element, key)), rec.ToS(rec.Round(expected, 4)))
}

// rounded is value.round(4).to_s: an Integer stays itself.
func rounded(value any) string {
	if number, ok := value.(int64); ok {
		return rec.ToS(number)
	}
	return rec.ToS(rec.Round(number(value), 4))
}

func (r *richCheck) verifyColumns(columns []any) {
	if len(columns) != len(r.layout.ColumnWidths) {
		rec.Fail("Browser changed rich table columns")
	}
	for index, value := range columns {
		observed, width := rec.AsMap(value), r.layout.ColumnWidths[index]
		if !rec.Equal(rec.Fetch(observed, "width_sp"), width) {
			rec.Fail("Browser changed declared column width")
		}
		richMeasure(observed, "width", pixels(width), tolerance)
	}
}

func (r *richCheck) verifyCell(observed, cell rec.Map) {
	scope := r.layout.CellScope(cell)
	tag, attributes := cellMarkup(scope != "", formulaNamed(scope != "", cell, richCellMarkup(cell)))
	if !rec.Equal([]any{observed["tag"], observed["attributes"], observed["visible"]}, []any{tag, attributes, true}) ||
		scope != "" && observed["scope"] != scope {
		rec.Fail("Browser changed rich table cell markup: %v %v visible=%v, expected %s %v scope %q (%v)", observed["tag"], observed["attributes"], observed["visible"], tag, attributes, scope, observed["scope"])
	}
	rich := rec.FetchMap(observed, "rich")
	if !rec.Equal(rec.Fetch(rich, "metadata"), r.layout.BrowserCell(cell)) ||
		!rec.Equal([]any{rich["rowspan"], rich["colspan"]}, []any{cell["rowspan"], cell["colspan"]}) {
		rec.Fail("Browser changed rich cell span/identity")
	}
	horizontal, ok := alignments[rec.ToS(rec.FetchOr(cell, "alignment", "l"))]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rec.Inspect(rec.FetchOr(cell, "alignment", "l")), Lookup: true})
	}
	wrapping := "nowrap"
	if truthy(rec.FetchOr(cell, "wrap", true)) {
		wrapping = "normal"
	}
	if !rec.Equal([]any{rich["alignment"], rich["vertical_align"], rich["wrapping"]},
		[]any{horizontal, rec.FetchOr(cell, "vertical_align", "top"), wrapping}) {
		rec.Fail("Browser changed rich cell alignment/wrapping")
	}
	if background := cellBackground(cell); !rec.Equal(rich["background"], background) {
		rec.Fail("Browser changed rich cell fill: %v, expected %v", rich["background"], background)
	}
	for _, key := range []string{"padding_left", "padding_right"} {
		richMeasure(rich, key, pixels(rec.FetchOr(cell, "padding_sp", int64(0))), tolerance)
	}
	rect := r.layout.CellRect(cell)
	richMeasure(observed, "width", pixels(number(rect[2])-number(rect[0])), tolerance)
	// A browser that sets a row shorter than the native one (its own font metrics) does not
	// stop the export: the loss is reported in the verification instead.
	if native := pixels(number(rect[3]) - number(rect[1])); number(rec.Fetch(observed, "height"))+tolerance < native {
		shrunkCells = append(shrunkCells, rec.Map{"table": r.table, "native_px": rec.Round(native, 4),
			"browser_px": rec.Round(number(rec.Fetch(observed, "height")), 4)})
	}
	rules := r.layout.RulesFor(cell)
	observedRules := rec.FetchList(rich, "rules")
	if len(observedRules) != len(rules) {
		rec.Fail("Browser changed rich cell border count")
	}
	for index, rule := range observedRules {
		r.verifyRule(rec.AsMap(rule), rules[index], observed)
	}
}

// cellMarkup is a table cell's element and attributes: a header cell (decisions 088, 131) is a
// TH with scope, any other a TD.
func cellMarkup(header bool, attributes []any) (string, []any) {
	if !header {
		return "TD", attributes
	}
	names := make([]string, 0, len(attributes)+1)
	for _, name := range attributes {
		names = append(names, rec.ToS(name))
	}
	names = append(names, "scope")
	sortStrings(names)
	return "TH", strings2any(names)
}

// formulaNamed adds the aria-label a header cell of formulas alone carries (decision 095, amendment 2).
func formulaNamed(header bool, cell rec.Map, attributes []any) []any {
	if !header || !tablelayout.FormulaCell(cell) {
		return attributes
	}
	names := []string{"aria-label"}
	for _, name := range attributes {
		names = append(names, rec.ToS(name))
	}
	sortStrings(names)
	return strings2any(names)
}

var ruleFields = []string{"metadata", "hidden", "visible", "position", "background", "text", "children"}

func (r *richCheck) verifyRule(observed, native, cell rec.Map) {
	expected := []any{native, "true", true, "absolute", r.color, "", int64(0)}
	differing := []string{}
	for index, field := range ruleFields {
		if !rec.Equal(observed[field], expected[index]) {
			differing = append(differing, fmt.Sprintf("%s %v, expected %v", field, observed[field], expected[index]))
		}
	}
	if len(differing) > 0 {
		rec.Fail("Browser changed rich table rule paint/identity (%s)", strings.Join(differing, ", "))
	}
	width, height := number(rec.Fetch(cell, "width")), number(rec.Fetch(cell, "height"))
	thickness, offset := pixels(rec.Fetch(native, "thickness_sp")), pixels(rec.Fetch(native, "offset_sp"))
	start, end := number(rec.Fetch(native, "start")), number(rec.Fetch(native, "end"))
	var box [4]float64
	if rec.Fetch(native, "axis") == "horizontal" {
		edge := height
		if rec.Fetch(native, "side") == "top" {
			edge = 0
		}
		box = [4]float64{width * start, edge + offset - float64(thickness/2), width * (end - start), thickness}
	} else {
		edge := width
		if rec.Fetch(native, "side") == "left" {
			edge = 0
		}
		box = [4]float64{edge + offset - float64(thickness/2), height * start, thickness, height * (end - start)}
	}
	// A rule is placed from its cell's edge: two boxes, each snapped to a layout unit.
	for index, key := range []string{"left", "top", "width", "height"} {
		richMeasure(observed, key, box[index], 2*tolerance)
	}
}

// richCellMarkup are a rich cell's attributes: a cell with a colortbl fill of its own is marked
// as a fill box (data-fill-box), which each theme paints.
func richCellMarkup(cell rec.Map) []any {
	if contrast.CellFill(cell) == "" {
		return richCellAttributes
	}
	names := []string{"data-fill-box"}
	for _, name := range richCellAttributes {
		names = append(names, rec.ToS(name))
	}
	sortStrings(names)
	return strings2any(names)
}

// cellBackground is the background the light theme paints a rich cell with: its colortbl fill,
// the PDF's paint, or none.
func cellBackground(cell rec.Map) string {
	if fill := contrast.CellFill(cell); fill != "" {
		return cssRGB(fill)
	}
	return "rgba(0, 0, 0, 0)"
}
