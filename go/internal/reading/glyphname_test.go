package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A named glyph without a character (marvosym's WashCotton) is a span of its own with its name;
// the space before it and the text after it stay ordinary spans.
func TestNamedUnmappedGlyphIsItsOwnSpan(t *testing.T) {
	e := &Evidence{origins: map[uintptr]rec.Map{}}
	block := rec.Map{"id": "b1", "content": []any{}}
	paint := spanPaint{}
	e.appendItemText(block, rec.Map{"type": "text", "text": "Wash", "space": ""}, paint, nil)
	e.appendItemText(block, rec.Map{"type": "text", "text": "[unmapped]", "space": " ", "glyph_name": "WashCotton"}, paint, nil)
	e.appendItemText(block, rec.Map{"type": "text", "text": "[unmapped]", "space": "", "glyph_name": "Tumbler"}, paint, nil)
	e.appendItemText(block, rec.Map{"type": "text", "text": "x", "space": " "}, paint, nil)
	want := []any{
		rec.Map{"type": "text", "text": "Wash "},
		rec.Map{"type": "text", "text": "[unmapped]", "glyph_name": "WashCotton"},
		rec.Map{"type": "text", "text": "[unmapped]", "glyph_name": "Tumbler"},
		rec.Map{"type": "text", "text": " x"},
	}
	if got := rec.FetchList(block, "content"); !rec.Equal(got, want) {
		t.Errorf("spans = %v, want %v", got, want)
	}
}
