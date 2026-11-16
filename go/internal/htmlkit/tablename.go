package htmlkit

import (
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
)

// TableName is the table_grid a table_float names with its first caption (a11y-804): the ids of
// the table and of its caption. A float of several top-level tables (subtables with their own
// captions) names none, a presumed layout keeps role="presentation" (an aria-labelledby would
// override it, a11y-802), and nested tables are never reached: they sit in a grid's cells.
func TableName(float, nodes rec.Map) (table, caption string, named bool) {
	if float["type"] != "table_float" {
		return "", "", false
	}
	for _, child := range rec.FetchList(float, "children") {
		if rec.Node(nodes, child)["type"] == "float_caption" {
			caption = rec.ToS(child)
			break
		}
	}
	if caption == "" {
		return "", "", false
	}
	spans := topLevelTableSpans(float, nodes)
	if len(spans) != 1 || tablelayout.Presentational(rec.FetchMap(spans[0], "grid"), nodes) {
		return "", "", false
	}
	return rec.ToS(rec.Fetch(spans[0], "id")), caption, true
}

// TableNames are the tables every table_float in a document's nodes names, by table_grid id.
func TableNames(nodes rec.Map) map[string]string {
	names := map[string]string{}
	for _, value := range nodes {
		if float, ok := value.(rec.Map); ok {
			if table, caption, named := TableName(float, nodes); named {
				names[table] = caption
			}
		}
	}
	return names
}

// FigureGrouped says whether a float's <figure> is a plain group (decision 183, amendment 1). A
// figure is named once: a float with a figcaption is named by it natively (HTML-AAM), a caption
// written as a division between the float's blocks is read once where it stands, and neither
// carries an aria-labelledby. A table_float whose table the caption names (TableName) names the
// caption once, at the table: its figure is role="group".
func FigureGrouped(float, nodes rec.Map) bool {
	_, _, named := TableName(float, nodes)
	return named
}

// topLevelTableSpans are the table_grid spans the float holds in its blocks' content.
func topLevelTableSpans(float, nodes rec.Map) []rec.Map {
	spans := []rec.Map{}
	var walk func(rec.Map)
	walk = func(node rec.Map) {
		content, _ := node["content"].([]any)
		for _, value := range content {
			if span := rec.AsMap(value); span["type"] == "table_grid" {
				spans = append(spans, span)
			}
		}
		children, _ := node["children"].([]any)
		for _, child := range children {
			next := rec.Node(nodes, child)
			if next["type"] == "figure_float" || next["type"] == "table_float" {
				continue // a nested float names its own table
			}
			walk(next)
		}
	}
	walk(float)
	return spans
}
