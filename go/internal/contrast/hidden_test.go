package contrast

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// White text on the page is hidden by its author (papers 236 and 587): it is not adjusted. A
// visible light grey still is, and white inside a table (a cell may paint behind it) is not hidden.
func TestHiddenInksAreNotAdjusted(t *testing.T) {
	span := func(color string) any { return rec.Map{"type": "text", "text": "x", "color": color} }
	cell := rec.Map{"id": "c1", "content": []any{span("#fefefe")}}
	grid := rec.Map{"type": "table_grid", "grid": rec.Map{"schema_version": tablemodel.Version, "cells": []any{cell}}}
	document := rec.Map{"nodes": rec.Map{"p": rec.Map{"type": "paragraph",
		"content": []any{span("#ffffff"), span("#fdfdfd"), span("#e0e0e0"), grid}}}}
	hidden := Hidden(document)
	if !hidden["#ffffff"] || !hidden["#fdfdfd"] || hidden["#e0e0e0"] || hidden["#fefefe"] {
		t.Fatalf("hidden = %v", hidden)
	}
	adjusted := map[string]bool{}
	for _, value := range rec.FetchList(Adjustments(document), "items") {
		adjusted[rec.FetchString(rec.AsMap(value), "ink")] = true
	}
	if adjusted["#ffffff"] || adjusted["#fdfdfd"] || !adjusted["#e0e0e0"] || !adjusted["#fefefe"] {
		t.Fatalf("adjusted = %v", adjusted)
	}
}

// A heading in white is a beamer frame title on its painted bar (decision 095): it is not hidden.
func TestHeadingInksAreNotHidden(t *testing.T) {
	white := rec.Map{"type": "text", "text": "x", "color": "#ffffff"}
	document := rec.Map{"nodes": rec.Map{"h": rec.Map{"type": "heading", "content": []any{white}},
		"p": rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": "y", "color": "#fdfdfd"}}}}}
	if hidden := Hidden(document); hidden["#ffffff"] || !hidden["#fdfdfd"] {
		t.Fatalf("hidden = %v", hidden)
	}
}
