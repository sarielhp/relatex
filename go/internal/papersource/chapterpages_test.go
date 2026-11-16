package papersource

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestChapterPagesAreThoseOfThePartAndChapterHeadings(t *testing.T) {
	reading := rec.Map{"blocks": []any{
		rec.Map{"id": "b1", "pages": []any{int64(1)}}, rec.Map{"id": "b2", "pages": []any{int64(4), int64(5)}},
		rec.Map{"id": "b3", "pages": []any{int64(9)}}, rec.Map{"id": "b4", "pages": []any{}}, rec.Map{"id": "b5", "pages": []any{int64(12)}},
	}}
	heading := func(key, counter string) rec.Map {
		return rec.Map{"type": "heading", "semantics": rec.Map{"counter": counter}, "provenance": rec.Map{"inspection_key": key}}
	}
	document := rec.Map{"main_block_order": []any{"n1", "n2", "n3", "n4", "n5"}, "nodes": rec.Map{
		"n1": rec.Map{"type": "paragraph", "provenance": rec.Map{"inspection_key": "b1"}},
		"n2": heading("b2", "part"), "n3": heading("b3", "section"), "n4": heading("b4", "chapter"), "n5": heading("b5", "chapter"),
	}}
	source := &Source{blockPages: firstPages(reading)}
	got := source.ChapterPages(document)
	want := rec.Map{"schema_version": ChapterPagesVersion, "headings": []any{
		rec.Map{"node": "n2", "counter": "part", "page": int64(4)}, rec.Map{"node": "n5", "counter": "chapter", "page": int64(12)}}}
	if !rec.Equal(got, want) {
		t.Fatalf("chapter pages %v, want %v", got, want)
	}
}
