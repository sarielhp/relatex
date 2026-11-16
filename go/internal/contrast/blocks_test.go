package contrast

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// White on a beamer block's coloured title reads in the PDF (decision 083, amendment 1): it is
// adjusted for the page's theorem background, not hidden. White outside a block still is hidden.
func TestBlockInksAreNotHidden(t *testing.T) {
	span := func(color string) any { return rec.Map{"type": "text", "text": "x", "color": color} }
	document := rec.Map{"nodes": rec.Map{
		"b":     rec.Map{"type": "theorem", "children": []any{"title"}, "semantics": rec.Map{"kind": "theorem", "beamer_block": "block"}},
		"title": rec.Map{"type": "paragraph", "children": []any{}, "content": []any{span("#ffffff")}},
		"p":     rec.Map{"type": "paragraph", "children": []any{}, "content": []any{span("#fdfdfd")}},
	}}
	hidden := Hidden(document)
	if hidden["#ffffff"] || !hidden["#fdfdfd"] {
		t.Fatalf("hidden = %v", hidden)
	}
	adjusted := map[string]bool{}
	for _, value := range rec.FetchList(Adjustments(document), "items") {
		adjusted[rec.FetchString(rec.AsMap(value), "ink")] = true
	}
	if !adjusted["#ffffff"] || adjusted["#fdfdfd"] {
		t.Fatalf("adjusted = %v", adjusted)
	}
}
