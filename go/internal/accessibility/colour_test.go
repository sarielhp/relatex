package accessibility

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A float caption naming colours is a 1.4.1 item; a paragraph naming one is not.
func TestColourItemsAreCaptionsNamingColours(t *testing.T) {
	text := func(id, value string) rec.Map {
		return rec.Map{"id": id, "type": "paragraph", "children": []any{}, "content": []any{rec.Map{"type": "text", "text": value}}}
	}
	document := rec.Map{
		"block_order": []any{"p1", "c1", "c2"},
		"nodes": rec.Map{
			"root": rec.Map{"id": "root", "type": "document", "children": []any{"p1", "f1"}},
			"f1":   rec.Map{"id": "f1", "type": "figure_float", "children": []any{"k1"}},
			"k1":   rec.Map{"id": "k1", "type": "float_caption", "children": []any{"c1", "c2"}},
			"p1":   text("p1", "The red line in the text."),
			"c1":   text("c1", "Figure 1: the bound (red) and the samples (blue dots)."),
			"c2":   text("c2", "A red-black tree."),
		},
	}
	items, _ := colourItems(document, newWalk(document, nil))
	caption := "Figure 1: the bound (red) and the samples (blue dots)."
	want := []any{rec.Map{"block": "c1", "colours": []any{"red", "blue"}, "text": caption, "caption": caption,
		"float": "figure_float", "number": "1", "sha256": CaptionSHA(caption + "\nA red-black tree.")}}
	if !rec.Equal(items, want) {
		t.Errorf("colour items = %v", items)
	}
}
