package papersource

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/assets"
	"github.com/sarielhp/relatex/internal/glyphowner"
	"github.com/sarielhp/relatex/internal/reading"
	"github.com/sarielhp/relatex/internal/rec"
)

// A drawn picture goes to the image node made from its crop, table cells included; two identical
// crops go to their nodes in the order of the nodes' ids.
func TestDrawnPicturesFindTheirImageNodes(t *testing.T) {
	image := func(id, source string) rec.Map { return rec.Map{"type": "image", "id": id, "source": source} }
	document := rec.Map{"nodes": rec.Map{
		"n-b": rec.Map{"content": []any{rec.Map{"type": "text"}, image("i-2", "rendered/a.pdf")}},
		"n-a": rec.Map{"content": []any{image("i-1", "rendered/a.pdf")}},
		"n-t": rec.Map{"content": []any{rec.Map{"type": "table_grid", "grid": rec.Map{"cells": []any{
			rec.Map{"content": []any{image("i-3", "rendered/b.pdf")}}}}}}},
		"n-c": rec.Map{"content": []any{image("i-4", "figs/plot.pdf")}},
	}}
	cropped := map[string][]assets.DrawnPicture{
		"rendered/a.pdf": {{Page: 1, Box: [4]float64{1, 2, 3, 4}}, {Page: 2, Box: [4]float64{5, 6, 7, 8}}},
		"rendered/b.pdf": {{Page: 3, Box: [4]float64{0, 0, 9, 9}}},
	}
	pictures := drawnPictures(document, cropped)
	want := []glyphowner.Picture{{Node: "i-1", Page: 1, Box: [4]float64{1, 2, 3, 4}},
		{Node: "i-2", Page: 2, Box: [4]float64{5, 6, 7, 8}}, {Node: "i-3", Page: 3, Box: [4]float64{0, 0, 9, 9}}}
	if !slices.Equal(pictures, want) {
		t.Fatalf("pictures %+v", pictures)
	}
	if len(cropped["rendered/a.pdf"]) != 2 {
		t.Fatal("the cropped pictures were consumed")
	}
}

// A glyph of a formula belongs to the formula's math node on its page; another glyph to the
// paragraph its flow item went into; a glyph of no flow item to nothing.
func TestOwnerIndexBindsFormulasAndSlots(t *testing.T) {
	node := func(kind, key string) rec.Map {
		return rec.Map{"type": kind, "provenance": rec.Map{"inspection_key": key}}
	}
	document := rec.Map{"nodes": rec.Map{"n-par": node("paragraph", "b1"), "n-list": node("list_item", "b1"),
		"n-math": node("math", "m1"), "n-head": node("heading", "b2")}}
	retention := rec.Map{"occurrences": []any{rec.Map{"id": "m1", "formula": int64(7), "pages": []any{int64(2)}}}}
	slots := map[reading.SlotKey]string{{Owner: 3, Slot: 1, Page: 2}: "b1", {Owner: 4, Slot: 2, Page: 2}: "b2"}
	index := newOwnerIndex(document, retention, slots)
	cases := []struct {
		formula, owner, slot int64
		want                 string
	}{{7, 3, 1, "n-math"}, {0, 3, 1, "n-par"}, {0, 4, 2, "n-head"}, {0, 0, 0, ""}, {0, 3, 9, ""}, {8, 4, 2, "n-head"}}
	for _, c := range cases {
		if got := index.owner(2, c.formula, c.owner, c.slot); got != c.want {
			t.Errorf("formula %d owner %d slot %d: %q, want %q", c.formula, c.owner, c.slot, got, c.want)
		}
	}
	page := glyphMapPage{Page: 2}
	page.Glyphs.Text = []string{"x", "y"}
	page.Glyphs.DX, page.Glyphs.DY = []int64{65536 * 72, 0}, []int64{0, 0}
	page.Glyphs.Formula, page.Glyphs.Owner, page.Glyphs.Slot = []int64{7, 0}, []int64{3, 4}, []int64{1, 2}
	page.Glyphs.Flags = []int64{0, unknownPosition}
	index.page(page)
	got := index.owners.Pages[0]
	if index.owners.Owner(got, 0) != "n-math" || index.owners.Owner(got, 1) != "n-head" || len(got.Unknown) != 1 {
		t.Fatalf("page %+v, leaves %v", got, index.owners.Leaves)
	}
	// "dif-" / "ferent": the hyphen and the broken ligature's post-break f have no flow item.
	leaves := []int{3, 0, 0, 3, 0, 4, 0, 0, 0, 4}
	inheritBroken(leaves, []int64{1, 0, 0, 1, 0, 1, 0, 0, 0, 1}, make([]int64, 10))
	if !slices.Equal(leaves, []int{3, 3, 3, 3, 0, 4, 0, 0, 0, 4}) {
		t.Errorf("broken glyphs %v", leaves)
	}
	if got.X[0] != 71.73 {
		t.Errorf("72 pt is %v bp, want 71.73", got.X[0])
	}
}
