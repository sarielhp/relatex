// Package tablehints applies the author's table hints (decisions 131 and 188) to the document: a table
// float named by its \label or caption number gets its header rows and columns from the hint,
// written into each of its table grids as tablelayout.AuthorHeaderRows and AuthorHeaderColumns; a
// key with a position ("fig:wspd#1") names one table of a float, which is how a table inside a figure
// float is named. The page, the tagged PDF, the report and the acceptance then read the author's word
// from the grid, as they read the tagging project's declared_header_rows.
package tablehints

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/authorhints"
	"github.com/sarielhp/relatex/internal/gaplocate"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
)

// Apply writes each table hint into the grids of the tables it names and returns the keys that name
// no table of the document (a label of something else, a number no caption prints). A key names a
// table float whole ("tab:a", "3"), or one table of any float by its position in it ("fig:wspd#1":
// the first table of the float in reading order, decision 188).
func Apply(document rec.Map, hints *authorhints.Hints) (unmatched []string) {
	if hints == nil || len(hints.Sections[authorhints.TablesSection]) == 0 {
		return nil
	}
	nodes, _ := document["nodes"].(rec.Map)
	keys := floatKeys(document, nodes, floatNodes(nodes))
	located := gaplocate.New(document, nil)
	names := make([]string, 0, len(hints.Sections[authorhints.TablesSection]))
	for key := range hints.Sections[authorhints.TablesSection] {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		hint, valid := hints.Table(key)
		grids, found := hintGrids(key, nodes, keys, located)
		if !found {
			unmatched = append(unmatched, key)
		}
		if !valid || !found {
			continue
		}
		for _, grid := range grids {
			apply(grid, hint)
		}
	}
	return unmatched
}

// Position splits "fig:wspd#2" into the float's key and the table's position in it (1-based); a key
// without a position has 0.
func Position(key string) (string, int) {
	cut := strings.LastIndexByte(key, '#')
	if cut < 0 {
		return key, 0
	}
	number, err := strconv.Atoi(key[cut+1:])
	if err != nil || number < 1 {
		return key, 0
	}
	return key[:cut], number
}

// hintGrids are the grids a hint key names: every grid of the table float a plain key names, or the
// grid at the key's position in the first float (table floats before figure floats) that has one.
func hintGrids(key string, nodes rec.Map, keys map[string][]string, located *gaplocate.Locator) ([]rec.Map, bool) {
	base, at := Position(key)
	if at == 0 {
		for _, float := range keys[key] {
			if rec.AsMap(nodes[float])["type"] == "table_float" {
				return floatGrids(nodes, float), true
			}
		}
		return nil, false
	}
	for _, float := range keys[base] {
		tables := located.FloatTables(float)
		if at > len(tables) {
			continue
		}
		if grid := located.Grid(tables[at-1]); grid != nil {
			return []rec.Map{grid}, true
		}
	}
	return nil, false
}

// apply sets a grid's author keys: the first N rows (columns) the hint names, as many as it has.
func apply(grid rec.Map, hint authorhints.TableHint) {
	if hint.Rows != nil {
		grid[tablelayout.AuthorHeaderRows] = firstIndices(*hint.Rows, rec.FetchInt(grid, "row_count"))
	}
	if hint.Columns != nil {
		grid[tablelayout.AuthorHeaderColumns] = firstIndices(*hint.Columns, rec.FetchInt(grid, "column_count"))
	}
}

func firstIndices(count int, limit int64) []any {
	indices := []any{}
	for index := int64(0); index < min(int64(count), limit); index++ {
		indices = append(indices, index)
	}
	return indices
}

// isFloat is a node type that holds tables a hint can name.
func isFloat(node rec.Map) bool {
	return node["type"] == "table_float" || node["type"] == "figure_float"
}

// floatNodes maps every node inside a table or figure float to its innermost float, the floats
// themselves included.
func floatNodes(nodes rec.Map) map[string]string {
	owners := map[string]string{}
	var walk func(id, float string)
	walk = func(id, float string) {
		node, ok := nodes[id].(rec.Map)
		if !ok {
			return
		}
		if isFloat(node) {
			float = id
		}
		owners[id] = float
		for _, child := range rec.ListOf(node["children"]) {
			if name, ok := child.(string); ok {
				walk(name, float)
			}
		}
	}
	for id, value := range nodes {
		if isFloat(rec.AsMap(value)) {
			walk(id, id)
		}
	}
	return owners
}

// floatKeys are the keys that name each float: the labels whose target lies in it, their numbers, and
// the number its caption prints. A key may name several floats (a table and a figure with one number):
// table floats come first, then figure floats, each by node id.
func floatKeys(document, nodes rec.Map, owners map[string]string) map[string][]string {
	keys := map[string][]string{}
	add := func(key, float string) {
		for _, known := range keys[key] {
			if known == float {
				return
			}
		}
		keys[key] = append(keys[key], float)
	}
	ids := make([]string, 0, len(owners))
	for id := range owners {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if owners[id] == id {
			if number := captionNumber(nodes, id); number != "" {
				add(number, id)
			}
		}
	}
	labels, _ := document["labels"].(rec.Map)
	names := make([]string, 0, len(labels))
	for label := range labels {
		names = append(names, label)
	}
	sort.Strings(names)
	for _, label := range names {
		entry := rec.AsMap(labels[label])
		target, _ := entry["target"].(string)
		float, ok := owners[target]
		if !ok {
			continue
		}
		add(label, float)
		if number, _ := entry["number"].(string); number != "" {
			add(number, float)
		}
	}
	for key, floats := range keys {
		sort.SliceStable(floats, func(i, j int) bool {
			return rec.AsMap(nodes[floats[i]])["type"] == "table_float" && rec.AsMap(nodes[floats[j]])["type"] != "table_float"
		})
		keys[key] = floats
	}
	return keys
}

// captionText is how a caption's text begins: "Table 3: …", "Tab. A.1. …", "TABLE IV …".
var captionText = regexp.MustCompile(`^\s*\p{L}+\.?[\s\x{a0}]+([A-Z]?[0-9]+(?:\.[0-9]+)*|[A-Z]\.[0-9]+(?:\.[0-9]+)*|[IVXLC]+)(?:[:.\s\x{a0}]|$)`)

// captionNumber is the number a float's caption prints after its name, "" without one.
func captionNumber(nodes rec.Map, float string) string {
	for _, child := range rec.ListOf(rec.AsMap(nodes[float])["children"]) {
		caption, _ := nodes[rec.AsString(child)].(rec.Map)
		if caption["type"] != "float_caption" {
			continue
		}
		if match := captionText.FindStringSubmatch(leadingText(nodes, caption)); match != nil {
			return match[1]
		}
		return ""
	}
	return ""
}

// leadingText is the text of a node's first paragraph with text: its text spans up to the first
// span of another kind.
func leadingText(nodes rec.Map, node rec.Map) string {
	var text strings.Builder
	for _, value := range rec.ListOf(node["content"]) {
		span := rec.AsMap(value)
		if span["type"] != "text" {
			break
		}
		text.WriteString(rec.AsString(span["text"]))
	}
	if text.Len() > 0 {
		return text.String()
	}
	for _, child := range rec.ListOf(node["children"]) {
		if inner, ok := nodes[rec.AsString(child)].(rec.Map); ok {
			if found := leadingText(nodes, inner); found != "" {
				return found
			}
		}
	}
	return ""
}

// floatGrids are the grids of the table_grid spans in a float's own nodes (not those of a float
// nested in it, nor tables nested in cells).
func floatGrids(nodes rec.Map, float string) []rec.Map {
	grids := []rec.Map{}
	var walk func(id string)
	walk = func(id string) {
		node, ok := nodes[id].(rec.Map)
		if !ok || id != float && node["type"] == "table_float" {
			return
		}
		for _, value := range rec.ListOf(node["content"]) {
			if span := rec.AsMap(value); span["type"] == "table_grid" {
				if grid, ok := span["grid"].(rec.Map); ok {
					grids = append(grids, grid)
				}
			}
		}
		for _, child := range rec.ListOf(node["children"]) {
			walk(rec.AsString(child))
		}
	}
	walk(float)
	return grids
}
