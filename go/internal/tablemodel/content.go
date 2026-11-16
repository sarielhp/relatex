// Package tablemodel is the document model's rich table (RichTable, decision 102): the table_grid
// span's grid as document.json holds it (a legacy classic grid normalized), its validation, its
// rows and occupancy, and the contracts (schemas/) it is checked by. The core's table evidence
// (tables) builds grids; every reader of the model reads them here.
package tablemodel

import "github.com/sarielhp/relatex/internal/rec"

// LegacyCellContent is a legacy grid cell's content as rich inline spans: its runs, or its
// plain text, each a text span.
func LegacyCellContent(cell rec.Map) []any {
	content := []any{}
	runs, ok := cell["runs"]
	if !ok {
		return append(content, rec.Map{"text": rec.Fetch(cell, "text"), "type": "text"})
	}
	for _, run := range rec.AsList(runs) {
		span := rec.Map{}
		for key, value := range rec.AsMap(run) {
			span[key] = value
		}
		span["type"] = "text"
		content = append(content, span)
	}
	return content
}

// AreaFill reports whether a native table rule paints an area inside a row: colortbl's
// \rowcolor and \cellcolor fill a cell with a rule as tall as the row and as wide as the
// cell. A fill is the cell's background, not a border at a row or column edge.
func AreaFill(rule rec.Map) bool {
	rect := FetchList(rule, "rect_sp")
	return truthy(fetchDefault(rule, "in_row", false)) && number(rect[2]) > fillThickness && number(rect[3]) > fillThickness
}

// SourceCell is the grid cell a native cell entry belongs to, or nil (a covered placeholder,
// a hidden column, a legacy grid).
func SourceCell(cells map[string]rec.Map, source any) rec.Map {
	if source == nil {
		return nil
	}
	for _, cell := range cells {
		if value, ok := cell["source_cell"]; ok && rec.Equal(value, source) {
			return cell
		}
	}
	return nil
}

// fillThickness is the thinnest a painted rectangle is in both directions to be an area and
// not a line: table rules are at most a few tenths of a point thick.
const fillThickness = 131_072
