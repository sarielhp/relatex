package gaplocate

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func fixture() (document, sourceMap rec.Map) {
	inner := rec.Map{"type": "table_grid", "id": "inner", "grid": rec.Map{"cells": []any{}}}
	outer := rec.Map{"type": "table_grid", "id": "outer", "grid": rec.Map{"cells": []any{
		rec.Map{"content": []any{inner, rec.Map{"type": "image", "source_file": "x/fig.tex", "source_line": int64(9)}}}}}}
	document = rec.Map{
		"labels": rec.Map{"tab:one": rec.Map{"target": "p1"}},
		"nodes": rec.Map{
			"f1": rec.Map{"id": "f1", "type": "table_float", "children": []any{"p1", "k1"}},
			"p1": rec.Map{"id": "p1", "type": "paragraph", "children": []any{}, "content": []any{outer}},
			// A caption that opens with a formula: the lookup falls to its second text run.
			"k1": rec.Map{"id": "k1", "type": "float_caption", "children": []any{}, "content": []any{
				rec.Map{"type": "text", "text": "Table 7: (A)"}, rec.Map{"type": "math", "node": "m"},
				rec.Map{"type": "text", "text": " Counting the pets and plants."}}},
		},
	}
	words := []any{}
	for _, text := range []string{"Counting", "the", "pets", "and", "plants."} {
		words = append(words, rec.Map{"text": text, "origins": []any{rec.Map{"file": "t.tex", "line": int64(5)}}})
	}
	return document, rec.Map{"words": words}
}

// A table is placed by its float's caption, found through a later text run when the first has too few
// words, with its position among the float's tables (outer first), its label and the caption's number.
func TestTablePlace(t *testing.T) {
	document, sourceMap := fixture()
	locator := New(document, sourceMap)
	outer, inner := locator.Table("outer"), locator.Table("inner")
	if outer.File != "t.tex" || outer.Line != 5 || outer.Index != 1 || outer.Of != 2 || inner.Index != 2 {
		t.Errorf("outer = %+v, inner = %+v", outer, inner)
	}
	if outer.Number != "7" || outer.Label != "tab:one" || outer.Float != "table_float" {
		t.Errorf("outer = %+v", outer)
	}
	if outer.ImageFile != "x/fig.tex" || outer.ImageLine != 9 {
		t.Errorf("image place of outer = %s:%d", outer.ImageFile, outer.ImageLine)
	}
	if locator.Grid("inner") == nil || locator.Grid("none") != nil || locator.Table("none").Float != "" {
		t.Error("grid lookup of unknown ids")
	}
}

// Without a source map the first figure inside a table places it; without either there is no place.
func TestTablePlaceFallsBackToItsFigure(t *testing.T) {
	document, _ := fixture()
	place := New(document, nil).Table("outer")
	if place.File != "x/fig.tex" || place.Line != 9 {
		t.Errorf("place = %+v", place)
	}
	if place := New(document, nil).Table("inner"); place.File != "" {
		t.Errorf("inner = %+v", place)
	}
}
