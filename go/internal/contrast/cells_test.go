package contrast

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// cellTable is a rich table span whose cells hold content, each with its colortbl fill ("" none).
func cellTable(cells ...rec.Map) rec.Map {
	list := []any{}
	for _, cell := range cells {
		list = append(list, cell)
	}
	return rec.Map{"type": "table_grid", "grid": rec.Map{"schema_version": tablemodel.Version, "cells": list}}
}

func filledCell(fill string, content ...any) rec.Map {
	cell := rec.Map{"content": content}
	if fill != "" {
		cell["fill"] = fill
	}
	return cell
}

// A cell's colortbl fill is behind every span of its content no fill rule of its own covers; a
// table nested in a filled cell sets its unfilled cells on the outer fill (the browser shows that
// cell's background through them), and a malformed fill is none.
func TestCellFillsAreBehindTheirContent(t *testing.T) {
	if CellFill(rec.Map{"fill": "#333333"}) != "#333333" || CellFill(rec.Map{"fill": "red"}) != "" || CellFill(rec.Map{}) != "" {
		t.Error("CellFill reads only a #rrggbb fill")
	}
	on := BehindOn([]any{colored("#ffffff"), fillRule("#ffff00", 400000), colored("#000080"), colored("#000080")}, "#333333", nil)
	if !slices.Equal(on, []string{"#333333", "#333333", "#ffff00", "#333333"}) {
		t.Errorf("behind on a cell fill = %v", on)
	}
	nested := cellTable(filledCell("", colored("#ffffff")))
	outer := cellTable(filledCell("#1f3864", colored("#ffffff"), nested), filledCell("", colored("#808080")))
	document := rec.Map{"nodes": rec.Map{"t": rec.Map{"type": "paragraph", "content": []any{outer}}}}
	inks := Inks(document)
	if !slices.Equal(inks["#ffffff"], []string{"fill:#1f3864"}) || !slices.Equal(inks["#808080"], []string{"page"}) {
		t.Errorf("inks = %v", inks)
	}
	if fills := Fills(document); !slices.Equal(fills, []string{"#1f3864"}) {
		t.Errorf("fills = %v", fills)
	}
}

// White on a dark cell reads: no adjustment and no hidden ink. Grey on a light grey cell fails
// there and is adjusted against that fill (darker, decision 099), apart from its page use.
func TestInksOnCellFillsAreAdjustedAgainstTheCell(t *testing.T) {
	table := cellTable(filledCell("#333333", colored("#ffffff")), filledCell("#dfdfdf", colored("#808080")))
	document := rec.Map{"nodes": rec.Map{
		"t": rec.Map{"type": "paragraph", "content": []any{table}},
		"p": rec.Map{"type": "paragraph", "content": []any{colored("#808080")}},
	}}
	record := Adjustments(document)
	items := rec.FetchList(record, "items")
	if len(items) != 2 {
		t.Fatalf("items = %v", items)
	}
	onFill := rec.AsMap(items[1])
	if onFill["fill"] != "#dfdfdf" || onFill["ink"] != "#808080" {
		t.Fatalf("fill item = %v", onFill)
	}
	adjusted, _ := Parse(rec.FetchString(onFill, "adjusted"))
	fill, _ := Parse("#dfdfdf")
	if Ratio(adjusted, fill) < TextRatio {
		t.Errorf("adjusted %v reads %.2f on the cell", onFill["adjusted"], Ratio(adjusted, fill))
	}
	if Hidden(document)["#ffffff"] {
		t.Error("white on a dark cell is painted, not hidden")
	}
}

// On a page canvas (decision 107) a cell's own fill is innermost: text in a filled cell is on the
// cell, text in an unfilled cell and outside the table on the canvas.
func TestCellFillIsInsideThePageCanvas(t *testing.T) {
	table := cellTable(filledCell("#ffff00", colored("#808080")), filledCell("", colored("#202020")))
	document := rec.Map{"text_paint": rec.Map{"canvas": "#23373b"}, "nodes": rec.Map{
		"t": rec.Map{"type": "paragraph", "content": []any{table, colored("#ffffff")}},
	}}
	inks := Inks(document)
	if !slices.Equal(inks["#808080"], []string{"fill:#ffff00"}) || !slices.Equal(inks["#202020"], []string{"fill:#23373b"}) ||
		!slices.Equal(inks["#ffffff"], []string{"fill:#23373b"}) {
		t.Errorf("inks = %v", inks)
	}
	if fills := Fills(document); !slices.Equal(fills, []string{"#23373b", "#ffff00"}) {
		t.Errorf("fills = %v", fills)
	}
}
