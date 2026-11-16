package assets

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A TikZ picture in a table cell is rendered like one in a paragraph (third cohort 593, 776).
func TestRenderedSpansInTableCells(t *testing.T) {
	picture := rec.Map{"type": "rendered_graphic", "id": "r2"}
	grid := rec.Map{"cells": []any{rec.Map{"content": []any{rec.Map{"type": "text", "text": "A"}}},
		rec.Map{"content": []any{picture}}}}
	reading := rec.Map{"notes": rec.Map{"notes": []any{}}, "blocks": []any{rec.Map{"content": []any{
		rec.Map{"type": "rendered_graphic", "id": "r1"}, rec.Map{"type": "table_grid", "grid": grid}}}}}
	spans := renderedSpans(reading)
	if len(spans) != 2 || spans[1]["id"] != "r2" {
		t.Fatalf("rendered spans %v", spans)
	}
}
