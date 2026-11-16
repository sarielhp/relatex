package tablehints

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
	"github.com/sarielhp/relatex/internal/tablelayout/tablefixture"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// figureDocument is a figure float with a picture and two tabulars (the second inside the first's
// paragraph), labelled fig:wspd, and a table float with the same caption number.
func figureDocument() rec.Map {
	doc := document()
	nodes := rec.FetchMap(doc, "nodes")
	nodes["root"].(rec.Map)["children"] = []any{"f1", "f2", "g1"}
	nodes["g1"] = rec.Map{"id": "g1", "type": "figure_float", "children": []any{"g1c", "g1p"}}
	nodes["g1c"] = rec.Map{"id": "g1c", "type": "float_caption", "children": []any{"g1cp"}}
	nodes["g1cp"] = rec.Map{"id": "g1cp", "type": "paragraph", "children": []any{},
		"content": []any{rec.Map{"type": "text", "text": "Figure 2: A red and a green set."}}}
	nodes["g1p"] = rec.Map{"id": "g1p", "type": "paragraph", "children": []any{}, "content": []any{
		rec.Map{"type": "image", "id": "g1i"},
		tablefixture.Span("g1t1", tablefixture.Grid(plainRows(), nil)),
		tablefixture.Span("g1t2", tablefixture.Grid(plainRows(), nil))}}
	rec.FetchMap(doc, "labels")["fig:wspd"] = rec.Map{"target": "g1p", "number": "2", "status": "verified"}
	return doc
}

func figureGrid(doc rec.Map, position int) rec.Map {
	paragraph := rec.FetchMap(rec.FetchMap(doc, "nodes"), "g1p")
	return rec.FetchMap(rec.AsMap(rec.FetchList(paragraph, "content")[position]), "grid")
}

// A key with a position (label, or caption number, then #N) reaches that table of a figure float and
// only that one; a plain figure label, a position past the float's tables and a position of a float
// that does not exist name no table. A table float keeps its number: "2" is the Table.
func TestApplyReachesTablesInFigureFloats(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	doc := figureDocument()
	unmatched := Apply(doc, hints(t, "%% relatex tables:\n%%   fig:wspd#1 = \"header_rows=1\"\n%%   fig:wspd#2 = \"header_columns=1\"\n"+
		"%%   fig:wspd#3 = \"header_rows=1\"\n%%   fig:wspd = \"header_rows=1\"\n%%   fig:none#1 = \"header_rows=1\"\n%%   2 = \"header_rows=0\"\n"))
	if !slices.Equal(unmatched, []string{"fig:none#1", "fig:wspd", "fig:wspd#3"}) {
		t.Fatalf("unmatched %v", unmatched)
	}
	first, second := figureGrid(doc, 1), figureGrid(doc, 2)
	if !rec.Equal(first[tablelayout.AuthorHeaderRows], []any{int64(0)}) || first[tablelayout.AuthorHeaderColumns] != nil {
		t.Fatalf("first table keys %v %v", first[tablelayout.AuthorHeaderRows], first[tablelayout.AuthorHeaderColumns])
	}
	if second[tablelayout.AuthorHeaderRows] != nil || !rec.Equal(second[tablelayout.AuthorHeaderColumns], []any{int64(0)}) {
		t.Fatalf("second table keys %v %v", second[tablelayout.AuthorHeaderRows], second[tablelayout.AuthorHeaderColumns])
	}
	if layout := tablelayout.NewLayout(tablelayout.NewTable(first, nil)); layout.HeaderSource() != tablelayout.HeaderAuthor {
		t.Fatalf("hinted source %q", layout.HeaderSource())
	}
	// "2" is the table float's own number (Table 2): the figure's number needs a position.
	if !rec.Equal(grid(doc, "f1")[tablelayout.AuthorHeaderRows], []any{}) {
		t.Fatalf("table float keys %v", grid(doc, "f1")[tablelayout.AuthorHeaderRows])
	}
}

// The figure's caption number with a position names its table, a key naming a table float takes a
// position too, and the key splits at the last #.
func TestFigureNumberAndPositionKeys(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	doc := figureDocument()
	nodes := rec.FetchMap(doc, "nodes")
	nodes["g1"].(rec.Map)["children"] = []any{"g1c", "g1p"}
	labels := rec.FetchMap(doc, "labels")
	delete(labels, "tab:results")
	labels["fig:wspd"] = rec.Map{"target": "g1p", "number": "7", "status": "verified"}
	nodes["g1cp"].(rec.Map)["content"] = []any{rec.Map{"type": "text", "text": "Figure 7: A red and a green set."}}
	if unmatched := Apply(doc, hints(t, "%% relatex tables:\n%%   7#2 = \"header_rows=1\"\n%%   IV#1 = \"header_columns=1\"\n")); len(unmatched) != 0 {
		t.Fatalf("unmatched %v", unmatched)
	}
	if !rec.Equal(figureGrid(doc, 2)[tablelayout.AuthorHeaderRows], []any{int64(0)}) || figureGrid(doc, 1)[tablelayout.AuthorHeaderRows] != nil {
		t.Fatalf("figure keys %v %v", figureGrid(doc, 1)[tablelayout.AuthorHeaderRows], figureGrid(doc, 2)[tablelayout.AuthorHeaderRows])
	}
	if !rec.Equal(grid(doc, "f2")[tablelayout.AuthorHeaderColumns], []any{int64(0)}) {
		t.Fatalf("table float keys %v", grid(doc, "f2")[tablelayout.AuthorHeaderColumns])
	}
	for key, want := range map[string]int{"a#1": 1, "a": 0, "a#0": 0, "a#x": 0, "a#2#3": 3} {
		if base, at := Position(key); at != want || (want > 0 && base != key[:len(key)-2]) {
			t.Errorf("%q: %q %d", key, base, at)
		}
	}
}
