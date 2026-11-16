package accessibility

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// placedFixture is a figure with a table and a caption naming a colour, and a source map with the
// caption's words.
func placedFixture() (document, sourceMap rec.Map) {
	cell := rec.Map{"row": int64(0), "column": int64(0), "content": []any{rec.Map{"type": "text", "text": "x"}}}
	document = rec.Map{
		"block_order": []any{"c1"},
		"nodes": rec.Map{
			"f1": rec.Map{"id": "f1", "type": "figure_float", "children": []any{"p1", "k1"}},
			"p1": rec.Map{"id": "p1", "type": "paragraph", "children": []any{}, "content": []any{
				rec.Map{"type": "table_grid", "id": "g1", "grid": rec.Map{"cells": []any{cell}}}}},
			"k1": rec.Map{"id": "k1", "type": "float_caption", "children": []any{"c1"}},
			"c1": rec.Map{"id": "c1", "type": "paragraph", "children": []any{}, "content": []any{
				rec.Map{"type": "text", "text": "Figure 3.2: The red edges of the tree."}}},
		},
	}
	words := []any{}
	for _, text := range []string{"The", "red", "edges", "of", "the", "tree."} {
		words = append(words, rec.Map{"text": text, "origins": []any{rec.Map{"file": "a/b.tex", "line": int64(41)}}})
	}
	return document, rec.Map{"words": words}
}

// An item carries the file and line of its caption's words, and a table its place in its float.
func TestItemsCarryTheirSourcePlace(t *testing.T) {
	document, sourceMap := placedFixture()
	walker := newWalk(document, sourceMap)
	colours, _ := colourItems(document, walker)
	if len(colours) != 1 {
		t.Fatalf("colour items = %v", colours)
	}
	item := rec.MapOf(colours[0])
	if item["file"] != "a/b.tex" || item["line"] != int64(41) || item["number"] != "3.2" || item["float"] != "figure_float" {
		t.Errorf("colour item = %v", item)
	}
	nodes := rec.MapOf(document["nodes"])
	grid := rec.MapOf(rec.SliceOf(rec.MapOf(nodes["p1"])["content"])[0])
	tables, _ := tableItems([]rec.Map{grid}, nodes, walker.places)
	if len(tables) != 1 {
		t.Fatalf("table items = %v", tables)
	}
	table := rec.MapOf(tables[0])
	if table["file"] != "a/b.tex" || table["line"] != int64(41) || table["table"] != int64(1) || table["of"] != int64(1) {
		t.Errorf("table item = %v", table)
	}
}

// Without a source map the items have no place, and nothing fails.
func TestItemsWithoutASourceMapHaveNoPlace(t *testing.T) {
	document, _ := placedFixture()
	gaps, _ := colourItems(document, newWalk(document, nil))
	item := rec.MapOf(gaps[0])
	if _, has := item["file"]; has {
		t.Errorf("colour item = %v", item)
	}
	if item["caption"] == "" || item["number"] != "3.2" {
		t.Errorf("colour item = %v", item)
	}
}
