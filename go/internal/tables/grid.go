package tables

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
	"github.com/sarielhp/relatex/internal/tablemodel"
	"github.com/sarielhp/relatex/internal/textstyle"
)

var textFields = []string{"space", "text", "type"}

// borderItems are the flow items of a horizontal border slot (TableGrid::BORDER_ITEMS).
var borderItems = []any{
	rec.Map{"type": "unsupported", "text": "[rule]", "space": " "},
	rec.Map{"type": "unsupported", "text": "[rule]", "space": ""},
}

// slotKey is a retained flow slot: [owner, slot].
type slotKey struct {
	owner any
	slot  int64
}

func (s slotKey) name() string { return key(s.owner, s.slot) }

// tableGrid is TableGrid: a bounded classic table validated against the independently
// captured flow slots.
type tableGrid struct {
	table     rec.Map
	styles    textstyle.Catalog
	fallback  map[any]rec.Map
	owners    map[string]rec.Map
	seen      map[string]bool
	seenOrder []slotKey
	rowSlots  [][]slotKey
	columns   int
	widths    []any
	separate  *bool
	lastSlot  int64
	owner     any
	slotCount int64
}

func newGrid(table rec.Map, records []rec.Map) *tableGrid {
	schema.Validate(table, tablemodel.Contract("table-grid-evidence.schema.json"), "$")
	g := &tableGrid{table: table, styles: textstyle.NewCatalog(records), owners: map[string]rec.Map{}, seen: map[string]bool{}}
	g.adoptUnobservedFonts(records)
	candidates := 0
	for _, row := range records {
		if row["kind"] == "flow_candidate" {
			g.owners[key(rec.Fetch(row, "owner"))] = row
			candidates++
		}
	}
	if len(g.owners) != candidates {
		rec.Fail("Duplicate table flow owner")
	}
	return g
}

func (g *tableGrid) ownerRow(owner any) rec.Map {
	row, ok := g.owners[key(owner)]
	if !ok {
		missing(owner)
	}
	return row
}

func (g *tableGrid) flatContent() []rec.Map {
	items := []any{}
	for _, value := range tablemodel.FetchList(g.ownerRow(g.owner), "items") {
		items = append(items, g.styledItem(rec.AsMap(value), g.owner))
	}
	return textstyle.TextRuns(items, g.styles, false)
}

// adoptUnobservedFonts: a direct TeX font (\font\x=cmti10) has no NFSS declaration; its style,
// read from its name for the owners that print it, joins the grid catalog under its TeX font id.
func (g *tableGrid) adoptUnobservedFonts(records []rec.Map) {
	g.fallback = textstyle.UnobservedOwnerStyles(records, g.styles)
	for _, style := range g.fallback {
		g.styles[rec.FetchInt(style, "font_id")] = style
	}
}

// styledItem is an item with its owner's direct TeX font where the item names no style.
func (g *tableGrid) styledItem(item rec.Map, owner any) rec.Map {
	style, ok := g.fallback[owner]
	if _, styled := item["style"]; styled || !ok {
		return item
	}
	result := except(item)
	result["style"] = rec.Fetch(style, "font_id")
	return result
}

func (g *tableGrid) decode() rec.Map {
	preamble := rec.FetchString(g.table, "preamble")
	if !tablemodel.ClassicPreamble.MatchString(preamble) {
		rec.Fail("Unsupported table preamble")
	}
	g.columns = strings.Count(preamble, "l") + strings.Count(preamble, "c") + strings.Count(preamble, "r")
	tableRows := tablemodel.FetchList(g.table, "rows")
	if g.columns < 1 || g.columns > 16 || len(tableRows) < 1 || len(tableRows) > 64 {
		rec.Fail("Unsupported table dimensions")
	}
	rows := make([]any, len(tableRows))
	for index, row := range tableRows {
		rows[index] = g.decodeRow(rec.AsMap(row))
	}
	g.validateCoverage()
	return rec.Map{"preamble": preamble, "width_sp": rec.Fetch(g.table, "width"), "rows": rows,
		"horizontal_borders": rec.Fetch(g.table, "horizontal_borders")}
}

func (g *tableGrid) decodeRow(row rec.Map) []any {
	cells := maps(tablemodel.FetchList(row, "cells"))
	widths := make([]any, len(cells))
	for index, cell := range cells {
		widths[index] = rec.Fetch(cell, "width")
	}
	if len(cells) != g.columns || !rec.Equal(tablemodel.Sum(widths), rec.Fetch(row, "width")) || !rec.Equal(rec.Fetch(row, "width"), rec.Fetch(g.table, "width")) {
		rec.Fail("Ambiguous native row geometry")
	}
	if g.widths == nil {
		g.widths = widths
	}
	if !rec.Equal(widths, g.widths) {
		rec.Fail("Inconsistent column geometry")
	}
	start := len(g.seenOrder)
	result := make([]any, len(cells))
	for index, cell := range cells {
		g.validateCell(cell, row, index)
		items := g.decodeItems(cell)
		var text strings.Builder
		styled := false
		for _, item := range items {
			text.WriteString(rec.FetchString(item, "space") + rec.FetchString(item, "text"))
			_, has := item["style"]
			styled = styled || has
		}
		decoded := rec.Map{"text": rec.Strip(text.String()), "width_sp": rec.Fetch(cell, "width"),
			"height_sp": rec.Fetch(cell, "height"), "depth_sp": rec.Fetch(cell, "depth"),
			"font_size_sp": rec.Fetch(cell, "font_size"), "borders": rec.Fetch(cell, "borders")}
		runs := textstyle.TextRuns(anyList(items), g.styles, true)
		if styled && len(runs) > 0 {
			stripped := make([]any, len(runs))
			for position, run := range runs {
				stripped[position] = except(run, "type")
			}
			decoded["runs"] = stripped
		}
		result[index] = decoded
	}
	g.rowSlots = append(g.rowSlots, slices.Clone(g.seenOrder[start:]))
	return result
}

func (g *tableGrid) validateCell(cell, row rec.Map, index int) {
	if !rec.Equal(values(cell, "height", "depth"), values(row, "height", "depth")) || !positive(rec.Fetch(cell, "width")) {
		rec.Fail("Invalid cell geometry")
	}
	borders := maps(tablemodel.FetchList(cell, "borders"))
	if len(borders) != map[bool]int{true: 2, false: 1}[index == 0] {
		rec.Fail("Unexpected cell borders")
	}
	if g.separate == nil {
		separate := isZero(rec.Fetch(borders[0], "x"))
		g.separate = &separate
	}
	for borderIndex, border := range borders {
		width := rec.Fetch(border, "width")
		x := tablemodel.BorderX(rec.Fetch(cell, "width"), index, borderIndex, width, *g.separate)
		if !positive(width) || !tablemodel.Even(width) || !rec.Equal(rec.Fetch(border, "x"), x) ||
			!rec.Equal(values(border, "height", "depth"), values(row, "height", "depth")) {
			rec.Fail("Unmatched native border geometry")
		}
	}
}

func (g *tableGrid) decodeItems(cell rec.Map) []rec.Map {
	items := []rec.Map{}
	for _, value := range tablemodel.FetchList(cell, "runs") {
		run := rec.AsMap(value)
		ownerID := rec.Fetch(run, "owner")
		owner := g.ownerRow(ownerID)
		ownerItems := tablemodel.FetchList(owner, "items")
		first, firstInt := run["first"].(int64)
		last, lastInt := run["last"].(int64)
		if !firstInt || !lastInt || first < 1 || first > last || last > int64(len(ownerItems)) {
			rec.Fail("Invalid cell range")
		}
		for slot := first; slot <= last; slot++ {
			name := slotKey{ownerID, slot}
			if g.seen[name.name()] || slot <= g.lastSlot {
				rec.Fail("Repeated or reordered table content")
			}
			g.lastSlot = slot
			item := rec.AsMap(ownerItems[slot-1])
			text, isText := item["text"].(string)
			if !rec.SameKeys(except(item, "style"), textFields) || item["type"] != "text" || !isText || text == "[unmapped]" ||
				(item["space"] != "" && item["space"] != " ") {
				rec.Fail("Unsupported cell content")
			}
			g.seen[name.name()] = true
			g.seenOrder = append(g.seenOrder, name)
			items = append(items, g.styledItem(item, ownerID))
		}
	}
	return items
}

func (g *tableGrid) validateCoverage() {
	owners := []any{}
	for _, slot := range g.seenOrder {
		if !containsEql(owners, slot.owner) {
			owners = append(owners, slot.owner)
		}
	}
	if len(owners) != 1 {
		rec.Fail("Table is not a single bounded owner")
	}
	g.owner = owners[0]
	g.slotCount = int64(len(g.seenOrder))
	type border struct {
		item  rec.Map
		index int64
	}
	borders := []border{}
	for index, item := range tablemodel.FetchList(g.ownerRow(g.owner), "items") {
		if !g.seen[slotKey{g.owner, int64(index) + 1}.name()] {
			borders = append(borders, border{rec.AsMap(item), int64(index)})
		}
	}
	horizontal := maps(tablemodel.FetchList(g.table, "horizontal_borders"))
	valid := len(borders) == len(horizontal)
	for _, entry := range borders {
		valid = valid && contains(borderItems, slice(entry.item, textFields...))
	}
	if !valid {
		rec.Fail("Uncovered table flow slots")
	}
	for position, entry := range borders {
		g.validateBorder(horizontal[position], entry.index)
	}
}

func (g *tableGrid) validateBorder(border rec.Map, index int64) {
	after, ok := rec.Fetch(border, "after_row").(int64)
	if !ok || after < 0 || after > int64(len(g.rowSlots)) {
		rec.Fail("Invalid border row boundary")
	}
	valid := rec.Equal(rec.Fetch(border, "width"), rec.Fetch(g.table, "width")) && positive(rec.Fetch(border, "height")) &&
		isZero(rec.Fetch(border, "depth"))
	for row, slots := range g.rowSlots {
		for _, slot := range slots {
			if int64(row) < after {
				valid = valid && slot.slot <= index
			} else {
				valid = valid && slot.slot > index+1
			}
		}
	}
	if !valid {
		rec.Fail("Border lost row boundary ownership")
	}
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
