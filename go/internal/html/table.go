package html

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
	"github.com/sarielhp/relatex/internal/tablelayout"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// TableGridSpan is PaperTableHTML#table_grid_span: a classic grid as positioned cells, a
// rich grid by its layout.
func (p *Paper) TableGridSpan(span rec.Map) string {
	grid := rec.FetchMap(span, "grid")
	// A projected classic grid's horizontal borders are built in the order
	// ClassicTableProjection writes their keys; a published grid's are read sorted.
	projected := false
	headers := tablelayout.NewLayout(tablelayout.NewTable(grid, p.nodes))
	if grid["schema_version"] == tablemodel.Version {
		classic := tablelayout.ClassicProjection(grid)
		if classic == nil {
			return p.richTableSpan(span)
		}
		grid, projected = classic, true
	}
	color := ""
	if paint := p.tablePaint(span); paint != nil {
		color = ";color:" + p.inkColor(toS(rec.Fetch(paint, "color")))
	}
	var rows strings.Builder
	for index, row := range rec.FetchList(grid, "rows") {
		rows.WriteString(p.tableRow(grid, rec.AsList(row), index, projected, classicScopes(headers, index)))
	}
	return "<table" + layoutTableRole(rec.FetchMap(span, "grid"), p.nodes) + " id=\"" + toS(rec.Fetch(span, "id")) + "\"" + p.tableName(span) + " data-kind=\"table_grid\" data-preamble=\"" + toS(rec.Fetch(grid, "preamble")) +
		"\" data-width-sp=\"" + toS(rec.Fetch(grid, "width_sp")) + "\" style=\"border-collapse:collapse;border-spacing:0;table-layout:fixed;" +
		"width:" + tablePixels(rec.Fetch(grid, "width_sp")) + "px" + color + "\"><tbody>" + rows.String() + "</tbody></table>"
}

func (p *Paper) tablePaint(span rec.Map) rec.Map {
	paint, ok := p.document["table_paint"].(rec.Map)
	if !ok {
		return nil
	}
	tablesList, _ := paint["tables"].([]any)
	for _, entry := range tablesList {
		if item := rec.AsMap(entry); rec.Equal(rec.Fetch(item, "id"), rec.Fetch(span, "id")) {
			return item
		}
	}
	return nil
}

func (p *Paper) richTableSpan(span rec.Map) string {
	model := tablelayout.NewTable(rec.FetchMap(span, "grid"), p.nodes)
	grid := model.Data
	layout := tablelayout.NewLayout(model)
	width := "width:" + tablePixels(rubynum.Sum(layout.ColumnWidths)) + "px;"
	headerRows := rec.ListOf(grid["header_rows"])
	var rows strings.Builder
	for _, entry := range layout.DisplayRows() {
		var cells strings.Builder
		for _, cell := range entry.Cells {
			cells.WriteString(p.richTableCell(cell, layout))
		}
		role := ""
		if slices.ContainsFunc(headerRows, func(value any) bool { return rec.Equal(value, int64(entry.Row)) }) {
			role = ` data-header-row="true"`
		}
		rows.WriteString("<tr data-row=\"" + itoa(entry.Row) + "\" data-page=\"" + toS(entry.Page) + "\" data-occurrence=\"" +
			itoa(entry.Occurrence) + "\" data-role=\"" + htmlkit.Escape(toS(entry.Role)) + "\" style=\"height:" +
			tablePixels(layout.RowHeights[entry.Row]) + "px\"" + role + ">" + cells.String() + "</tr>")
	}
	var columns strings.Builder
	for column, value := range layout.ColumnWidths {
		columns.WriteString("<col data-width-sp=\"" + toS(value) + "\" style=\"width:" +
			tableDistance(rubynum.Sum(layout.ColumnWidths[:column]), value) + "px\"/>")
	}
	columnGroup := columns.String()
	if columnGroup != "" {
		columnGroup = "<colgroup>" + columnGroup + "</colgroup>"
	}
	metadata, err := canonjson.Marshal(layout.BrowserGrid())
	if err != nil {
		rec.Fail("%v", err)
	}
	color := "inherit"
	if paint := p.tablePaint(span); paint != nil {
		color = p.inkColor(toS(rec.Fetch(paint, "color")))
	}
	return "<table" + layoutTableRole(rec.FetchMap(span, "grid"), p.nodes) + " id=\"" + htmlkit.Escape(rec.FetchString(span, "id")) + "\"" + p.tableName(span) + " data-kind=\"table_grid\" data-schema=\"" + tablemodel.Version + "\" " +
		"data-grid-meta=\"" + htmlkit.Escape(string(metadata)) + "\" " +
		"style=\"" + width + "border-collapse:collapse;border-spacing:0;table-layout:fixed;color:" + color + "\">" +
		columnGroup + "<tbody>" + rows.String() + "</tbody></table>"
}

var (
	horizontalAlignments = map[string]string{"l": "left", "c": "center", "r": "right"}
	verticalAlignments   = map[string]string{"middle": "middle", "top": "top", "bottom": "bottom"}
)

func (p *Paper) richTableCell(cell rec.Map, layout *tablelayout.Layout) string {
	horizontal := tablelayout.HorizontalAlignment(cell)
	vertical, ok := verticalAlignments[toS(rec.FetchOr(cell, "vertical_align", "top"))]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rubyInspect(toS(cell["vertical_align"])), Lookup: true})
	}
	if !slices.Contains([]string{"left", "center", "right"}, horizontal) {
		rec.Fail("Unknown rich table alignment")
	}
	padding := tablePixels(rec.FetchOr(cell, "padding_sp", int64(0)))
	wrapping := "nowrap"
	if truthy(rec.FetchOr(cell, "wrap", true)) {
		wrapping = "normal"
	}
	// The cell's padding is its content's margin: Chromium widens spanning columns by fractions
	// of a pixel to distribute a spanning cell's own padding.
	style := "position:relative;box-sizing:border-box;border:0;min-width:0;width:0;padding:0;" +
		"vertical-align:" + vertical + ";text-align:" + horizontal + ";white-space:" + wrapping + ";line-height:normal;"
	content, fillBox := p.cellContent(cell)
	rect := layout.CellRect(cell)
	available := tableCoordinate(rect[2]) - tableCoordinate(rect[0]) - 2*parseFloat(padding)
	contentStyle := "width:" + fixed(max(available, 0)) + "px;min-width:0;margin:0 " + padding + "px" + targetHeight(content)
	metadata, err := canonjson.Marshal(layout.BrowserCell(cell))
	if err != nil {
		rec.Fail("%v", err)
	}
	var rules strings.Builder
	for _, rule := range layout.RulesFor(cell) {
		rules.WriteString(richTableRule(rule))
	}
	tag, scope, named := "td", "", ""
	if header := layout.CellScope(cell); header != "" {
		tag, scope = "th", headerScope(header)+" "
		if name := p.formulaCellName(cell); name != "" {
			scope += "aria-label=\"" + htmlkit.Escape(name) + "\" "
			named = formulaNameSpan(name)
		}
	}
	return "<" + tag + " " + scope + fillBox + "data-rich-cell=\"" + htmlkit.Escape(rec.FetchString(cell, "id")) + "\" rowspan=\"" + toS(rec.Fetch(cell, "rowspan")) + "\" " +
		"colspan=\"" + toS(rec.Fetch(cell, "colspan")) + "\" data-cell-meta=\"" + htmlkit.Escape(string(metadata)) + "\" " +
		"style=\"" + style + "\"><div data-cell-content style=\"" + contentStyle + "\">" + content + "</div>" + rules.String() + named + "</" + tag + ">"
}

// formulaCellName is the name of a header cell holding formulas alone (decision 095, amendment 2): the
// formulas' alttext (the linear form of decision 088, which the tagged PDF's Formula /Alt holds too),
// else the characters they show. Not the TeX annotation: it exists only where author math ran.
func (p *Paper) formulaCellName(cell rec.Map) string {
	if !tablelayout.FormulaCell(cell) {
		return ""
	}
	names := []string{}
	for _, value := range rec.FetchList(cell, "content") {
		span := rec.AsMap(value)
		if span["type"] != "math" {
			continue
		}
		node := p.node(rec.FetchString(span, "node"))
		name := mathexport.AltText(mathexport.ParsePaper(rec.FetchString(node, "mathml")))
		if name == "" {
			name = visibleText(p.renderMath(rec.FetchString(span, "node")))
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, " ")
}

// formulaNameSpan is the header's name as clipped text, the visible text Ace's empty-table-header
// rule needs (it reads no text from MathML and none from aria-label).
func formulaNameSpan(name string) string {
	return "<span data-header-name style=\"position:absolute;width:1px;height:1px;margin:-1px;overflow:hidden;" +
		"clip:rect(0 0 0 0);white-space:nowrap\">" + htmlkit.Escape(name) + "</span>"
}

// minimumTarget is WCAG 2.2's minimum target size (2.5.8) in CSS pixels.
const minimumTarget = "24px"

// targetHeight is the extra style of a rich cell's content that holds a link. A row as short as
// the TeX row (about 23 px for a 10 pt line) puts a column of links less than 24 px apart, so the
// content is made at least the minimum target tall. The cell only grows, which the rich table
// check allows (it reports only cells shorter than their native height).
func targetHeight(content string) string {
	if !strings.Contains(content, " href=\"") {
		return ""
	}
	return ";min-height:" + minimumTarget
}

func richTableRule(rule rec.Map) string {
	start := rubynum.Mul(rec.Fetch(rule, "start"), int64(100))
	length := rubynum.Mul(rubynum.Sub(rec.Fetch(rule, "end"), rec.Fetch(rule, "start")), int64(100))
	thickness := rubynum.Float(rec.Fetch(rule, "thickness_sp")) * 96.0 / (65_536 * texPoints())
	offset := rubynum.Float(rec.Fetch(rule, "offset_sp")) * 96.0 / (65_536 * texPoints())
	side := rec.FetchString(rule, "side")
	if side == "bottom" || side == "right" {
		offset = -offset
	}
	offset -= float64(thickness / 2)
	size := "top:" + toS(start) + "%;height:" + toS(length) + "%;width:" + toS(thickness) + "px"
	if rule["axis"] == "horizontal" {
		size = "left:" + toS(start) + "%;width:" + toS(length) + "%;height:" + toS(thickness) + "px"
	}
	style := "position:absolute;display:block;background:currentColor;pointer-events:none;" + size + ";" + side + ":" + toS(offset) + "px"
	encoded, err := canonjson.Marshal(rule)
	if err != nil {
		rec.Fail("%v", err)
	}
	return "<span data-rich-rule=\"" + htmlkit.Escape(string(encoded)) + "\" aria-hidden=\"true\" style=\"" + style + "\"></span>"
}

func (p *Paper) tableRow(grid rec.Map, row []any, index int, projected bool, scope func(column int) string) string {
	gap := tableRowGap(grid, index)
	originY := tableRowOrigin(grid, index)
	var cells strings.Builder
	widths := []any{}
	for column, value := range row {
		cell := rec.AsMap(value)
		originX := rubynum.Sum(widths)
		cells.WriteString(p.tableCell(grid, cell, cellPlace{index, column, gap, originX, originY, projected, scope(column)}))
		widths = append(widths, rec.Fetch(cell, "width_sp"))
	}
	return "<tr>" + cells.String() + "</tr>"
}

func tableRowGap(grid rec.Map, index int) any {
	heights := []any{}
	for _, value := range rec.ListOf(grid["horizontal_borders"]) {
		border := rec.AsMap(value)
		if rec.Equal(rec.Fetch(border, "after_row"), int64(index+1)) {
			heights = append(heights, rec.Fetch(border, "height"))
		}
	}
	return rubynum.Sum(heights)
}

func tableRowOrigin(grid rec.Map, index int) any {
	values := []any{}
	for ordinal, value := range rec.FetchList(grid, "rows")[:index] {
		first := rec.AsMap(rec.AsList(value)[0])
		values = append(values, rubynum.Add(rubynum.Add(rec.Fetch(first, "height_sp"), rec.Fetch(first, "depth_sp")), tableRowGap(grid, ordinal)))
	}
	return rubynum.Sum(values)
}

var letters = regexp.MustCompile(`[lcr]`)

// cellPlace is where a classic cell sits: its row and column, the row's border gap and the
// cell's origin.
type cellPlace struct {
	row, column           int
	gap, originX, originY any
	projected             bool
	scope                 string // the header scope its place gives a written cell (decisions 088, 131)
}

// headerScope is a header cell's scope attribute: col for a column header (decision 088), row for
// a row header (decision 131).
func headerScope(scope string) string { return `scope="` + scope + `"` }

// classicScopes is the header scope of each column of a classic row: col throughout a header row,
// row in a header column below them (tablelayout.CellScope, which the projection's one-column text
// cells meet when they hold text).
func classicScopes(layout *tablelayout.Layout, row int) func(column int) string {
	if slices.Contains(layout.HeaderRows(), row) {
		return func(int) string { return tablelayout.ScopeColumn }
	}
	columns := layout.HeaderColumns()
	return func(column int) string {
		if slices.Contains(columns, column) {
			return tablelayout.ScopeRow
		}
		return ""
	}
}

func (p *Paper) tableCell(grid, cell rec.Map, place cellPlace) string {
	row, column, gap, originX, originY := place.row, place.column, place.gap, place.originX, place.originY
	height := rubynum.Add(rec.Fetch(cell, "height_sp"), rec.Fetch(cell, "depth_sp"))
	alignments := letters.FindAllString(rec.FetchString(grid, "preamble"), -1)
	if column >= len(alignments) {
		panic(rec.Failure{Message: "index outside of array bounds", Lookup: true})
	}
	style := "position:relative;box-sizing:border-box;padding:0;border:0;vertical-align:top;white-space:nowrap;" +
		"width:" + tableDistance(originX, rec.Fetch(cell, "width_sp")) + "px;height:" + tableDistance(originY, rubynum.Add(height, gap)) +
		"px;text-align:" + horizontalAlignments[alignments[column]]
	textStyle := "display:block;height:" + tableDistance(originY, height) + "px;line-height:" + tableDistance(originY, height) + "px;" +
		"font-size:" + tablePixels(rec.Fetch(cell, "font_size_sp")) + "px"
	attributes := []string{}
	for _, key := range []string{"width_sp", "height_sp", "depth_sp", "font_size_sp"} {
		attributes = append(attributes, "data-"+strings.ReplaceAll(key, "_", "-")+"=\""+toS(rec.Fetch(cell, key))+"\"")
	}
	var borders strings.Builder
	for _, value := range rec.FetchList(cell, "borders") {
		border := rec.AsMap(value)
		borders.WriteString(tableRule("vertical", border, sortedNames(border), ruleBox{rec.Fetch(border, "x"), int64(0), height, originX, originY}))
	}
	if column == 0 {
		borders.WriteString(horizontalTableRules(grid, row, height, originY, place.projected))
	}
	text := ""
	if runs, ok := cell["runs"]; ok {
		// The projection is TeX pixels throughout: its runs keep their TeX size.
		p.absolute = true
		for _, run := range rec.AsList(runs) {
			text += p.renderSpan(rec.Merge(rec.AsMap(run), rec.Map{"type": "text"}))
		}
		p.absolute = false
	} else {
		text = htmlkit.Escape(rec.FetchString(cell, "text"))
	}
	tag := "td"
	if place.scope != "" && strings.TrimFunc(rec.FetchString(cell, "text"), unicode.IsSpace) != "" {
		tag, attributes = "th", append(attributes, headerScope(place.scope))
	}
	return "<" + tag + " " + strings.Join(attributes, " ") + " style=\"" + style + "\"><span data-cell-text style=\"" + textStyle + "\">" +
		text + "</span>" + borders.String() + "</" + tag + ">"
}

func horizontalTableRules(grid rec.Map, row int, height, originY any, projected bool) string {
	var builder strings.Builder
	for _, value := range rec.ListOf(grid["horizontal_borders"]) {
		border := rec.AsMap(value)
		after := rec.FetchInt(border, "after_row")
		if !(after == 0 && row == 0) && after != int64(row+1) {
			continue
		}
		top := height
		if after == 0 {
			top = rubynum.Mul(rec.Fetch(border, "height"), int64(-1))
		}
		names := sortedNames(border)
		if projected {
			names = []string{"after_row", "width", "height", "depth"}
		}
		builder.WriteString(tableRule("horizontal", border, names, ruleBox{int64(0), top, rec.Fetch(border, "height"), int64(0), originY}))
	}
	return builder.String()
}

func sortedNames(border rec.Map) []string {
	keys := make([]string, 0, len(border))
	for key := range border {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// ruleBox is a rule's position and height, and the origin it is measured from.
type ruleBox struct{ left, top, height, originX, originY any }

func tableRule(kind string, border rec.Map, keys []string, box ruleBox) string {
	left, top, height, originX, originY := box.left, box.top, box.height, box.originX, box.originY
	attributes := []string{}
	for _, key := range keys {
		attributes = append(attributes, "data-"+strings.ReplaceAll(key, "_", "-")+"=\""+toS(border[key])+"\"")
	}
	style := "position:absolute;display:block;background:currentColor;pointer-events:none;" +
		"left:" + tableDistance(originX, left) + "px;top:" + tableDistance(originY, top) + "px;" +
		"width:" + tableDistance(rubynum.Add(originX, left), rec.Fetch(border, "width")) + "px;height:" + tableDistance(rubynum.Add(originY, top), height) + "px"
	return "<span data-rule=\"" + kind + "\" " + strings.Join(attributes, " ") + " aria-hidden=\"true\" style=\"" + style + "\"></span>"
}

func tablePixels(value any) string { return fixed(tableCoordinate(value)) }

func tableDistance(start, length any) string {
	return fixed(tableCoordinate(rubynum.Add(start, length)) - tableCoordinate(start))
}

// tableCoordinate is (value * 96.0 * 64 / (65_536 * 72.27)).round / 64.0.
func tableCoordinate(value any) float64 {
	scaled := rubynum.Float(value) * 96.0 * 64 / (65_536 * texPoints())
	return float64(roundInteger(scaled)) / 64.0
}

func parseFloat(text string) float64 {
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		rec.Fail("%v", err)
	}
	return value
}

func itoa(value int) string { return toS(int64(value)) }
