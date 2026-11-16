package html

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A glyph with no character but a name shows U+FFFD and is an image named by the glyph's words.
func TestRenderSpanNamesAnUnmappedGlyph(t *testing.T) {
	paper := &Paper{document: rec.Map{}, nodes: rec.Map{}}
	got := paper.renderSpan(rec.Map{"type": "text", "text": "�", "glyph_name": "WashCotton"})
	want := `<span role="img" aria-label="Wash cotton" data-glyph-name="WashCotton" data-kind="text">` + "�</span>"
	if got != want {
		t.Errorf("named glyph = %s", got)
	}
}
