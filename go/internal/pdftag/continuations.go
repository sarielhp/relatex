package pdftag

import (
	"encoding/hex"
	"strings"
	"unicode/utf16"

	"github.com/sarielhp/relatex/internal/pdffonts"
)

// A glyph's ToUnicode value can be several characters (an ff or fi ligature). MuPDF's paint trace
// lists the extra characters after the glyph, zero-width, but leaves them out of text shown in a
// clip rendering mode (ocgx2's coloured links are a clip text filled through the layer: the
// contents page and the bibliography of books/0006): the layout then spells the ligature "f" and
// a reader of the tagged PDF, which uses the font's ToUnicode, reads "ff". The page's painted
// glyphs are completed from the font's ToUnicode before the order check reads them, so the check
// reads the text the file gives a reader; a glyph is completed only when the layout's text is
// the start of the font's, so no other disagreement is hidden.

// completeContinuations gives the painted glyphs of a prepared page the characters of their
// font's ToUnicode value that the paint trace dropped.
func (t *tagger) completeContinuations(page *prepared) {
	if page.failure != "" || page.mapper == nil {
		return
	}
	painted := t.painted[page.number]
	for _, entry := range page.items {
		if entry.kind != kindShow || entry.font == "" || entry.actual.sequence != 0 {
			continue
		}
		values := t.toUnicode(entry.font)
		ordinal := entry.first
		for _, shown := range entry.elements {
			for _, code := range shown.codes {
				t.completeGlyph(painted, page.mapper.mapping, ordinal, code, values)
				ordinal++
			}
		}
	}
}

// completeGlyph completes the painted glyph of one shown code when the font's value starts with
// the layout's text.
func (t *tagger) completeGlyph(painted []paintedGlyph, mapping []int, ordinal int, code []byte, values map[int]string) {
	if len(code) != 2 || ordinal >= len(mapping) || mapping[ordinal] < 0 || mapping[ordinal] >= len(painted) {
		return
	}
	glyph := &painted[mapping[ordinal]]
	full := values[int(code[0])<<8|int(code[1])]
	if glyph.unknown || len(full) <= len(glyph.text) || glyph.text == "" || !strings.HasPrefix(full, glyph.text) {
		return
	}
	glyph.text = full
}

// toUnicode is a composite font's ToUnicode values by code, as text; nil for a font without a
// usable map. Each font is read once.
func (t *tagger) toUnicode(font string) map[int]string {
	if values, done := t.unicodes[font]; done {
		return values
	}
	if t.unicodes == nil {
		t.unicodes = map[string]map[int]string{}
	}
	t.unicodes[font] = nil
	reference, _ := t.file.dict(font)["/ToUnicode"].(string)
	if reference == "" {
		return nil
	}
	data, err := t.file.streamData(reference)
	if err != nil {
		return nil
	}
	mapped, width := pdffonts.ReadCMap(string(data))
	if width != 2 {
		return nil
	}
	values := make(map[int]string, len(mapped))
	for code, digits := range mapped {
		if text, ok := utf16Text(digits); ok {
			values[code] = text
		}
	}
	t.unicodes[font] = values
	return values
}

// utf16Text decodes a ToUnicode value's UTF-16BE hex digits.
func utf16Text(digits string) (string, bool) {
	raw, err := hex.DecodeString(digits)
	if err != nil || len(raw)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(raw)/2)
	for index := range units {
		units[index] = uint16(raw[2*index])<<8 | uint16(raw[2*index+1])
	}
	return string(utf16.Decode(units)), true
}
