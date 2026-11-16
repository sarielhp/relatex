package tablehints

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/authorhints"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
	"github.com/sarielhp/relatex/internal/tablelayout/tablefixture"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

func plainRows() [][]tablefixture.Cell {
	rows := [][]tablefixture.Cell{}
	for _, text := range []string{"a", "b", "c"} {
		rows = append(rows, []tablefixture.Cell{{Text: text}, {Text: "1"}, {Text: "2"}})
	}
	return rows
}

// document is two table floats, one labelled, each with a caption and a grid of plain text.
func document() rec.Map {
	nodes := rec.Map{"root": rec.Map{"id": "root", "type": "section", "children": []any{"f1", "f2"}}}
	for _, float := range []struct{ id, caption string }{{"f1", "Table 2: Results"}, {"f2", "TABLE IV Costs"}} {
		nodes[float.id] = rec.Map{"id": float.id, "type": "table_float", "children": []any{float.id + "c", float.id + "p"}}
		nodes[float.id+"c"] = rec.Map{"id": float.id + "c", "type": "float_caption", "children": []any{float.id + "cp"}}
		nodes[float.id+"cp"] = rec.Map{"id": float.id + "cp", "type": "paragraph", "children": []any{},
			"content": []any{rec.Map{"type": "text", "text": float.caption}}}
		nodes[float.id+"p"] = rec.Map{"id": float.id + "p", "type": "paragraph", "children": []any{},
			"content": []any{tablefixture.Span(float.id+"t", tablefixture.Grid(plainRows(), nil))}}
	}
	return rec.Map{"root": "root", "nodes": nodes,
		"labels": rec.Map{"tab:results": rec.Map{"target": "f1", "number": "2", "status": "verified"},
			"fig:x": rec.Map{"target": "root", "number": "1", "status": "verified"}}}
}

func grid(document rec.Map, float string) rec.Map {
	paragraph := rec.FetchMap(rec.FetchMap(document, "nodes"), float+"p")
	return rec.FetchMap(rec.AsMap(rec.FetchList(paragraph, "content")[0]), "grid")
}

func hints(t *testing.T, comment string) *authorhints.Hints {
	t.Helper()
	parsed, err := authorhints.Parse([]authorhints.Source{{Path: "main.tex", Text: comment}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// A hint keyed by a label or by the number a caption prints reaches the float's grid, clipped to
// the table; a key naming no table is returned and changes nothing.
func TestApplyByLabelAndNumber(t *testing.T) {
	tablemodel.Root = filepath.Join("..", "..", "..")
	doc := document()
	unmatched := Apply(doc, hints(t, "%% relatex tables:\n%%   tab:results = \"header_rows=1 header_columns=5\"\n"+
		"%%   IV = \"header_rows=0\"\n%%   fig:x = \"header_rows=1\"\n%%   7 = \"header_rows=1\"\n"))
	if !slices.Equal(unmatched, []string{"7", "fig:x"}) {
		t.Fatalf("unmatched %v", unmatched)
	}
	first, second := grid(doc, "f1"), grid(doc, "f2")
	if !rec.Equal(first[tablelayout.AuthorHeaderRows], []any{int64(0)}) ||
		!rec.Equal(first[tablelayout.AuthorHeaderColumns], []any{int64(0), int64(1), int64(2)}) {
		t.Fatalf("f1 grid keys %v %v", first[tablelayout.AuthorHeaderRows], first[tablelayout.AuthorHeaderColumns])
	}
	if !rec.Equal(second[tablelayout.AuthorHeaderRows], []any{}) || second[tablelayout.AuthorHeaderColumns] != nil {
		t.Fatalf("f2 grid keys %v %v", second[tablelayout.AuthorHeaderRows], second[tablelayout.AuthorHeaderColumns])
	}
	layout := tablelayout.NewLayout(tablelayout.NewTable(first, nil))
	if !slices.Equal(layout.HeaderRows(), []int{0}) || layout.HeaderSource() != tablelayout.HeaderAuthor {
		t.Fatalf("hinted rows %v %q", layout.HeaderRows(), layout.HeaderSource())
	}
}

// The number from a caption without a label, and nothing for a caption that prints none.
func TestCaptionNumbers(t *testing.T) {
	for text, want := range map[string]string{"Table 3: x": "3", "Tab. A.1. y": "A.1", "TABLE IV Z": "IV", "Table S2 z": "S2",
		"Tabelle 4.2 x": "4.2", "Results of the run": "", "Table: none": ""} {
		nodes := rec.Map{"f": rec.Map{"type": "table_float", "children": []any{"c"}},
			"c": rec.Map{"type": "float_caption", "children": []any{}, "content": []any{rec.Map{"type": "text", "text": text}}}}
		if got := captionNumber(nodes, "f"); got != want {
			t.Errorf("%q: %q, want %q", text, got, want)
		}
	}
}

func TestNoHints(t *testing.T) {
	doc := document()
	if unmatched := Apply(doc, nil); unmatched != nil {
		t.Fatal(unmatched)
	}
	if _, ok := grid(doc, "f1")[tablelayout.AuthorHeaderRows]; ok {
		t.Fatal("a grid without a hint has no author key")
	}
}
