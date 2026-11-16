package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func rejectsMarker(kind string, marker any) (failed bool) {
	defer func() { failed = recover() != nil }()
	validateItemMarker(kind, marker)
	return false
}

// Decision 127: only a list item has a marker, a known shape in an optional colour.
func TestItemMarkerValidation(t *testing.T) {
	validateItemMarker("list_item", rec.Map{"shape": "square", "color": "#3333b3"})
	validateItemMarker("list_item", rec.Map{"shape": "disc"})
	cases := []struct {
		kind   string
		marker any
	}{
		{"paragraph", rec.Map{"shape": "disc"}},
		{"list_item", rec.Map{"shape": "star"}},
		{"list_item", rec.Map{"shape": "disc", "color": "red"}},
		{"list_item", rec.Map{"shape": "disc", "size": int64(3)}},
		{"list_item", "disc"},
	}
	for _, c := range cases {
		if !rejectsMarker(c.kind, c.marker) {
			t.Errorf("accepted %s %v", c.kind, c.marker)
		}
	}
}

// A list item takes the item_marker of its first block.
func TestListItemTakesItsFirstBlocksMarker(t *testing.T) {
	marker := rec.Map{"shape": "disc"}
	b := &builder{reading: rec.Map{"blocks": []any{rec.Map{"id": "b1", "item_marker": marker}, rec.Map{"id": "b2"}}, "notes": rec.Map{"notes": []any{}}}}
	fields := b.withItemMarker(rec.Map{}, rec.Map{"content": []any{rec.Map{"block": "b1"}}})
	if !rec.Equal(fields["marker"], marker) {
		t.Fatalf("fields = %v", fields)
	}
	if _, has := b.withItemMarker(rec.Map{}, rec.Map{"content": []any{rec.Map{"block": "b2"}}})["marker"]; has {
		t.Fatal("unmarked block gave a marker")
	}
}
