package html

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// tableFixture is a table_grid span from a cohort paper (testdata), its fonts, glyphs and math
// left out.
func tableFixture(t *testing.T, name string) rec.Map {
	t.Helper()
	tablemodel.Root = filepath.Join("..", "..", "..") // the schemas the grid is validated by
	file, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	span, err := canonjson.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return rec.AsMap(span)
}

func TestRuledFirstRowIsTheHeader(t *testing.T) {
	span := tableFixture(t, "ruled-table.json")
	layout := tablelayout.NewLayout(tablelayout.NewTable(rec.FetchMap(span, "grid"), nil))
	if rows := layout.HeaderRows(); !slices.Equal(rows, []int{0}) {
		t.Fatalf("header rows %v", rows)
	}
	page := (&Paper{document: rec.Map{}}).richTableSpan(span)
	if strings.Count(page, `<th scope="col" `) != 2 || !strings.Contains(page, `</th>`) {
		t.Fatalf("header cells: %d", strings.Count(page, "<th "))
	}
}

// A rule after every row marks no header by itself; the words over numbers do (decision 167 amendment 3).
func TestFullyRuledGridHasNoRuleHeader(t *testing.T) {
	layout := tablelayout.NewLayout(tablelayout.NewTable(rec.FetchMap(tableFixture(t, "grid-table.json"), "grid"), nil))
	if rows := layout.HeaderRows(); !slices.Equal(rows, []int{0}) || layout.HeaderSource() != tablelayout.HeaderNumeric {
		t.Fatalf("a rule after every row marks no header, numbers under words do: %v %q", rows, layout.HeaderSource())
	}
}

func TestHeaderRowsNeedText(t *testing.T) {
	span := tableFixture(t, "ruled-table.json")
	grid := rec.FetchMap(span, "grid")
	for _, value := range rec.FetchList(grid, "cells") {
		if cell := rec.AsMap(value); rec.FetchInt(cell, "row") == 0 {
			cell["content"] = []any{rec.Map{"type": "image", "asset": "a"}}
		}
	}
	if rows := tablelayout.NewLayout(tablelayout.NewTable(grid, nil)).HeaderRows(); len(rows) != 0 {
		t.Fatalf("a row of images is no header: %v", rows)
	}
}
