package html

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// testGrid is a rich grid of rows x columns of one-text cells; a nested rich grid in its first
// cell, and its declared header rows, when given.
func testGrid(rows, columns int, declared []any, nested rec.Map) rec.Map {
	cells := []any{}
	for row := range rows {
		for column := range columns {
			content := []any{rec.Map{"type": "text", "text": "x"}}
			if nested != nil && row == 0 && column == 0 {
				content = append(content, rec.Map{"type": "table_grid", "id": "nested", "grid": nested})
			}
			cells = append(cells, rec.Map{"id": fmt.Sprintf("c%d-%d", row, column), "row": int64(row), "column": int64(column),
				"rowspan": int64(1), "colspan": int64(1), "alignment": "l", "vertical_align": "top", "content": content})
		}
	}
	grid := rec.Map{"schema_version": tablemodel.Version, "row_count": int64(rows), "column_count": int64(columns), "cells": cells}
	if declared != nil {
		grid["declared_header_rows"] = declared
	}
	return grid
}

func tableSpan(id string, grid rec.Map) rec.Map {
	return rec.Map{"id": id, "type": "table_grid", "grid": grid}
}

// opener is the page's first table start tag.
func opener(page string) string {
	start := strings.Index(page, "<table")
	end := strings.Index(page[start:], ">")
	return page[start : start+end]
}

// A 1x1 grid nested in a cell (makecell's or multirowcell's tabular) is written role="presentation",
// so a reader announces no layout table as data; the 2x2 grid holding it stays a data table.
func TestNestedLayoutGridIsPresentational(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	paper := &Paper{document: rec.Map{}, nodes: rec.Map{}}
	page := paper.richTableSpan(tableSpan("outer", testGrid(2, 2, nil, testGrid(1, 1, nil, nil))))
	if strings.HasPrefix(opener(page), `<table role="presentation"`) {
		t.Errorf("the 2x2 grid is no layout: %s", opener(page))
	}
	inner := page[strings.Index(page, "<table")+len("<table"):]
	inner = inner[strings.Index(inner, "<table"):] // the nested grid, after the outer one
	if !strings.HasPrefix(opener(inner), `<table role="presentation"`) {
		t.Errorf("the nested 1x1 grid is a layout: %s", opener(inner))
	}
	if strings.Count(page, `role="presentation"`) != 1 {
		t.Errorf("presentation roles = %d", strings.Count(page, `role="presentation"`))
	}
}

// A top-level table of one column (papers/0060's 6x1 author block) is presumed a layout too.
func TestTopLevelLayoutGridIsPresentational(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	page := (&Paper{document: rec.Map{}, nodes: rec.Map{}}).richTableSpan(tableSpan("authors", testGrid(6, 1, nil, nil)))
	if !strings.HasPrefix(opener(page), `<table role="presentation"`) {
		t.Errorf("the 6x1 grid is a layout: %s", opener(page))
	}
}

// A one-row grid whose row is a header row is a data table: the accessibility report counts its
// header, not a presumed layout.
func TestHeaderRowGridIsNoLayout(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	page := (&Paper{document: rec.Map{}, nodes: rec.Map{}}).richTableSpan(tableSpan("header", testGrid(1, 3, []any{int64(0)}, nil)))
	if strings.Contains(opener(page), "role=") {
		t.Errorf("a grid with a header row is no layout: %s", opener(page))
	}
}

func TestDataGridIsNoLayout(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	page := (&Paper{document: rec.Map{}, nodes: rec.Map{}}).richTableSpan(tableSpan("data", testGrid(3, 3, nil, nil)))
	if strings.Contains(opener(page), "role=") {
		t.Errorf("the 3x3 grid is no layout: %s", opener(page))
	}
}
