package tables

import (
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// layout holds what RichTableLayout's checks need: the reading evidence builds the layout
// every output draws (its browser grid and display rows) only so that a table it cannot draw
// falls back alone. The segments, row heights and rule identities it computes are not part of
// the reading, so only the checks that can fail are ported.
type layout struct {
	model        *tablemodel.Rich
	cells        map[string]rec.Map
	columnWidths []any
}

// checkLayout is RichTableLayout.new(model) followed by #browser_grid and #display_rows.
func checkLayout(model *tablemodel.Rich) {
	l := &layout{model: model, cells: map[string]rec.Map{}}
	grid := model.Data
	for _, value := range tablemodel.FetchList(grid, "cells") {
		cell := rec.AsMap(value)
		l.cells[rec.FetchString(cell, "id")] = cell
	}
	if widths, ok := grid["column_widths_sp"]; ok {
		l.columnWidths = rec.AsList(widths)
	} else {
		count := fetchInt(grid, "column_count")
		width := number(fetchDefault(grid, "width_sp", 3_276_800*count)) / float64(count)
		for range count {
			l.columnWidths = append(l.columnWidths, width)
		}
	}
	fragments := maps(rec.AsList(fetchDefault(grid, "native_fragments", []any{})))
	for _, fragment := range fragments {
		for _, row := range maps(tablemodel.FetchList(fragment, "rows")) {
			rec.Fetch(row, "row")
			add(rec.Fetch(row, "height"), rec.Fetch(row, "depth"))
		}
	}
	for _, fragment := range fragments {
		l.nativeRules(fragment)
	}
	// browser_grid: a nested rich grid is read back by its own layout unless it is classic.
	for _, cells := range model.Rows() {
		for _, cell := range cells {
			for _, span := range maps(tablemodel.FetchList(cell, "content")) {
				nested, isGrid := span["grid"].(rec.Map)
				if span["type"] == "table_grid" && isGrid && nested["schema_version"] == tablemodel.Version && !classicReadable(nested) {
					checkLayout(tablemodel.New(nested))
				}
			}
		}
	}
}

func (l *layout) nativeRules(fragment rec.Map) {
	for _, rule := range maps(tablemodel.FetchList(fragment, "rules")) {
		if truthy(fetchDefault(rule, "math", false)) || tablemodel.AreaFill(rule) {
			continue
		}
		rect := tablemodel.FetchList(rule, "rect_sp")
		x, y, width, height := rect[0], rect[1], rect[2], rect[3]
		if number(width) >= number(height) {
			l.horizontal(fragment, x, y, width, height)
		} else {
			l.vertical(fragment, x, y, width, height)
		}
	}
}

// rowLeft is the left edge of a native row's first grid column: its origin plus its leftmost
// cell, unless column 0 holds no entry (an empty cell under a \multirow prints no tagged node).
// The row is then measured from its first cell in the grid, less that cell's column offset.
func (l *layout) rowLeft(native rec.Map) float64 {
	lefts := []any{}
	column, start := int64(-1), 0.0
	for _, entry := range maps(tablemodel.FetchList(native, "cells")) {
		lefts = append(lefts, rec.Fetch(entry, "left"))
		if cell := tablemodel.SourceCell(l.cells, entry["cell"]); cell != nil && (column < 0 || fetchInt(cell, "column") < column) {
			x0, _ := l.cellExtent(cell)
			column, start = fetchInt(cell, "column"), number(rec.Fetch(entry, "left"))-x0
		}
	}
	left := number(minimum(lefts))
	if column >= 0 {
		left = start
	}
	return number(rec.Index(tablemodel.FetchList(native, "origin_sp"), 0)) + left
}

func (l *layout) horizontal(fragment rec.Map, ruleX, ruleY, width, height any) {
	center := number(ruleY) + float64(number(height)/2.0)
	var native rec.Map
	var side string
	best := 0.0
	for _, row := range maps(tablemodel.FetchList(fragment, "rows")) {
		origin := tablemodel.FetchList(row, "origin_sp")
		top := number(origin[len(origin)-1])
		bottom := number(add(add(origin[len(origin)-1], rec.Fetch(row, "height")), rec.Fetch(row, "depth")))
		for _, choice := range []struct {
			side string
			edge float64
		}{{"top", top}, {"bottom", bottom}} {
			if distance := abs(choice.edge - center); native == nil || distance < best {
				native, side, best = row, choice.side, distance
			}
		}
	}
	left := l.rowLeft(native)
	from := number(ruleX) - left
	to := number(ruleX) + number(width) - left
	covered := 0.0
	for _, cell := range l.ownersForNativeEdge(fetchInt(native, "row"), side) {
		x0, x1 := l.cellExtent(cell)
		first, last := max(from, x0), min(to, x1)
		if first < last {
			covered += last - first
		}
	}
	expected := min(to, number(tablemodel.Sum(l.columnWidths))) - max(from, 0)
	if abs(covered-max(expected, 0)) > 2 {
		rec.Fail("Native horizontal rule crosses a spanned cell interior")
	}
}

// cellExtent is the horizontal extent of RichTableLayout#cell_rect.
func (l *layout) cellExtent(cell rec.Map) (float64, float64) {
	column, colspan := fetchInt(cell, "column"), fetchInt(cell, "colspan")
	x := number(tablemodel.Sum(l.columnWidths[:min(column, int64(len(l.columnWidths)))]))
	end := min(column+colspan, int64(len(l.columnWidths)))
	return x, x + number(tablemodel.Sum(l.columnWidths[min(column, end):end]))
}

func (l *layout) ownersForNativeEdge(row int64, side string) []rec.Map {
	occupied := rec.AsList(fetchIndex(anyList(l.model.Occupancy), row))
	ids := []any{}
	for _, id := range occupied {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	result := []rec.Map{}
	for _, id := range ids {
		cell := l.cell(id)
		var atEdge bool
		if side == "top" {
			atEdge = fetchInt(cell, "row") == row
		} else {
			atEdge = fetchInt(cell, "row")+fetchInt(cell, "rowspan") == row+1
		}
		if atEdge {
			result = append(result, cell)
		}
	}
	return result
}

func (l *layout) cell(id any) rec.Map {
	name, _ := id.(string)
	cell, ok := l.cells[name]
	if !ok {
		missing(id)
	}
	return cell
}

func (l *layout) vertical(fragment rec.Map, ruleX, ruleY, width, height any) {
	positions := []float64{0}
	running := any(int64(0))
	for _, value := range l.columnWidths {
		running = add(running, value)
		positions = append(positions, number(running))
	}
	for _, native := range maps(tablemodel.FetchList(fragment, "rows")) {
		origin := tablemodel.FetchList(native, "origin_sp")
		rowTop := origin[len(origin)-1]
		rowBottom := add(add(rowTop, rec.Fetch(native, "height")), rec.Fetch(native, "depth"))
		from := max(number(ruleY), number(rowTop))
		to := min(number(add(ruleY, height)), number(rowBottom))
		// \cline pulls the next row up by a rule width: a sliver of overlap is not a crossing.
		if !(to-from > min(65_536, number(half(height)))) {
			continue
		}
		left := l.rowLeft(native)
		boundary, best := 0, 0.0
		for index, position := range positions {
			distance := abs(position + left - number(ruleX) - float64(number(width)/2.0))
			if index == 0 || distance < best {
				boundary, best = index, distance
			}
		}
		column := max(boundary-1, 0)
		row := rec.AsList(fetchIndex(anyList(l.model.Occupancy), fetchInt(native, "row")))
		cell := l.cell(fetchIndex(row, int64(column)))
		expected := fetchInt(cell, "column")
		if boundary != 0 {
			expected += fetchInt(cell, "colspan")
		}
		if int64(boundary) != expected {
			rec.Fail("Native vertical rule cuts a spanned cell")
		}
	}
}

// classicReadable reports whether ClassicTableProjection.read returns a grid for a rich grid:
// a strict compatibility view for already supported classic native tables.
func classicReadable(grid rec.Map) (readable bool) {
	_, failed := rescue(func() { readable = classicProjection(grid) })
	return readable && !failed
}

func classicProjection(grid rec.Map) bool {
	fragments := rec.AsList(fetchDefault(grid, "native_fragments", []any{}))
	sections := rec.AsMap(fetchDefault(grid, "sections", rec.Map{}))
	_, body := sections["body"]
	preamble, _ := fetchDefault(grid, "native_preamble", "").(string)
	if len(fragments) != 1 || len(sections) != 1 || !body || !tablemodel.ClassicPreamble.MatchString(preamble) {
		return false
	}
	cellList := maps(tablemodel.FetchList(grid, "cells"))
	for _, cell := range cellList {
		if !classicCell(cell) {
			return false
		}
	}
	fragment := rec.AsMap(fragments[0])
	ordered := maps(tablemodel.FetchList(fragment, "rows"))
	slices.SortStableFunc(ordered, func(a, b rec.Map) int { return compareNumbers(rec.Fetch(a, "row"), rec.Fetch(b, "row")) })
	for index, row := range ordered {
		if !rec.Equal(rec.Fetch(row, "row"), int64(index)) {
			return false
		}
	}
	if !rec.Equal(int64(len(ordered)), rec.Fetch(grid, "row_count")) {
		return false
	}
	cells := map[string]rec.Map{}
	for _, cell := range cellList {
		cells[key(rec.Fetch(cell, "source_cell"))] = cell
	}
	rows := make([]any, len(ordered))
	borderCount := 0
	for index, row := range ordered {
		native := nativeRow(row, cells)
		for _, cell := range native {
			borderCount += size(rec.Fetch(rec.AsMap(cell), "borders"))
		}
		rows[index] = native
	}
	rules := maps(tablemodel.FetchList(fragment, "rules"))
	horizontal := []any{}
	for _, rule := range rules {
		if !truthy(rec.Fetch(rule, "in_row")) {
			horizontal = append(horizontal, horizontalBorder(rule, ordered))
		}
	}
	if len(rules) != len(horizontal)+borderCount {
		rec.Fail("Classic table projection lost native rules")
	}
	tablemodel.ValidateShape(rec.Map{"preamble": rec.Fetch(grid, "native_preamble"), "width_sp": rec.Fetch(ordered[0], "width"),
		"rows": rows, "horizontal_borders": horizontal})
	return true
}

// size is #size of an array or of an object (lualibs writes an empty Lua table as {}).
func size(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case rec.Map:
		return len(typed)
	case string:
		return len([]rune(typed))
	}
	rec.Fail("undefined method 'size' for %s", tablemodel.Inspect(value))
	return 0
}

func classicCell(cell rec.Map) bool {
	_, source := cell["source_cell"]
	if !source || !rec.Equal(rec.Fetch(cell, "rowspan"), int64(1)) || !rec.Equal(rec.Fetch(cell, "colspan"), int64(1)) ||
		fetchDefault(cell, "role", "body") != "body" {
		return false
	}
	for _, span := range maps(tablemodel.FetchList(cell, "content")) {
		if span["type"] != "text" {
			return false
		}
		for name := range span {
			if name != "type" && name != "text" && name != "style" {
				return false
			}
		}
	}
	return true
}

func nativeRow(row rec.Map, cells map[string]rec.Map) []any {
	entries := maps(tablemodel.FetchList(row, "cells"))
	result := make([]any, len(entries))
	for index, entry := range entries {
		cell, ok := cells[key(rec.Fetch(entry, "cell"))]
		if !ok {
			missing(entry["cell"])
		}
		runs := []rec.Map{}
		for _, span := range maps(tablemodel.FetchList(cell, "content")) {
			runs = append(runs, except(span, "type"))
		}
		runs = trimRuns(runs)
		text := ""
		styled := false
		for _, run := range runs {
			text += rec.FetchString(run, "text")
			_, has := run["style"]
			styled = styled || has
		}
		native := rec.Map{"text": text, "width_sp": rec.Fetch(entry, "width"), "height_sp": rec.Fetch(entry, "height"),
			"depth_sp": rec.Fetch(entry, "depth"), "font_size_sp": rec.Fetch(entry, "font_size"), "borders": rec.Fetch(entry, "borders")}
		if styled {
			native["runs"] = anyList(runs)
		}
		result[index] = native
	}
	return result
}

// trimRuns is TextStyle.trim_runs!.
func trimRuns(runs []rec.Map) []rec.Map {
	for len(runs) > 0 && rec.Strip(rec.FetchString(runs[0], "text")) == "" {
		runs = runs[1:]
	}
	for len(runs) > 0 && rec.Strip(rec.FetchString(runs[len(runs)-1], "text")) == "" {
		runs = runs[:len(runs)-1]
	}
	if len(runs) == 0 {
		return runs
	}
	runs[0]["text"] = rec.LStrip(rec.FetchString(runs[0], "text"))
	runs[len(runs)-1]["text"] = rec.RStrip(rec.FetchString(runs[len(runs)-1], "text"))
	return runs
}

func horizontalBorder(rule rec.Map, rows []rec.Map) rec.Map {
	if truthy(fetchDefault(rule, "math", false)) {
		rec.Fail("Classic table projection encountered cell math")
	}
	if !isZero(rec.Fetch(rule, "depth")) || !positive(rec.Fetch(rule, "height")) {
		rec.Fail("Classic table projection encountered non-horizontal rule")
	}
	rect := tablemodel.FetchList(rule, "rect_sp")
	x, y, width, height := rect[0], rect[1], rect[2], rect[3]
	first := rows[0]
	if !rec.Equal(width, rec.Fetch(first, "width")) || abs(number(x)-number(rec.Index(tablemodel.FetchList(first, "origin_sp"), 0))) > 1 {
		rec.Fail("Classic table projection encountered a partial rule")
	}
	center := number(y) + float64(number(height)/2.0)
	after := int64(0)
	for _, row := range rows {
		origin := tablemodel.FetchList(row, "origin_sp")
		if number(add(add(origin[len(origin)-1], rec.Fetch(row, "height")), rec.Fetch(row, "depth"))) <= center {
			after++
		}
	}
	return rec.Map{"after_row": after, "width": rec.Fetch(rule, "width"), "height": rec.Fetch(rule, "height"), "depth": rec.Fetch(rule, "depth")}
}
