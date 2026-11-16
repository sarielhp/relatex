package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Decision 127: a list and a text-marked item show no list style; an item whose marker a beamer
// theme draws shows its shape, and the check rejects a page that shows another.
func TestListStylesOfDrawnMarkers(t *testing.T) {
	structures := []rec.Map{{"id": "l", "type": "unordered_list"}, {"id": "i", "type": "list_item"}}
	nodes := rec.Map{"l": rec.Map{"id": "l", "type": "unordered_list"},
		"i": rec.Map{"id": "i", "type": "list_item", "marker": rec.Map{"shape": "disc"}}}
	good := rec.Map{"lists": []any{
		rec.Map{"id": "l", "type": "unordered_list", "tag": "ul", "list_style": "none"},
		rec.Map{"id": "i", "type": "list_item", "tag": "li", "list_style": "disc"}}}
	validateLists(good, structures, nodes)
	bad := rec.Map{"lists": []any{
		rec.Map{"id": "l", "type": "unordered_list", "tag": "ul", "list_style": "none"},
		rec.Map{"id": "i", "type": "list_item", "tag": "li", "list_style": "none"}}}
	defer func() {
		if recover() == nil {
			t.Fatal("a drawn marker shown as none passed")
		}
	}()
	validateLists(bad, structures, nodes)
}
