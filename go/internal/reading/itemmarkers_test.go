package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Decision 127: a rule marked as a drawn item marker opens its item's block as the block's
// item_marker, not as a rule span, and the item's first words lose the space after it.
func TestItemMarkerOpensItsBlock(t *testing.T) {
	block := rec.Map{"content": []any{}}
	item := rec.Map{"type": "unsupported", "item_marker": "square"}
	if !itemMarkerTaken(block, item, []any{int64(300), int64(300), int64(0), "#3333b3"}) {
		t.Fatal("marked rule not taken")
	}
	marker := rec.AsMap(block["item_marker"])
	if marker["shape"] != "square" || marker["color"] != "#3333b3" {
		t.Fatalf("marker = %v", marker)
	}
	words := dropMarkerSpace(block, rec.Map{"type": "text", "text": "One", "space": " "})
	if words["space"] != "" {
		t.Fatalf("space kept: %v", words)
	}
	if itemMarkerTaken(block, item, []any{int64(300), int64(300), int64(0)}) {
		t.Fatal("a second marked rule in one block is no marker")
	}
}

func TestUnmarkedOrLateRulesStayRules(t *testing.T) {
	if itemMarkerTaken(rec.Map{"content": []any{}}, rec.Map{"type": "unsupported"}, []any{int64(1), int64(1), int64(0)}) {
		t.Fatal("unmarked rule taken")
	}
	begun := rec.Map{"content": []any{rec.Map{"type": "text", "text": "x"}}}
	if itemMarkerTaken(begun, rec.Map{"item_marker": "disc"}, []any{int64(1), int64(1), int64(0)}) {
		t.Fatal("a marked rule after text opens no item")
	}
	plain := rec.Map{"content": []any{}}
	words := rec.Map{"type": "text", "text": "x", "space": " "}
	if dropMarkerSpace(plain, words)["space"] != " " {
		t.Fatal("space dropped without a marker")
	}
}
