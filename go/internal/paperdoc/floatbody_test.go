package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A float whose body is not observed (a lost \includegraphics, display math in a table) keeps
// its caption instead of stopping the export.
func TestValidateFloatsAcceptsCaptionOnlyFloat(t *testing.T) {
	nodes := rec.Map{
		"f": rec.Map{"id": "f", "type": "figure_float", "children": []any{"c"}},
		"c": rec.Map{"id": "c", "type": "float_caption", "children": []any{}},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("caption-only float rejected: %v", r)
		}
	}()
	validateFloats(nodes, []string{"f"})
}

// A theorem set in a float (an ntheorem "algorithm" theorem in a figure; book 0003) is a structure
// node with children and no content field: it is the float's body, not missing evidence.
func TestValidateFloatsAcceptsTheoremBody(t *testing.T) {
	nodes := rec.Map{
		"f": rec.Map{"id": "f", "type": "figure_float", "children": []any{"t", "c"}},
		"t": rec.Map{"id": "t", "type": "theorem", "children": []any{"p"}},
		"p": rec.Map{"id": "p", "type": "paragraph", "children": []any{},
			"content": []any{rec.Map{"type": "text", "text": "x"}}},
		"c": rec.Map{"id": "c", "type": "float_caption", "children": []any{}},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("theorem float body rejected: %v", r)
		}
	}()
	validateFloats(nodes, []string{"f"})
}

// A display inside a float (a TikZ picture set in \[ \] in a subfigure; third cohort 817) is
// part of its body; a heading is not.
func TestValidateFloatsAcceptsDisplayInBody(t *testing.T) {
	nodes := func(kind string) rec.Map {
		return rec.Map{
			"f": rec.Map{"id": "f", "type": "figure_float", "children": []any{"d", "p", "c"}},
			"d": rec.Map{"id": "d", "type": kind, "children": []any{}, "content": []any{}},
			"p": rec.Map{"id": "p", "type": "paragraph", "children": []any{},
				"content": []any{rec.Map{"type": "text", "text": "x"}}},
			"c": rec.Map{"id": "c", "type": "float_caption", "children": []any{}},
		}
	}
	validateFloats(nodes("display_math"), []string{"f"})
	defer func() {
		if recover() == nil {
			t.Fatal("heading accepted in a float body")
		}
	}()
	validateFloats(nodes("heading"), []string{"f"})
}

// A float whose only body paragraph is empty (its picture dropped by the reading: slides 0019's
// plot) lost its body; it is not a float of another category, and the export goes on.
func TestValidateFloatsAcceptsEmptyParagraphBody(t *testing.T) {
	nodes := rec.Map{
		"f": rec.Map{"id": "f", "type": "figure_float", "children": []any{"p"}},
		"p": rec.Map{"id": "p", "type": "paragraph", "children": []any{}, "content": []any{}},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("float with an empty body paragraph rejected: %v", r)
		}
	}()
	validateFloats(nodes, []string{"f"})
	if retained(rec.AsMap(nodes["p"])) {
		t.Fatal("an empty paragraph counted as a retained body")
	}
}
