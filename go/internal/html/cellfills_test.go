package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// A cell with a colortbl fill is a fill box (data-fill-box, painted by the theme's fill rule) and
// its text takes the fill's inks: white its ink on the fill, uncoloured text the fill's own ink
// (data-fill). A table nested in it and the cells after it start from their own fills.
func TestCellContentIsSetOnTheCellFill(t *testing.T) {
	white := rec.Map{"type": "text", "text": "Head", "color": "#ffffff"}
	plain := rec.Map{"type": "text", "text": "plain"}
	filled := rec.Map{"id": "cell-1", "fill": "#333333", "content": []any{white, plain}}
	bare := rec.Map{"id": "cell-2", "content": []any{rec.Map{"type": "text", "text": "page", "color": "#808080"}}}
	grid := rec.Map{"schema_version": tablemodel.Version, "cells": []any{filled, bare}}
	document := rec.Map{"nodes": rec.Map{"t": rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "table_grid", "grid": grid}}}}}
	document[contrast.Field] = contrast.Adjustments(document)
	paper := &Paper{document: document, nodes: rec.FetchMap(document, "nodes")}
	content, box := paper.cellContent(filled)
	if box != `data-fill-box="333333" ` {
		t.Errorf("fill box = %q", box)
	}
	for _, want := range []string{"var(--ink-ffffff-on-333333)", `data-fill="333333" data-kind="text">plain<`} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %s in %s", want, content)
		}
	}
	if paper.cellFill != "" {
		t.Errorf("the cell's fill outlives it: %q", paper.cellFill)
	}
	content, box = paper.cellContent(bare)
	if box != "" || strings.Contains(content, "data-fill") || !strings.Contains(content, "var(--ink-808080)") {
		t.Errorf("a cell without a fill = %q %s", box, content)
	}
	dark, light, rules := fillInksCSS(document)
	for _, want := range []string{"--fill-333333: #333333;", "--ink-on-333333: var(--ink);"} {
		if !strings.Contains(light, want) {
			t.Errorf("light theme lacks %s: %s", want, light)
		}
	}
	if !strings.Contains(dark, "--fill-333333: #333333;") || !strings.Contains(rules, `[data-fill-box="333333"]`) {
		t.Errorf("a dark fill stays in the dark theme and is painted: %s / %s", dark, rules)
	}
}

// On a page canvas (decision 107) an unfilled cell's text is on the canvas, a filled cell's on
// the cell's own fill.
func TestCellContentOnACanvas(t *testing.T) {
	plain := rec.Map{"type": "text", "text": "plain"}
	filled := rec.Map{"id": "cell-1", "fill": "#ffff00", "content": []any{plain}}
	bare := rec.Map{"id": "cell-2", "content": []any{plain}}
	grid := rec.Map{"schema_version": tablemodel.Version, "cells": []any{filled, bare}}
	document := rec.Map{"text_paint": rec.Map{"canvas": "#23373b"},
		"nodes": rec.Map{"t": rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "table_grid", "grid": grid}}}}}
	paper := &Paper{document: document, nodes: rec.FetchMap(document, "nodes")}
	if content, _ := paper.cellContent(filled); !strings.Contains(content, `data-fill="ffff00"`) {
		t.Errorf("filled cell on the canvas = %s", content)
	}
	if content, _ := paper.cellContent(bare); !strings.Contains(content, `data-fill="23373b"`) {
		t.Errorf("unfilled cell on the canvas = %s", content)
	}
}
