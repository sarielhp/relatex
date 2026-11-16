package tables

import (
	"math"
	"runtime"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// RichSource is RichTableEvidence::SOURCE.
const RichSource = "native-array-cells"

var richCodes = []string{"rich_table_cell", "rich_table_row", "rich_table_rule", "rich_table_presence", "rich_table_glyphs",
	"rich_table_section", "rich_table_caption", "rich_table_nesting", "rich_table_declared_header"}

var alignments = map[string]string{"l": "l", "c": "c", "r": "r", "p": "l", "m": "l", "b": "l"}

var cellFields = []string{"id", "table", "row", "column", "kind", "colspan", "rowspan", "vertical_align", "padding_sp"}

// rich is RichTableEvidence: executed array cells reconciled with the independently
// retained scalar flow slots. Native fragments remain explicit: pagination is not inferred
// from source text.
type rich struct {
	records []rec.Map
	origins map[uintptr]rec.Map
	events  []rec.Map
	owners  map[string]rec.Map
	cells   map[string]rec.Map
	upward  []any
	clamped []any
	roles   map[string]string
	nesting map[string]rec.Map
	// declared are the tagging project's header-rows settings by table (decision 111).
	declared    map[string]string
	readOwners  []any
	blocks      []rec.Map
	diagnostics []rec.Map
	targets     []rec.Map
	// alive keeps every span whose identity is a key of origins reachable while the
	// projection runs, so that no new span can take the address of a dropped one.
	alive []rec.Map
	// droppedMath are the occurrences of formulas in unprinted repeat cells a grid replaced:
	// no printed node reads them, so the document builds none.
	droppedMath []any
}

// ProjectRich is RichTableEvidence.new(records, table_span_origins).project!(blocks,
// diagnostics, targets). origins maps rec.Identity(span) to the span's table origin
// ({table, copy, cell?}; a span not in it has none); it is updated as Ruby updates it. A
// malformed cell declaration fails the reading (it panics with rec.Failure, as Ruby's
// constructor raises); a table that cannot be modeled reads as text, with a diagnostic.
func ProjectRich(records []rec.Map, origins map[uintptr]rec.Map, blocks, diagnostics, targets []rec.Map) (newBlocks, newDiagnostics, newTargets []rec.Map, dropped []any, projected bool) {
	e := newRich(records, origins)
	e.blocks, e.diagnostics, e.targets = blocks, diagnostics, targets
	for _, block := range blocks {
		e.alive = append(e.alive, maps(tablemodel.FetchList(block, "content"))...)
	}
	projected = e.project()
	runtime.KeepAlive(e.alive)
	return e.blocks, e.diagnostics, e.targets, e.droppedMath, projected
}

func newRich(records []rec.Map, origins map[uintptr]rec.Map) *rich {
	e := &rich{records: records, origins: origins, owners: map[string]rec.Map{}, cells: map[string]rec.Map{},
		nesting: map[string]rec.Map{}, declared: map[string]string{}}
	for _, row := range records {
		if code, _ := row["code"].(string); row["kind"] == "diagnostic" && slices.Contains(richCodes, code) {
			e.events = append(e.events, row)
		}
		if row["kind"] == "flow_candidate" {
			e.owners[key(rec.Fetch(row, "owner"))] = row
		}
	}
	for _, row := range e.events {
		if row["code"] == "rich_table_cell" {
			e.declare(row)
		}
	}
	e.roles = e.cellRoles()
	for _, row := range e.events {
		if row["code"] == "rich_table_nesting" {
			e.nesting[key(rec.Fetch(row, "table"))] = row
		}
		if row["code"] == "rich_table_declared_header" {
			e.declared[key(rec.Fetch(row, "table"))] = rec.FetchString(row, "rows")
		}
	}
	return e
}

func (e *rich) project() (projected bool) {
	if len(e.events) == 0 {
		return false
	}
	message, failed := rescue(func() { projected = e.projectAll() })
	if failed {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "unsupported_table_layout", "source": RichSource, "reason": message})
		return false
	}
	return projected
}

func (e *rich) projectAll() bool {
	if !stream.Enabled(e.records[0], "tables-0.2") {
		rec.Fail("Rich tables require tables-0.2")
	}
	for _, row := range e.events {
		if row["source"] != RichSource {
			rec.Fail("Unrecognized rich table provenance")
		}
	}
	snapshots, expected := e.snapshots()
	for _, table := range uniqueValues(e.upward) {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "upward_multirow", "table": table})
	}
	// Each table models or falls back to plain text on its own; a nested table first, so its
	// grid (or its text) becomes the content of its parent's cell.
	grouped := map[string][]rec.Map{}
	tables := []any{}
	for _, row := range snapshots {
		table := rec.Fetch(row, "table")
		if _, ok := grouped[key(table)]; !ok {
			tables = append(tables, table)
		}
		grouped[key(table)] = append(grouped[key(table)], row)
	}
	for _, table := range expected {
		if !containsEql(tables, table) {
			tables = append(tables, table)
		}
	}
	projected := false
	for _, table := range e.nestedFirst(tables) {
		rows, ok := grouped[key(table)]
		if e.projectNested(table, rows, ok) {
			projected = true
		}
	}
	return projected
}

// snapshots is the captured native rows and the tables whose presence was recorded. A
// tabular holding only images (a grid of subfigures) has no glyphs or rules to place; its
// images read in the flow.
func (e *rich) snapshots() (kept []rec.Map, expected []any) {
	rows := []rec.Map{}
	for _, row := range e.events {
		if row["code"] == "rich_table_presence" && !containsEql(expected, rec.Fetch(row, "table")) {
			expected = append(expected, rec.Fetch(row, "table"))
		}
		if row["code"] == "rich_table_row" {
			rows = append(rows, fetchMap(row, "snapshot"))
		}
	}
	expected = tablemodel.SortValues(expected)
	for _, row := range rows {
		empty := true
		for _, cell := range maps(tablemodel.FetchList(row, "cells")) {
			empty = empty && toI(cell["glyphs"]) == 0 && toI(cell["rules"]) == 0
		}
		if !empty || contains(expected, rec.Fetch(row, "table")) {
			kept = append(kept, row)
		}
	}
	return kept, expected
}

// nestedFirst orders tables by nesting depth, deepest first, then by number.
func (e *rich) nestedFirst(tables []any) []any {
	depths := map[string]int{}
	for _, table := range tables {
		depths[key(table)] = e.depth(table)
	}
	slices.SortStableFunc(tables, func(a, b any) int {
		if order := depths[key(b)] - depths[key(a)]; order != 0 {
			return order
		}
		return compareNumbers(a, b)
	})
	return tables
}

// toI is #to_i on a count that may be missing.
func toI(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int64:
		return typed
	}
	return int64(number(value))
}

func (e *rich) cellRoles() map[string]string {
	result := map[string]string{}
	for _, row := range e.events {
		if row["code"] != "rich_table_section" {
			continue
		}
		role, _ := rec.Fetch(row, "role").(string)
		if !slices.Contains(tablemodel.SectionRoles, role) || role == "caption" || role == "body" {
			rec.Fail("Unknown native table section")
		}
		for _, id := range tablemodel.FetchList(row, "cells") {
			if !rec.Equal(rec.Fetch(e.cell(id), "table"), rec.Fetch(row, "table")) {
				rec.Fail("Section refers to a foreign table cell")
			}
			if prior, ok := result[key(id)]; ok && prior != role {
				rec.Fail("Cell has multiple native section roles")
			}
			result[key(id)] = role
		}
	}
	for _, row := range e.events {
		if row["code"] != "rich_table_caption" {
			continue
		}
		if !rec.Equal(rec.Fetch(e.cell(rec.Fetch(row, "cell")), "table"), rec.Fetch(row, "table")) {
			rec.Fail("Caption refers to a foreign table cell")
		}
		result[key(row["cell"])] = "caption"
	}
	return result
}

func (e *rich) cell(id any) rec.Map {
	cell, ok := e.cells[key(id)]
	if !ok {
		missing(id)
	}
	return cell
}

func (e *rich) owner(id any) rec.Map {
	owner, ok := e.owners[key(id)]
	if !ok {
		missing(id)
	}
	return owner
}

func (e *rich) role(id any) string {
	if role, ok := e.roles[key(id)]; ok {
		return role
	}
	return "body"
}

func (e *rich) declare(record rec.Map) {
	value := rec.Fetch(record, "cell")
	cell, isMap := value.(rec.Map)
	if !rec.SameKeys(record, []string{"cell", "code", "kind", "seq", "source"}) || !isMap || !rec.SameKeys(cell, cellFields) {
		rec.Fail("Malformed rich cell declaration")
	}
	if _, ok := e.cells[key(rec.Fetch(cell, "id"))]; ok {
		rec.Fail("Duplicate rich cell declaration")
	}
	cell = wholeRowspan(cell)
	// \multirow{-n} spans the n rows ending at its own; the rows above keep their own cells,
	// so the content stays in its row.
	if rowspan, ok := cell["rowspan"].(int64); ok && rowspan < 0 {
		e.upward = append(e.upward, rec.Fetch(cell, "table"))
		cell = copyMap(cell)
		cell["rowspan"] = int64(1)
	}
	for _, name := range []string{"id", "table", "row", "column", "colspan", "rowspan"} {
		if !rec.Positive(cell[name]) {
			rec.Fail("Unsupported rich cell span or coordinates")
		}
	}
	if kind, _ := rec.Fetch(cell, "kind").(string); alignments[kind] == "" {
		rec.Fail("Unsupported native column type")
	}
	e.cells[key(cell["id"])] = cell
}

// wholeRowspan: \multirow takes a fractional row count to fine-tune the height its content is
// centred in (bigdelim's \rdelim\}{2.2}{3mm} sets a brace over 2.2 rows; tamarin lecture 1); the
// cell covers the nearest whole number of rows, at least one, in the count's direction.
func wholeRowspan(cell rec.Map) rec.Map {
	fraction, ok := cell["rowspan"].(canonjson.Float)
	if !ok || math.IsNaN(float64(fraction)) || math.IsInf(float64(fraction), 0) {
		return cell
	}
	rows := int64(math.Max(1, math.Round(math.Abs(float64(fraction)))))
	if fraction < 0 {
		rows = -rows
	}
	cell = copyMap(cell)
	cell["rowspan"] = rows
	return cell
}

// origin is the span's table origin ({} when it has none).
func (e *rich) origin(span rec.Map) rec.Map {
	if origin, ok := e.origins[rec.Identity(span)]; ok {
		return origin
	}
	return rec.Map{}
}

func (e *rich) spans() []rec.Map {
	result := []rec.Map{}
	for _, block := range e.blocks {
		result = append(result, maps(tablemodel.FetchList(block, "content"))...)
	}
	return result
}

func tableID(table any) string { return "rich-t" + tablemodel.Inspect(table) }

func (e *rich) depth(table any) int {
	nesting, ok := e.nesting[key(table)]
	if !ok || !truthy(nesting["parent"]) {
		return 0
	}
	return 1 + e.depth(nesting["parent"])
}

// projectNested projects one table: false when it reads as text.
func (e *rich) projectNested(table any, rows []rec.Map, captured bool) (projected bool) {
	defer e.adoptNested(table)
	message, failed := rescue(func() { projected = e.projectNestedTable(table, rows, captured) })
	if failed {
		if !e.plainLabel(table) {
			e.diagnostics = append(e.diagnostics, rec.Map{"code": "unsupported_table_layout", "source": RichSource, "reason": message,
				"table": table})
		}
		return false
	}
	return projected
}

// plainLabel is a table that is one cell and paints no rule: a label set in a tabular (an Inkscape
// pdf_tex figure puts every label in `\smash{\begin{tabular}[t]{l}..`). Reading it as text loses no
// grid, so it is not reported as an unmodeled table.
func (e *rich) plainLabel(table any) bool {
	cells, rules := 0, int64(0)
	for _, row := range e.events {
		if !rec.Equal(row["table"], table) && row["code"] != "rich_table_cell" {
			continue
		}
		switch row["code"] {
		case "rich_table_cell":
			if rec.Equal(rec.Fetch(rec.AsMap(row["cell"]), "table"), table) {
				cells++
			}
		case "rich_table_presence":
			counts := rec.AsMap(rec.Fetch(row, "counts"))
			rules += rec.AsInt(rec.Fetch(counts, "rules")) + rec.AsInt(rec.Fetch(counts, "row_rules"))
		}
	}
	return cells == 1 && rules == 0
}

func (e *rich) projectNestedTable(table any, rows []rec.Map, captured bool) bool {
	for _, row := range e.diagnostics {
		if row["code"] == "table_inside_formula" && rec.Equal(row["table"], table) {
			e.adoptNested(table)
			return false
		}
	}
	present := false
	for _, row := range e.events {
		present = present || (row["code"] == "rich_table_presence" && rec.Equal(row["table"], table))
	}
	if !captured || !present {
		rec.Fail("Uncaptured native rich table rows")
	}
	e.projectTable(table, rows)
	return true
}

// adoptNested: a nested table's grid, or its text when it reads as text, is its parent
// cell's content.
func (e *rich) adoptNested(table any) {
	nesting, ok := e.nesting[key(table)]
	if !ok {
		return
	}
	spans := e.spans()
	parent := rec.Map{"table": rec.Fetch(nesting, "parent"), "cell": rec.Fetch(nesting, "cell")}
	var sibling rec.Map
	for _, span := range spans {
		if rec.Equal(slice(e.origin(span), "table", "cell"), parent) {
			sibling = span
			break
		}
	}
	parent["copy"] = int64(0)
	if sibling != nil {
		parent["copy"] = rec.Fetch(e.origin(sibling), "copy")
	}
	for _, span := range spans {
		if rec.Equal(e.origin(span)["table"], table) || span["id"] == tableID(table) {
			e.origins[rec.Identity(span)] = parent
		}
	}
}

// selectedSpans are the reading spans of a table, less those of a cell no native row holds: a
// repeated head or foot cell a longtable measured but never printed (the table fits its first
// page) is returned apart as unprinted, so the grid replaces its text (its formulas are dropped
// with it, see droppedMath); any other cell outside the
// rows (the caption a class typesets in a table cell, as deluxetable does) stays in the flow as
// prose.
func (e *rich) selectedSpans(table any, rows []rec.Map) (selected, unprinted []rec.Map) {
	captured := map[string]bool{}
	for _, row := range rows {
		for _, entry := range maps(tablemodel.FetchList(row, "cells")) {
			captured[key(rec.Fetch(entry, "cell"))] = true
		}
	}
	for _, span := range e.spans() {
		origin := e.origin(span)
		if !rec.Equal(origin["table"], table) {
			continue
		}
		id, inCell := origin["cell"]
		switch {
		case !inCell || captured[key(id)]:
			selected = append(selected, span)
		case slices.Contains([]string{"repeat_head", "repeat_foot"}, e.role(id)):
			unprinted = append(unprinted, span)
		}
	}
	return selected, unprinted
}

func (e *rich) projectTable(table any, rows []rec.Map) {
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, func(a, b rec.Map) int { return compareLists(rowOrder(a), rowOrder(b)) })
	e.verifyNativeCoverage(table, rows)
	selected, unprinted := e.selectedSpans(table, rows)
	if len(selected) == 0 && e.pageDecoration(rows) {
		return
	}
	if len(selected) == 0 {
		rec.Fail("Rich table has no retained reading body")
	}
	content, order := e.cellContent(selected)
	// A formula in a cell copy the grid does not keep would lose its reading.
	kept := []any{}
	for _, id := range order {
		for _, span := range content[id] {
			if span["type"] == "math" && truthy(span["occurrence"]) {
				kept = append(kept, span["occurrence"])
			}
		}
	}
	for _, span := range selected {
		if span["type"] == "math" && !contains(kept, span["occurrence"]) {
			rec.Fail("Rich table drops a formula of a repeated cell")
		}
	}
	grid := e.buildGrid(table, rows, content, order)
	// The layout every output draws is computed here, so a table it cannot draw falls back alone.
	checkLayout(tablemodel.New(grid))
	// Text of the table outside its cells (notes set in its scope) stays in the flow as prose.
	inCells := slices.Clone(unprinted)
	for _, span := range selected {
		if _, ok := e.origin(span)["cell"]; ok || decorative(span) {
			inCells = append(inCells, span)
		}
	}
	// Every check raises before the reading changes, so a table that fails leaves it intact.
	e.removeProvenDiagnostics(table)
	span := rec.Map{"type": "table_grid", "id": tableID(table), "grid": grid}
	e.alive = append(e.alive, span)
	e.replaceSpans(inCells, span)
	for _, gone := range unprinted {
		if gone["type"] == "math" && truthy(gone["occurrence"]) {
			e.droppedMath = append(e.droppedMath, gone["occurrence"])
		}
	}
	if containsEql(e.clamped, table) {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "clamped_multirow", "table": table})
	}
}

// rowOrder is a native row's sort key: [page, *origin_sp.reverse].
func rowOrder(row rec.Map) []any {
	origin := slices.Clone(tablemodel.FetchList(row, "origin_sp"))
	slices.Reverse(origin)
	return append([]any{rec.Fetch(row, "page")}, origin...)
}

// cellContent is each cell's canonical content (its first copy, trimmed), by cell id in
// first-appearance order.
func (e *rich) cellContent(spans []rec.Map) (map[string][]rec.Map, []string) {
	byCell := map[string][]rec.Map{}
	order := []string{}
	for _, span := range spans {
		origin := e.origin(span)
		if _, ok := origin["cell"]; !ok {
			continue
		}
		id := key(rec.Fetch(origin, "cell"))
		if _, seen := byCell[id]; !seen {
			order = append(order, id)
		}
		byCell[id] = append(byCell[id], span)
	}
	result := map[string][]rec.Map{}
	for _, id := range order {
		copies := map[string][]rec.Map{}
		copyOrder := []string{}
		for _, span := range byCell[id] {
			name := key(rec.Fetch(e.origin(span), "copy"))
			if _, seen := copies[name]; !seen {
				copyOrder = append(copyOrder, name)
			}
			copies[name] = append(copies[name], span)
		}
		canonical := copies[copyOrder[0]]
		for _, name := range copyOrder {
			if !rec.Equal(anyList(copies[name]), anyList(canonical)) {
				rec.Fail("Repeated table cell changed retained content")
			}
		}
		result[id] = trimCellContent(canonical)
	}
	return result, order
}

func trimCellContent(content []rec.Map) []rec.Map {
	result := make([]rec.Map, len(content))
	for index, span := range content {
		result[index] = copyMap(span)
	}
	blank := func(span rec.Map) bool {
		return span["type"] == "text" && rec.Strip(rec.FetchString(span, "text")) == ""
	}
	for len(result) > 0 && blank(result[0]) {
		result = result[1:]
	}
	for len(result) > 0 && blank(result[len(result)-1]) {
		result = result[:len(result)-1]
	}
	if len(result) > 0 && result[0]["type"] == "text" {
		result[0]["text"] = rec.LStrip(rec.FetchString(result[0], "text"))
	}
	if len(result) > 0 && result[len(result)-1]["type"] == "text" {
		result[len(result)-1]["text"] = rec.RStrip(rec.FetchString(result[len(result)-1], "text"))
	}
	return result
}

// pageDecoration: a table set in a page header or footer (a running tabular) is kept as a
// page decoration, not read: none of its text's owners occurs in the reading. Shipped glyphs
// that are neither read nor decoration are reported unclassified, so nothing is lost here.
func (e *rich) pageDecoration(rows []rec.Map) bool {
	owners := []any{}
	for _, row := range rows {
		for _, cell := range maps(tablemodel.FetchList(row, "cells")) {
			runs, _ := cell["runs"].([]any)
			for _, run := range maps(runs) {
				if owner := rec.Fetch(run, "owner"); !containsEql(owners, owner) {
					owners = append(owners, owner)
				}
			}
		}
	}
	if e.readOwners == nil {
		e.readOwners = []any{}
		for _, row := range e.records {
			if row["kind"] == "flow_occurrence" && truthy(row["owner"]) && !containsEql(e.readOwners, row["owner"]) {
				e.readOwners = append(e.readOwners, row["owner"])
			}
		}
	}
	for _, owner := range owners {
		if containsEql(e.readOwners, owner) {
			return false
		}
	}
	return len(owners) > 0
}

func decorative(span rec.Map) bool {
	return span["type"] == "text" && rec.Strip(strings.ReplaceAll(rec.FetchString(span, "text"), "[rule]", "")) == ""
}

func (e *rich) verifyNativeCoverage(table any, rows []rec.Map) {
	presences := []rec.Map{}
	for _, row := range e.events {
		if row["code"] == "rich_table_presence" && rec.Equal(row["table"], table) {
			presences = append(presences, row)
		}
	}
	byPage := map[string][]rec.Map{}
	pages := []any{}
	for _, row := range rows {
		page := rec.Fetch(row, "page")
		if _, ok := byPage[key(page)]; !ok {
			pages = append(pages, page)
		}
		byPage[key(page)] = append(byPage[key(page)], row)
	}
	presencePages := []any{}
	for _, presence := range presences {
		presencePages = append(presencePages, rec.Fetch(presence, "page"))
	}
	if !rec.Equal(tablemodel.SortValues(presencePages), tablemodel.SortValues(pages)) {
		rec.Fail("Rich table physical pages differ")
	}
	for _, presence := range presences {
		page := rec.Fetch(presence, "page")
		pageRows, ok := byPage[key(page)]
		if !ok {
			missing(page)
		}
		cells := []rec.Map{}
		fills := int64(0)
		for _, row := range pageRows {
			cells = append(cells, maps(tablemodel.FetchList(row, "cells"))...)
			fills += rec.AsInt(fetchDefault(row, "blank_fills", int64(0)))
		}
		e.verifyPresence(table, presence, cells, fills)
	}
}

// verifyPresence: fills are the colour fills of empty cells (colortbl's \rowcolor across a
// \multirow), row paint that no cell entry holds.
func (e *rich) verifyPresence(table any, presence rec.Map, cells []rec.Map, fills int64) {
	page := rec.Fetch(presence, "page")
	expected := map[string]any{}
	for _, kind := range []string{"glyphs", "rules"} {
		counts := []any{}
		for _, cell := range cells {
			counts = append(counts, rec.Fetch(cell, kind))
		}
		expected[kind] = tablemodel.Sum(counts)
	}
	expected["rules"] = add(expected["rules"], fills)
	counts := fetchMap(presence, "counts")
	// Every row glyph is in a cell; glyphs outside the rows are read as prose.
	if !(number(rec.Fetch(counts, "glyphs")) >= number(expected["glyphs"])) || !rec.Equal(rec.Fetch(counts, "row_glyphs"), expected["glyphs"]) ||
		!rec.Equal(rec.Fetch(counts, "row_rules"), expected["rules"]) || !(number(rec.Fetch(counts, "rules")) >= number(expected["rules"])) {
		rec.Fail("Rich table native nodes are not fully attributed to cells and decorations")
	}
	outside := int64(0)
	ink := []any{}
	for _, row := range e.events {
		if row["code"] == "rich_table_rule" && rec.Equal(values(row, "table", "page", "in_row"), []any{table, page, false}) {
			outside++
		}
		if row["code"] == "rich_table_glyphs" && rec.Equal(values(row, "table", "page"), []any{table, page}) {
			ink = append(ink, int64(size(rec.Fetch(row, "glyphs"))))
		}
	}
	if !rec.Equal(sub(rec.Fetch(counts, "rules"), expected["rules"]), outside) {
		rec.Fail("Uncovered rich table decorations")
	}
	if !rec.Equal(tablemodel.Sum(ink), rec.Fetch(counts, "glyphs")) {
		rec.Fail("Rich table glyph paint coverage differs")
	}
	for _, cell := range cells {
		e.verifyCellSlots(cell, table)
	}
}

func (e *rich) verifyCellSlots(cell rec.Map, table any) {
	source := e.cell(rec.Fetch(cell, "cell"))
	if !rec.Equal(rec.Fetch(source, "table"), table) {
		rec.Fail("Cell belongs to another table")
	}
	if !isZero(rec.Fetch(cell, "unowned_glyphs")) {
		rec.Fail("Unbound native rich cell glyphs")
	}
	seen := map[string]bool{}
	for _, run := range maps(tablemodel.FetchList(cell, "runs")) {
		items := tablemodel.FetchList(e.owner(rec.Fetch(run, "owner")), "items")
		first, last := rec.Fetch(run, "first"), rec.Fetch(run, "last")
		if !positive(first) || number(first) > number(last) || number(last) > float64(len(items)) {
			rec.Fail("Invalid native cell flow range")
		}
		for index := rec.AsInt(first); index <= rec.AsInt(last); index++ {
			item := rec.AsMap(fetchIndex(items, index-1))
			native := rec.Equal(values(item, "table", "table_cell"), []any{table, rec.Fetch(source, "id")})
			name := key(run["owner"], index)
			unique := !seen[name]
			seen[name] = true
			if !native || !unique {
				rec.Fail("Repeated or foreign rich cell slot")
			}
		}
	}
	missingRules := rec.Fetch(cell, "unowned_rules")
	layoutGlyphs := fetchDefault(cell, "layout_glyphs", int64(0))
	rules := rec.Fetch(cell, "rules")
	coverage := sub(sub(add(rec.Fetch(cell, "glyphs"), rules), missingRules), layoutGlyphs)
	// A picture's ink is covered by the picture's one slot.
	coverage = add(sub(coverage, fetchDefault(cell, "graphic_ink", int64(0))), fetchDefault(cell, "graphics", int64(0)))
	if number(missingRules) >= 0 && number(missingRules) <= number(rules) && rec.Equal(int64(len(seen)), coverage) {
		return
	}
	rec.Fail("Rich cell scalar/native node coverage differs")
}

// replaceSpans puts the table where its first selected span was and removes the others;
// a block left empty goes, and what pointed to it points to the table's block.
func (e *rich) replaceSpans(selected []rec.Map, table rec.Map) {
	ids := map[uintptr]bool{}
	for _, span := range selected {
		ids[rec.Identity(span)] = true
	}
	var first rec.Map
	for _, block := range e.blocks {
		if slices.ContainsFunc(maps(tablemodel.FetchList(block, "content")), func(span rec.Map) bool { return ids[rec.Identity(span)] }) {
			first = block
			break
		}
	}
	inserted := false
	for _, block := range e.blocks {
		content := []any{}
		for _, span := range maps(tablemodel.FetchList(block, "content")) {
			switch {
			case !ids[rec.Identity(span)]:
				content = append(content, span)
			case !inserted:
				inserted = true
				content = append(content, table)
			}
		}
		block["content"] = content
	}
	removed := []any{}
	kept := []rec.Map{}
	for _, block := range e.blocks {
		if len(tablemodel.FetchList(block, "content")) == 0 {
			removed = append(removed, rec.Fetch(block, "id"))
		}
	}
	for _, block := range e.blocks {
		if !contains(removed, rec.Fetch(block, "id")) {
			kept = append(kept, block)
		}
	}
	e.blocks = kept
	for _, target := range e.targets {
		if contains(removed, target["block"]) {
			target["block"] = firstID(first)
		}
	}
	for _, block := range e.blocks {
		for _, span := range flat(maps(tablemodel.FetchList(block, "content"))) {
			if contains(removed, span["target_block"]) {
				span["target_block"] = firstID(first)
			}
		}
	}
}

// firstID is first.fetch('id'); Ruby fails with NoMethodError (not rescued) without one.
func firstID(block rec.Map) any {
	if block == nil {
		panic("tables: undefined method 'fetch' for nil")
	}
	return rec.Fetch(block, "id")
}

func (e *rich) removeProvenDiagnostics(table any) {
	names := []string{"page", "width", "height", "depth", "box_height", "box_depth", "box_kind"}
	observed := []rec.Map{}
	observedValues := []any{}
	for _, row := range e.diagnostics {
		if row["code"] == "unclassified_running_rule" && rec.Equal(row["table"], table) {
			observed = append(observed, row)
			observedValues = append(observedValues, values(row, names...))
		}
	}
	// Leaders are counted with the table's paint; only rule nodes are audited as running rules.
	captured := []any{}
	for _, row := range e.events {
		if row["code"] == "rich_table_rule" && rec.Equal(row["table"], table) &&
			(rec.Equal(row["height"], int64(runningSize)) || rec.Equal(row["depth"], int64(runningSize))) && !truthy(row["native_leader"]) {
			captured = append(captured, values(row, names...))
		}
	}
	if !rec.Equal(tablemodel.SortValues(observedValues), tablemodel.SortValues(captured)) {
		rec.Fail("Uncovered running rich table rules")
	}
	unsupported := []rec.Map{}
	unsupportedSlots := []any{}
	for _, row := range e.diagnostics {
		if row["code"] == "unsupported_reading_node" && rec.Equal(row["table"], table) {
			unsupported = append(unsupported, row)
			unsupportedSlots = append(unsupportedSlots, values(row, "owner", "slot"))
		}
	}
	if !rec.Equal(tablemodel.SortValues(e.scalarRuleSlots(table)), tablemodel.SortValues(unsupportedSlots)) {
		rec.Fail("Uncovered scalar rich table rules")
	}
	kept := []rec.Map{}
	for _, row := range e.diagnostics {
		if !containsMap(observed, row) && !containsMap(unsupported, row) {
			kept = append(kept, row)
		}
	}
	e.diagnostics = kept
}

// scalarRuleSlots is the [owner, slot] of each captured rule the flow retained as an
// unsupported item.
func (e *rich) scalarRuleSlots(table any) []any {
	slots := []any{}
	for _, row := range e.events {
		_, hasOwner := row["owner"]
		_, hasSlot := row["slot"]
		if row["code"] != "rich_table_rule" || !rec.Equal(row["table"], table) || !hasOwner || !hasSlot {
			continue
		}
		item := rec.AsMap(fetchIndex(tablemodel.FetchList(e.owner(rec.Fetch(row, "owner")), "items"), number64(rec.Fetch(row, "slot"))-1))
		if item["type"] == "unsupported" {
			slots = append(slots, values(row, "owner", "slot"))
		}
	}
	return slots
}

func number64(value any) int64 {
	integer, ok := value.(int64)
	if !ok {
		rec.Fail("no implicit conversion of %s into Integer", tablemodel.Inspect(value))
	}
	return integer
}

// flat is PaperInline.flat: the spans of a reading, the content of each table's cells in
// row and column order in place of the table.
func flat(content []rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, span := range content {
		if span["type"] != "table_grid" {
			result = append(result, span)
			continue
		}
		model := tablemodel.New(fetchMap(span, "grid"))
		for _, cells := range model.Rows() {
			for _, cell := range cells {
				result = append(result, flat(maps(tablemodel.FetchList(cell, "content")))...)
			}
		}
	}
	return result
}
