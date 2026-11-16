package paperdoc

import (
	"regexp"

	"github.com/sarielhp/relatex/internal/rec"
)

// glyphNamePattern is a PostScript glyph name (marvosym's Tumbler, WashCotton).
var glyphNamePattern = regexp.MustCompile(`\A[A-Za-z][A-Za-z0-9._]{0,62}\z`)

// validateSpanGlyphName: a text span that holds one glyph with no Unicode character (U+FFFD) may
// carry the glyph's name from its symbol font, which the outputs speak in its place.
func validateSpanGlyphName(span rec.Map, kind any) {
	value, named := span["glyph_name"]
	if !named {
		return
	}
	name, isString := value.(string)
	if kind != "text" || !isString || !glyphNamePattern.MatchString(name) || span["text"] != "�" {
		rec.Fail("Invalid span glyph name")
	}
}
