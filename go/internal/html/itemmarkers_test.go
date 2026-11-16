package html

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Decision 127: a drawn marker is the item's list style, painted in its colour by one ::marker rule.
func TestListStyleOfDrawnMarkers(t *testing.T) {
	if got := listStyle(rec.Map{"type": "unordered_list"}); got != ` style="list-style:none"` {
		t.Fatalf("list: %s", got)
	}
	item := rec.Map{"type": "list_item", "marker": rec.Map{"shape": "disc", "color": "#e32726"}}
	if got := listStyle(item); got != ` style="list-style-type:disc" data-marker-color="#e32726"` {
		t.Fatalf("item: %s", got)
	}
	document := rec.Map{"nodes": rec.Map{"a": item, "b": rec.Map{"marker": rec.Map{"shape": "square", "color": "#e32726"}}}}
	if got := itemMarkerCSS(document); got != `li[data-marker-color="#e32726"]::marker{color:#e32726}` {
		t.Fatalf("css: %s", got)
	}
}
