package html

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// tableNamePaper is a paper whose table caption "cap" holds one paragraph, with a 2x2
// table_grid span for every given id (a data table, not a presumed layout).
func tableNamePaper(tableIDs ...string) *Paper {
	tablemodel.Root = filepath.Join("..", "..", "..") // the schemas the grid is validated by
	nodes := rec.Map{
		"cap": rec.Map{"id": "cap", "type": "float_caption", "children": []any{"ct"}},
		"ct":  rec.Map{"id": "ct", "type": "paragraph", "children": []any{}, "content": []any{rec.Map{"type": "text", "text": "Table 1: A table."}}},
	}
	children := []any{"cap"}
	for _, id := range tableIDs {
		nodes[id] = rec.Map{"id": id, "type": "paragraph", "children": []any{},
			"content": []any{rec.Map{"type": "table_grid", "id": id, "grid": testGrid(2, 2, nil, nil)}}}
		children = append(children, id)
	}
	nodes["f"] = rec.Map{"id": "f", "type": "table_float", "children": children}
	return &Paper{document: rec.Map{}, nodes: nodes}
}

// TestTableInACaptionedFloatIsNamedByItsCaption: table navigation names the table by the float's
// caption, whose figcaption still names the figure (a11y-804), top or bottom caption.
func TestTableInACaptionedFloatIsNamedByItsCaption(t *testing.T) {
	for _, order := range [][]any{{"cap", "t1"}, {"t1", "cap"}} {
		paper := tableNamePaper("t1")
		rec.AsMap(paper.nodes["f"])["children"] = order
		page := paper.RenderNode("f")
		for _, want := range []string{`<figure id="f" data-structure="table_float" class="table_float" style="margin:0" role="group">`,
			`<table id="t1" aria-labelledby="cap" data-kind="table_grid"`} {
			if !strings.Contains(page, want) {
				t.Errorf("%v: missing %q in %s", order, want, page)
			}
		}
	}
}

// TestTablesAreUnnamedWithoutOneCaptionAndTable: two top-level tables in one float (subtables,
// each with its own caption) get no shared name; a table outside a float gets none either.
func TestTablesAreUnnamedWithoutOneCaptionAndTable(t *testing.T) {
	two := tableNamePaper("t1", "t2").RenderNode("f")
	if strings.Contains(two, `aria-labelledby="cap" data-kind`) || strings.Count(two, "aria-labelledby") != 0 {
		t.Errorf("two tables share the float's caption: %s", two)
	}
	outside := tableNamePaper("t1").RenderNode("t1")
	if strings.Contains(outside, "aria-labelledby") {
		t.Errorf("a table outside a float is named: %s", outside)
	}
}

// TestANestedFloatNamesItsOwnTable: an outer table_float does not reach into a float it holds,
// so a table never gets two names (the browser check reads them from an unordered map).
func TestANestedFloatNamesItsOwnTable(t *testing.T) {
	paper := tableNamePaper("t1")
	paper.nodes["cap2"] = rec.Map{"id": "cap2", "type": "float_caption", "children": []any{}}
	paper.nodes["inner"] = rec.Map{"id": "inner", "type": "table_float", "children": []any{"cap2", "t1"}}
	rec.AsMap(paper.nodes["f"])["children"] = []any{"cap", "inner"}
	if names := htmlkit.TableNames(paper.nodes); len(names) != 1 || names["t1"] != "cap2" {
		t.Errorf("a nested float's table is named %v, want t1 by cap2", names)
	}
}
