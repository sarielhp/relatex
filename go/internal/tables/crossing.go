package tables

import (
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// rowEdge is a horizontal rule printed between two logical rows: after is the row above it (an
// index into the logical rows), from and to its extent in the rows' own coordinates.
type rowEdge struct {
	after    int64
	from, to float64
}

// cutAtCrossingRules ends a \multirow cell at a horizontal rule printed across all its columns
// inside its span. \multirow reserves nothing: a span longer than its group (a \multirow{9} over
// eight rows before an \hline) lowers its text past a rule that a grid can only keep as an edge
// between rows; the rows it no longer spans keep their own (empty) cells (fourth cohort 1159).
func cutAtCrossingRules(cells []rec.Map, fragments []any, boundaries []int64) {
	edges := ruleEdges(fragments)
	for _, cell := range cells {
		row, span := fetchInt(cell, "row"), fetchInt(cell, "rowspan")
		column := fetchInt(cell, "column")
		left := float64(boundaries[column])
		right := float64(boundaries[min(column+fetchInt(cell, "colspan"), int64(len(boundaries)-1))])
		for _, edge := range edges {
			if row <= edge.after && edge.after < row+span-1 && edge.from <= left+2 && edge.to >= right-2 {
				span = edge.after - row + 1
			}
		}
		cell["rowspan"] = span
	}
}

// ruleEdges are the native horizontal rules outside rows, each at the row edge nearest to it, as
// the layout check reads them.
func ruleEdges(fragments []any) []rowEdge {
	edges := []rowEdge{}
	for _, fragment := range maps(fragments) {
		rows := maps(tablemodel.FetchList(fragment, "rows"))
		for _, rule := range maps(tablemodel.FetchList(fragment, "rules")) {
			rect := tablemodel.FetchList(rule, "rect_sp")
			if truthy(fetchDefault(rule, "math", false)) || truthy(fetchDefault(rule, "in_row", false)) ||
				number(rect[2]) < number(rect[3]) {
				continue
			}
			if edge, ok := nearestEdge(rows, rect); ok {
				edges = append(edges, edge)
			}
		}
	}
	return edges
}

func nearestEdge(rows []rec.Map, rect []any) (rowEdge, bool) {
	center := number(rect[1]) + float64(number(rect[3])/2.0)
	var native rec.Map
	after, best := int64(0), 0.0
	for _, row := range rows {
		if rec.Fetch(row, "row") == nil {
			continue
		}
		origin := tablemodel.FetchList(row, "origin_sp")
		top := number(origin[len(origin)-1])
		bottom := top + number(rec.Fetch(row, "height")) + number(rec.Fetch(row, "depth"))
		index := fetchInt(row, "row")
		if distance := abs(top - center); native == nil || distance < best {
			native, after, best = row, index-1, distance
		}
		if distance := abs(bottom - center); distance < best {
			native, after, best = row, index, distance
		}
	}
	if native == nil || after < 0 {
		return rowEdge{}, false
	}
	from := number(rect[0]) - number(rec.Index(tablemodel.FetchList(native, "origin_sp"), 0))
	return rowEdge{after: after, from: from, to: from + number(rect[2])}, true
}
