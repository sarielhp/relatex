package tablelayout

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
)

// Column header rows (decision 088). A table's header rows are a longtable's declared head, or
// the rows above the first full-width rule when that rule sets them apart from the body; a cell
// of those rows holding text renders as <th scope="col">. A table the source does not mark this
// way has no header cells: a layout is never guessed into a data table. The acceptance reads the
// rule on its own (lib/relatex/table_header_rows.rb).
const (
	// headerCoverage is the share of the table's width a full-width rule covers.
	headerCoverage = 0.98
	// lineQuantum (0.25 pt) merges rule centres into one line of a double rule.
	lineQuantum = 16_384.0
	// strongerRule is how much thicker a rule must be to set its rows apart.
	strongerRule = 1.2
)

var headerTextTypes = []string{"text", "math", "reference", "note_reference", "symbol"}

// boundaryRule is a horizontal rule segment at a row boundary: its extent across the table,
// its line (the side and centre it was placed by) and its thickness.
type boundaryRule struct {
	from, to  float64
	side      string
	centre    float64
	thickness float64
}

// How a table's header rows are known (HeaderSource): the author's hint (decision 131), the
// tagging project's header-rows setting, longtable's declared head, booktabs' \midrule under a
// heavier \toprule (decision 131), a rule that sets them apart, \multicolumn group headings over a
// row of column labels (decision 131), or a bold first row over a body that is not bold (decision
// 111, inferred).
const (
	HeaderAuthor    = "author"
	HeaderTagging   = "tagging"
	HeaderLongtable = "longtable"
	HeaderBooktabs  = "booktabs"
	HeaderRule      = "rule"
	HeaderSpan      = "span"
	HeaderBold      = "bold"
)

// The author's header rows and columns (decision 131): the grid keys tablehints writes from
// relatex/hints.json or a %% relatex tables: comment. A key present is the author's word, even an
// empty list (no headers); typesetting is then not read for that direction.
const (
	AuthorHeaderRows    = "author_header_rows"
	AuthorHeaderColumns = "author_header_columns"
)

// rowReader is one way to know the header rows, tried in order until one finds rows.
type rowReader struct {
	source string
	read   func(*Layout) []int
}

var rowReaders = []rowReader{
	{HeaderTagging, (*Layout).taggedHeaderRows},
	{HeaderLongtable, (*Layout).declaredHeaderRows},
	{HeaderBooktabs, (*Layout).booktabsHeaderRows},
	{HeaderRule, (*Layout).ruledHeaderRows},
	{HeaderSpan, (*Layout).spanHeaderRows},
	{HeaderBold, (*Layout).boldHeaderRows},
	{HeaderCorner, (*Layout).cornerHeaderRows},
	{HeaderNumeric, (*Layout).numericHeaderRows},
	{HeaderInferred, (*Layout).inferredHeaderRows},
}

// inferredRows are the first rows a reader in rowReaders finds, with its source; a head a rule ends is
// extended to its label row (withLabelRow).
func (l *Layout) inferredRows() ([]int, string) {
	for _, reader := range rowReaders {
		rows := reader.read(l)
		if len(rows) == 0 {
			continue
		}
		if reader.source == HeaderBooktabs || reader.source == HeaderRule {
			rows = l.withLabelRow(rows)
		}
		return rows, reader.source
	}
	return nil, ""
}

// HeaderRows are the table's column header rows, in order; none when the source marks none.
func (l *Layout) HeaderRows() []int {
	if l.headers != nil {
		return *l.headers
	}
	rows, source := authorIndices(l.Model.Data, AuthorHeaderRows)
	if rows == nil {
		rows, source = l.inferredRows()
	}
	if !l.textualRows(rows) {
		rows, source = []int{}, ""
	}
	l.headers, l.headerSource = &rows, source
	return rows
}

// authorIndices are the rows or columns a grid's author key names, with HeaderAuthor; nil when the
// author gave none.
func authorIndices(grid rec.Map, key string) ([]int, string) {
	values, ok := grid[key].([]any)
	if !ok {
		return nil, ""
	}
	indices := []int{}
	for _, value := range values {
		indices = append(indices, int(rec.AsInt(value)))
	}
	return indices, HeaderAuthor
}

// HeaderSource is how the header rows are known (one of the Header* sources); "" when the table
// has none.
func (l *Layout) HeaderSource() string {
	l.HeaderRows()
	return l.headerSource
}

// HeaderSourceOf is how a grid's header cells are known, "" without one: the source of its column
// headers, else of its row headers. It is the loop the accessibility report read locally, shared
// so the page writer's grid role and the report agree: a grid the reader cannot model, or one
// whose header rows and columns hold no header cell, has none.
func HeaderSourceOf(grid, nodes rec.Map) (source string) {
	defer func() {
		if recover() != nil {
			source = ""
		}
	}()
	layout := NewLayout(NewTable(grid, nodes))
	row := ""
	for _, cell := range rec.FetchList(layout.Model.Data, "cells") {
		switch layout.CellScope(rec.AsMap(cell)) {
		case ScopeColumn:
			return layout.HeaderSource()
		case ScopeRow:
			row = layout.HeaderColumnSource()
		}
	}
	return row
}

// HeaderScopes are the scopes of a grid's header cells by cell id (CellScope), for a writer that
// does not hold a Layout (the tagged PDF); none when the reader cannot model the grid.
func HeaderScopes(grid, nodes rec.Map) (scopes map[string]string) {
	defer func() {
		if recover() != nil {
			scopes = nil
		}
	}()
	layout := NewLayout(NewTable(grid, nodes))
	scopes = map[string]string{}
	for _, value := range rec.FetchList(layout.Model.Data, "cells") {
		cell := rec.AsMap(value)
		if scope := layout.CellScope(cell); scope != "" {
			scopes[rec.FetchString(cell, "id")] = scope
		}
	}
	return scopes
}

// PresumedLayout reports whether the grid has one row or one column (a grid that does not count them is not),
// or is a panel layout of figures (panelGrid). No header can relate such a table's data, so decision 111 (2)
// presumes it a layout: the page writes it role="presentation" and the accessibility report counts it
// layout_presumed.
func PresumedLayout(grid rec.Map) bool {
	rows, counted := grid["row_count"].(int64)
	columns, both := grid["column_count"].(int64)
	return counted && both && (rows < 2 || columns < 2 || panelGrid(grid, int(rows)))
}

// PanelReason says why a grid that holds pictures is read as a table, not as an arrangement of
// figures (panelGrid): the first row that breaks the rule. It is "" for a grid without a picture and
// for one that is an arrangement.
func PanelReason(grid rec.Map) string {
	rows, _ := grid["row_count"].(int64)
	kinds := panelRowKinds(grid, int(rows))
	if !slices.Contains(kinds, panelPictures) && !slices.Contains(kinds, panelMixed) {
		return ""
	}
	previous := panelEmpty
	for index, kind := range kinds {
		switch {
		case kind == panelPictures:
		case kind == panelEmpty:
			continue
		case kind == panelText && previous == panelPictures:
		case kind == panelMixed:
			return fmt.Sprintf("row %d mixes pictures with text that is neither a short label, a panel letter or a formula alone beside them nor a connector between them", index+1)
		default:
			return fmt.Sprintf("row %d is text that does not sit directly under a row of pictures", index+1)
		}
		previous = kind
	}
	return ""
}

// The kinds of a grid row for panelGrid, from its cells': no content, pictures, text, or both.
const (
	panelEmpty = iota
	panelPictures
	panelText
	panelMixed
)

// panelGrid: the grid arranges figures (decision 167 point 6, the export's side of the lint's judgement):
// every row holds pictures (each cell with content an image or a nested table only, or with a panel letter,
// a connector or a label, see panelRowKind), or is a row of text directly under a row of pictures (their
// captions), or holds nothing; one row of pictures at least. A text row anywhere else, or pictures beside
// text in one row, keeps the table.
func panelGrid(grid rec.Map, rows int) bool {
	pictures, previous := false, panelEmpty
	for _, kind := range panelRowKinds(grid, rows) {
		switch {
		case kind == panelPictures:
			pictures = true
		case kind == panelEmpty:
			continue
		case kind == panelText && previous == panelPictures:
		default:
			return false
		}
		previous = kind
	}
	return pictures
}

// panelPart is one cell with content in a grid row: its column, its kind (panelCell), and whether it reads
// as a connector (an arrow or operator between panels), a short label, a panel letter, or a formula alone.
type panelPart struct {
	column                            int
	kind                              int
	connector, label, letter, formula bool
}

// labelRunes is the longest text that names a panel as a row label ("(a)", "Input", "Ours").
const labelRunes = 24

// panelRowKinds are the kinds of the grid's rows (panelRowKind).
func panelRowKinds(grid rec.Map, rows int) []int {
	parts := make([][]panelPart, rows)
	for _, value := range rec.FetchList(grid, "cells") {
		cell := rec.AsMap(value)
		row, _ := rec.Number(cell["row"])
		column, _ := rec.Number(cell["column"])
		content := rec.FetchList(cell, "content")
		index, kind := int(row), panelCell(content)
		if index < 0 || index >= rows || kind == panelEmpty {
			continue
		}
		parts[index] = append(parts[index], panelPart{column: int(column), kind: kind,
			connector: connectorCell(content), label: labelCell(content), letter: panelLetter(content),
			formula: formulaCell(content)})
	}
	kinds := make([]int, rows)
	for index, row := range parts {
		kinds[index] = panelRowKind(row)
	}
	return kinds
}

// panelRowKind is a row's kind: empty without content, text when it holds no picture, mixed when a cell
// holds both or a picture sits beside text that is neither a connector between the row's pictures (an arrow
// or operator glyph, or a formula alone in its cell), nor a panel letter outside them, nor a short label
// outside them (a row of at least two pictures), nor a formula alone beside them (a panel of the
// arrangement), else pictures.
func panelRowKind(row []panelPart) int {
	if len(row) == 0 {
		return panelEmpty
	}
	first, last, pictures := math.MaxInt, -1, 0
	for _, part := range row {
		if part.kind == panelPictures {
			first, last, pictures = min(first, part.column), max(last, part.column), pictures+1
		}
	}
	for _, part := range row {
		switch {
		case pictures == 0 && part.kind == panelMixed:
			return panelMixed
		case pictures == 0 || part.kind == panelPictures:
		case part.kind != panelText:
			return panelMixed
		case part.column > first && part.column < last:
			if !part.connector {
				return panelMixed
			}
		case !panelBeside(part, pictures):
			return panelMixed
		}
	}
	if pictures == 0 {
		return panelText
	}
	return panelPictures
}

// panelBeside: a cell outside the row's pictures keeps the row a row of pictures when it names a panel (a
// letter, "(A)"), or is a formula alone or a short label beside a row of at least two pictures.
func panelBeside(part panelPart, pictures int) bool {
	switch {
	case part.letter:
		return true
	case part.formula || part.label:
		return pictures >= 2
	}
	return false
}

// connectorCell: the cell's content is one formula alone, or text of at most three arrow or operator glyphs
// and no letter or digit.
func connectorCell(content []any) bool {
	written, symbols := 0, true
	for _, value := range content {
		span := rec.AsMap(value)
		switch rec.FetchString(span, "type") {
		case "math":
			written++
		case "text":
			text := strings.TrimSpace(rec.FetchString(span, "text"))
			if text == "" {
				continue
			}
			written++
			symbols = symbols && utf8.RuneCountInString(text) <= 3 && !strings.ContainsFunc(text, notGlyph)
		default:
			return false
		}
	}
	return written == 1 && symbols
}

// formulaCell: the cell's content is one formula alone (a panel of the arrangement, or a connector).
func formulaCell(content []any) bool {
	written := 0
	for _, value := range content {
		span := rec.AsMap(value)
		switch rec.FetchString(span, "type") {
		case "math":
			written++
		case "text":
			if strings.TrimSpace(rec.FetchString(span, "text")) == "" {
				continue
			}
			return false
		default:
			return false
		}
	}
	return written == 1
}

// panelLetter: the cell's text names a panel by one letter in parentheses ("(A)", "(a)"), the form the source
// lint reads as a label beside its picture (go/internal/a11ylint/layout.go, cellLabel, for one letter). A
// number, a word ("Cat") and a multi-letter label ("(iii)") are data here: a grid with words or numbers in
// two or more rows of non-picture cells stays a data table, and a caption row under the pictures is read as
// their captions, not as panel letters. A picture in the cell is no text of the name.
func panelLetter(content []any) bool {
	text := ""
	for _, value := range content {
		span := rec.AsMap(value)
		switch rec.FetchString(span, "type") {
		case "image", "table_grid":
		case "text":
			text += rec.FetchString(span, "text")
		default:
			return false
		}
	}
	trimmed := strings.TrimSpace(text)
	if len(trimmed) != 3 || trimmed[0] != '(' || trimmed[2] != ')' {
		return false
	}
	c := trimmed[1]
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// notGlyph: the rune is no symbol (math or other), so no arrow or operator.
func notGlyph(r rune) bool { return !unicode.IsOneOf([]*unicode.RangeTable{unicode.Sm, unicode.So}, r) }

// labelCell: the cell's content is text only, at most labelRunes long, with a letter or digit.
func labelCell(content []any) bool {
	text := ""
	for _, value := range content {
		span := rec.AsMap(value)
		if rec.FetchString(span, "type") != "text" {
			return false
		}
		text += rec.FetchString(span, "text")
	}
	text = strings.TrimSpace(text)
	return utf8.RuneCountInString(text) <= labelRunes && strings.ContainsFunc(text, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}

// panelCell is a cell's kind: blank text only is empty, images and nested tables (with blank text or a panel
// letter) are pictures, anything else text, both mixed.
func panelCell(content []any) int {
	pictures, text := false, false
	for _, value := range content {
		span := rec.AsMap(value)
		switch rec.FetchString(span, "type") {
		case "image", "table_grid":
			pictures = true
		case "text":
			text = text || strings.TrimSpace(rec.FetchString(span, "text")) != ""
		default:
			text = true
		}
	}
	switch {
	case pictures && text && panelLetter(content):
		return panelPictures
	case pictures && text:
		return panelMixed
	case pictures:
		return panelPictures
	case text:
		return panelText
	}
	return panelEmpty
}

// Presentational is a grid the page writes role="presentation": a presumed layout whose header
// cells are not known. A grid with an inferred or marked header row stays a data table.
func Presentational(grid, nodes rec.Map) bool {
	return PresumedLayout(grid) && HeaderSourceOf(grid, nodes) == ""
}

// The scope of a header cell: a column header (<th scope="col">, PDF /Scope /Column) or a row
// header (<th scope="row">, /Scope /Row).
const (
	ScopeColumn = "col"
	ScopeRow    = "row"
)

// CellScope is a cell's header scope, "" for a data cell. A cell of the header rows is a column
// header, else a cell of the header columns a row header; either holds text or a formula. A cell
// of formulas alone is a header too, its name added by the writers (decision 095, amendment 2:
// Ace reads MathML alone as an empty header).
func (l *Layout) CellScope(cell rec.Map) string {
	if !textCell(cell) && !FormulaCell(cell) {
		return ""
	}
	if within(l.HeaderRows(), cell, "row", "rowspan") {
		return ScopeColumn
	}
	if within(l.HeaderColumns(), cell, "column", "colspan") {
		return ScopeRow
	}
	return ""
}

// HeaderCell tells whether a cell renders as <th>: a column or a row header (CellScope).
func (l *Layout) HeaderCell(cell rec.Map) bool {
	return l.CellScope(cell) != ""
}

// within: the cell's first and last row (or column) are both among indices.
func within(indices []int, cell rec.Map, start, extent string) bool {
	first := int(rec.FetchInt(cell, start))
	last := first + int(rec.FetchInt(cell, extent)) - 1
	return slices.Contains(indices, first) && slices.Contains(indices, last)
}

// taggedHeaderRows are the rows \tagpdfsetup{table/header-rows={…}} names for the table (decision
// 111, the grid's declared_header_rows).
func (l *Layout) taggedHeaderRows() []int {
	rows := []int{}
	for _, value := range rec.ListOf(l.Model.Data["declared_header_rows"]) {
		rows = append(rows, int(rec.AsInt(value)))
	}
	return rows
}

// declaredHeaderRows are longtable's \endfirsthead rows, or its \endhead rows without a first head.
func (l *Layout) declaredHeaderRows() []int {
	for _, role := range []string{"first_head", "repeat_head"} {
		rows := []int{}
		for row, cells := range l.Model.Rows {
			if rowRole(cells) == role {
				rows = append(rows, row)
			}
		}
		if len(rows) > 0 {
			return rows
		}
	}
	return nil
}

func (l *Layout) ruledHeaderRows() []int {
	count := int(rec.FetchInt(l.Model.Data, "row_count"))
	if count < 2 || rec.FetchInt(l.Model.Data, "column_count") < 2 {
		return nil
	}
	boundaries := l.boundaryRules()
	for _, head := range []int{1, 2} {
		if head >= count || !l.fullRule(boundaries[head]) {
			continue
		}
		if !l.setApart(boundaries, head, count) {
			return nil
		}
		rows := make([]int, head)
		for index := range rows {
			rows[index] = index
		}
		return rows
	}
	return nil
}

// setApart: fewer than half of the body's row boundaries are ruled, or the head's rule is
// stronger than each of theirs; a single body row needs a head rule thinner than the top rule
// (booktabs' \midrule under \toprule).
func (l *Layout) setApart(boundaries map[int][]boundaryRule, head, count int) bool {
	if head+1 >= count {
		return l.fullRule(boundaries[0]) && ruleThickness(boundaries[head]) < ruleThickness(boundaries[0])
	}
	others, ruled, weaker := 0, 0, 0
	for boundary := head + 1; boundary < count; boundary++ {
		others++
		if !l.fullRule(boundaries[boundary]) {
			continue
		}
		ruled++
		if stronger(boundaries[head], boundaries[boundary]) {
			weaker++
		}
	}
	return ruled*2 < others || weaker == ruled
}

func stronger(head, other []boundaryRule) bool {
	return ruleLines(head) > ruleLines(other) || ruleThickness(head) > ruleThickness(other)*strongerRule
}

// fullRule: the boundary's rule segments cover the table's width.
func (l *Layout) fullRule(rules []boundaryRule) bool {
	return covers(rules, 0, rubynum.Float(rubynum.Sum(l.ColumnWidths)))
}

// covers: the rule segments cover the stretch from low to high (headerCoverage of it).
func covers(rules []boundaryRule, low, high float64) bool {
	spans := make([][2]float64, len(rules))
	for index, rule := range rules {
		spans[index] = [2]float64{max(rule.from, low), min(rule.to, high)}
	}
	sort.Slice(spans, func(a, b int) bool {
		return spans[a][0] < spans[b][0] || spans[a][0] == spans[b][0] && spans[a][1] < spans[b][1]
	})
	covered, reach := 0.0, math.Inf(-1)
	for _, span := range spans {
		from := math.Max(span[0], reach)
		if span[1] > from {
			covered += span[1] - from
		}
		reach = math.Max(reach, span[1])
	}
	return high > low && covered >= headerCoverage*(high-low)
}

func ruleLines(rules []boundaryRule) int {
	type line struct {
		side   string
		centre float64
	}
	seen := map[line]bool{}
	for _, rule := range rules {
		seen[line{rule.side, rule.centre}] = true
	}
	return len(seen)
}

func ruleThickness(rules []boundaryRule) float64 {
	thickness := 0.0
	for _, rule := range rules {
		thickness = math.Max(thickness, rule.thickness)
	}
	return thickness
}

// boundaryRules are the horizontal rule segments at each row boundary.
func (l *Layout) boundaryRules() map[int][]boundaryRule {
	return l.axisRules(horizontalAxis)
}

// A rule's axis and the cell fields that place it: a horizontal rule lies at a row boundary and
// runs across the cell's columns, a vertical one at a column boundary down the cell's rows.
type ruleAxis struct {
	name, start, extent, near string
	along                     [2]int // the cell rectangle's coordinates the rule runs between
}

var (
	horizontalAxis = ruleAxis{name: "horizontal", start: "row", extent: "rowspan", near: "top", along: [2]int{0, 2}}
	verticalAxis   = ruleAxis{name: "vertical", start: "column", extent: "colspan", near: "left", along: [2]int{1, 3}}
)

// axisRules are the rule segments of one axis at each boundary between rows (or columns), with
// their extent along the table.
func (l *Layout) axisRules(axis ruleAxis) map[int][]boundaryRule {
	result := map[int][]boundaryRule{}
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		rect := l.CellRect(cell)
		low, high := rubynum.Float(rect[axis.along[0]]), rubynum.Float(rect[axis.along[1]])
		for _, rule := range l.RulesFor(cell) {
			if rule["axis"] != axis.name {
				continue
			}
			boundary := int(rec.FetchInt(cell, axis.start))
			if rule["side"] != axis.near {
				boundary += int(rec.FetchInt(cell, axis.extent))
			}
			result[boundary] = append(result[boundary], boundaryRule{
				from: low + float64(rubynum.Float(rule["start"])*(high-low)),
				to:   low + float64(rubynum.Float(rule["end"])*(high-low)),
				side: rec.AsString(rule["side"]), centre: math.Round(rubynum.Float(rule["offset_sp"]) / lineQuantum),
				thickness: rubynum.Float(rule["thickness_sp"])})
		}
	}
	return result
}

// textualRows: the rows hold text only (no image or nested table), and some text at all.
func (l *Layout) textualRows(rows []int) bool {
	return len(rows) > 0 && l.textual(func(cell rec.Map) bool { return slices.Contains(rows, int(rec.FetchInt(cell, "row"))) })
}

// textual: the cells member selects hold text only (no image or nested table), and some text at all.
func (l *Layout) textual(member func(rec.Map) bool) bool {
	written := false
	for _, value := range rec.FetchList(l.Model.Data, "cells") {
		cell := rec.AsMap(value)
		if !member(cell) {
			continue
		}
		for _, span := range rec.ListOf(cell["content"]) {
			if !slices.Contains(headerTextTypes, rec.AsString(rec.AsMap(span)["type"])) {
				return false
			}
		}
		written = written || !blankCell(cell)
	}
	return written
}

// cellSpans: whether the cell holds a text span with more than white space (written), and whether
// it holds any span that is not text (other).
func cellSpans(cell rec.Map) (written, other bool) {
	for _, value := range rec.ListOf(cell["content"]) {
		span := rec.AsMap(value)
		if span["type"] != "text" {
			other = true
			continue
		}
		text, _ := span["text"].(string)
		if strings.TrimFunc(text, unicode.IsSpace) != "" {
			written = true
		}
	}
	return written, other
}

// FormulaCell tells whether the cell holds a formula and nothing else but white space, the header cell whose
// name the writers take from its formulas (decision 095, amendment 2).
func FormulaCell(cell rec.Map) bool {
	formula := false
	for _, value := range rec.ListOf(cell["content"]) {
		span := rec.AsMap(value)
		switch span["type"] {
		case "math":
			formula = true
		case "text":
			if text, _ := span["text"].(string); strings.TrimFunc(text, unicode.IsSpace) != "" {
				return false
			}
		default:
			return false
		}
	}
	return formula
}

// textCell: the cell holds a text span with more than white space.
func textCell(cell rec.Map) bool {
	written, _ := cellSpans(cell)
	return written
}

// blankCell: the cell holds no text but white space.
func blankCell(cell rec.Map) bool {
	written, other := cellSpans(cell)
	return !written && !other
}
