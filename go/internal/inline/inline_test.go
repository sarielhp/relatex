package inline

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

func TestDeepOrdersRichCells(t *testing.T) {
	cell := func(id string, row, column int64) rec.Map {
		return rec.Map{"id": id, "row": row, "column": column, "content": []any{rec.Map{"type": "text", "text": id}}}
	}
	grid := rec.Map{"schema_version": tablemodel.Version, "cells": []any{cell("b", 1, 0), cell("a", 0, 1), cell("c", 0, 0)}}
	spans := Deep([]any{rec.Map{"type": "text", "text": "x"}, rec.Map{"type": "table_grid", "grid": grid}})
	texts := []string{}
	for _, span := range spans {
		if text, ok := span["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	if len(spans) != 5 || texts[1] != "c" || texts[2] != "a" || texts[3] != "b" {
		t.Errorf("deep order %v", texts)
	}
}
