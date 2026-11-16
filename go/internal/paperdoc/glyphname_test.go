package paperdoc

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestSpanGlyphNameIsValidatedOnASingleReplacementCharacter(t *testing.T) {
	validateSpanGlyphName(rec.Map{"type": "text", "text": "�", "glyph_name": "WashCotton"}, "text")
	validateSpanGlyphName(rec.Map{"type": "text", "text": "plain"}, "text")
	for _, span := range []rec.Map{
		{"type": "text", "text": "a", "glyph_name": "WashCotton"},
		{"type": "text", "text": "�", "glyph_name": "Wash Cotton"},
		{"type": "reference", "text": "�", "glyph_name": "Tumbler"},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("accepted %v", span)
				}
			}()
			validateSpanGlyphName(span, span["type"])
		}()
	}
}

func TestUnmappedGlyphDiagnosticNamesTheGlyph(t *testing.T) {
	message, _ := degradation(rec.Map{"code": "unmapped_reading_glyph", "block": "b1", "name": "WashCotton"})
	if !strings.Contains(message, "WashCotton") || !strings.Contains(message, `"Wash cotton"`) {
		t.Errorf("message = %q", message)
	}
	if message, _ := degradation(rec.Map{"code": "unmapped_reading_glyph", "block": "b1"}); message != placedMessages["unmapped_reading_glyph"] {
		t.Errorf("unnamed message = %q", message)
	}
}
