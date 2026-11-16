package tablelayout

import (
	"regexp"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

var classicPreamble = regexp.MustCompile(`\A\|[lcr](?:\|[lcr])*\|\z`)

// ClassicProjection is ClassicTableProjection.read: a strict compatibility view for
// already supported classic native tables; nil when the grid is no such table.
func ClassicProjection(grid rec.Map) (result rec.Map) {
	if grid["schema_version"] != tablemodel.Version {
		return grid
	}
	defer classicRescue(&result)
	fragments := rec.ListOf(grid["native_fragments"])
	sections, _ := rec.FetchOr(grid, "sections", rec.Map{}).(rec.Map)
	if countTruthy(fragments) != 1 || len(fragments) != 1 || len(sections) != 1 || sections["body"] == nil {
		return nil
	}
	preamble, _ := rec.FetchOr(grid, "native_preamble", "").(string)
	if !classicPreamble.MatchString(preamble) {
		return nil
	}
	for _, cell := range rec.FetchList(grid, "cells") {
		if !classicCell(rec.AsMap(cell)) {
			return nil
		}
	}
	fragment := rec.AsMap(fragments[0])
	ordered := []rec.Map{}
	for _, row := range rec.FetchList(fragment, "rows") {
		ordered = append(ordered, rec.AsMap(row))
	}
	sort.SliceStable(ordered, func(a, b int) bool { return rec.FetchInt(ordered[a], "row") < rec.FetchInt(ordered[b], "row") })
	for index, row := range ordered {
		if rec.FetchInt(row, "row") != int64(index) {
			return nil
		}
	}
	if int64(len(ordered)) != rec.FetchInt(grid, "row_count") {
		return nil
	}
	return classicGrid(grid, fragment, ordered)
}

func countTruthy(values []any) int {
	count := 0
	for _, value := range values {
		if truthy(value) {
			count++
		}
	}
	return count
}

// classicRescue: the canonical rich path remains responsible for non-classic contracts.
func classicRescue(result *rec.Map) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if _, ok := recovered.(rec.Failure); ok {
		*result = nil
		return
	}
	panic(recovered)
}

func classicCell(cell rec.Map) bool {
	if _, ok := cell["source_cell"]; !ok {
		return false
	}
	if !rec.Equal(rec.Fetch(cell, "rowspan"), int64(1)) || !rec.Equal(rec.Fetch(cell, "colspan"), int64(1)) ||
		rec.FetchOr(cell, "role", "body") != "body" {
		return false
	}
	for _, value := range rec.FetchList(cell, "content") {
		item := rec.AsMap(value)
		if item["type"] != "text" {
			return false
		}
		for key := range item {
			if key != "type" && key != "text" && key != "style" {
				return false
			}
		}
	}
	return true
}

func classicGrid(grid, fragment rec.Map, ordered []rec.Map) rec.Map {
	cells := map[any]rec.Map{}
	for _, value := range rec.FetchList(grid, "cells") {
		cell := rec.AsMap(value)
		cells[rec.Fetch(cell, "source_cell")] = cell
	}
	rows := []any{}
	ruleCount := 0
	for _, row := range ordered {
		line := nativeRow(row, cells)
		for _, cell := range line {
			ruleCount += len(rec.FetchList(rec.AsMap(cell), "borders"))
		}
		rows = append(rows, line)
	}
	horizontal := []any{}
	rules := rec.ListOf(fragment["rules"])
	for _, value := range rules {
		rule := rec.AsMap(value)
		if !truthy(rec.Fetch(rule, "in_row")) {
			horizontal = append(horizontal, horizontalBorder(rule, ordered))
		}
	}
	if len(rules) != len(horizontal)+ruleCount {
		rec.Fail("Classic table projection lost native rules")
	}
	result := rec.Map{"preamble": rec.Fetch(grid, "native_preamble"), "width_sp": rec.Fetch(ordered[0], "width"),
		"rows": rows, "horizontal_borders": horizontal}
	return tablemodel.ValidateShape(result)
}

func nativeRow(row rec.Map, cells map[any]rec.Map) []any {
	result := []any{}
	for _, value := range rec.FetchList(row, "cells") {
		entry := rec.AsMap(value)
		cell, ok := cells[rec.Fetch(entry, "cell")]
		if !ok {
			panic(rec.Failure{Message: "key not found: " + rec.ToS(rec.Fetch(entry, "cell")), Lookup: true})
		}
		runs := []rec.Map{}
		for _, item := range rec.FetchList(cell, "content") {
			run := rec.Map{}
			for key, field := range rec.AsMap(item) {
				if key != "type" {
					run[key] = field
				}
			}
			runs = append(runs, run)
		}
		runs = trimRuns(runs)
		var text strings.Builder
		styled := false
		listed := []any{}
		for _, run := range runs {
			text.WriteString(rec.FetchString(run, "text"))
			_, has := run["style"]
			styled = styled || has
			listed = append(listed, run)
		}
		native := rec.Map{"text": text.String(), "width_sp": rec.Fetch(entry, "width"), "height_sp": rec.Fetch(entry, "height"),
			"depth_sp": rec.Fetch(entry, "depth"), "font_size_sp": rec.Fetch(entry, "font_size"), "borders": rec.Fetch(entry, "borders")}
		if styled {
			native["runs"] = listed
		}
		result = append(result, native)
	}
	return result
}

// trimRuns is TextStyle.trim_runs!.
func trimRuns(runs []rec.Map) []rec.Map {
	items := make([]rec.Map, len(runs))
	copy(items, runs)
	for len(items) > 0 && rec.Strip(rec.FetchString(items[0], "text")) == "" {
		items = items[1:]
	}
	for len(items) > 0 && rec.Strip(rec.FetchString(items[len(items)-1], "text")) == "" {
		items = items[:len(items)-1]
	}
	if len(items) == 0 {
		return items
	}
	items[0]["text"] = rec.LStrip(rec.FetchString(items[0], "text"))
	items[len(items)-1]["text"] = rec.RStrip(rec.FetchString(items[len(items)-1], "text"))
	return items
}

func horizontalBorder(rule rec.Map, rows []rec.Map) rec.Map {
	if truthy(rec.FetchOr(rule, "math", false)) {
		rec.Fail("Classic table projection encountered cell math")
	}
	if !rec.Equal(rec.Fetch(rule, "depth"), int64(0)) || !rubynum.Positive(rec.Fetch(rule, "height")) {
		rec.Fail("Classic table projection encountered non-horizontal rule")
	}
	rect := rec.FetchList(rule, "rect_sp")
	x, y, width, height := rect[0], rect[1], rect[2], rect[3]
	first := rows[0]
	if !rec.Equal(width, rec.Fetch(first, "width")) || rubynum.Less(int64(1), rubynum.Abs(rubynum.Sub(x, rec.FetchList(first, "origin_sp")[0]))) {
		rec.Fail("Classic table projection encountered a partial rule")
	}
	center := rubynum.Add(y, rubynum.Fdiv(height, 2.0))
	after := int64(0)
	for _, row := range rows {
		origin := rec.FetchList(row, "origin_sp")
		if !rubynum.Less(center, rubynum.Add(rubynum.Add(origin[len(origin)-1], rec.Fetch(row, "height")), rec.Fetch(row, "depth"))) {
			after++
		}
	}
	return rec.Map{"after_row": after, "width": rec.Fetch(rule, "width"), "height": rec.Fetch(rule, "height"), "depth": rec.Fetch(rule, "depth")}
}
