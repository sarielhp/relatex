package layout

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// glyphsFor is PDFLayout#glyphs_for: a page's structured-text characters, line by line.
func glyphsFor(page *nativetrace.Node, number int64) []rec.Map {
	records := []rec.Map{}
	for lineIndex, line := range page.Descendants("line") {
		for _, char := range line.Descendants("char") {
			font := char.Parent
			quad := nativetrace.Floats(attribute(char, "quad"))
			if len(quad) != 8 || !finite(quad) {
				rec.Fail("Invalid PDF glyph quad")
			}
			text := attribute(char, "c")
			x, y := nativetrace.Float(attribute(char, "x")), nativetrace.Float(attribute(char, "y"))
			name, named := structuredFontName(font)
			size, sized := font.Attr("size")
			if !sized {
				rec.Fail("can't convert nil into Float")
			}
			color, colored := char.Attr("color")
			records = append(records, rec.Map{"id": fmt.Sprintf("p%d-g%d", number, len(records)+1), "line": int64(lineIndex + 1),
				"text": text, "metric_quad": floatList(quad), "baseline": []any{x, y}, "font": optional(name, named),
				"size": nativetrace.Float(size), "color": optional(color, colored)})
		}
	}
	return records
}

// structuredFontName is a structured-text font's name. MuPDF writes a name holding bytes outside printable
// ASCII as hexname, its bytes in hexadecimal (a subset font named "MUFUZY弫ArialMT", papers2/0385);
// the paint trace writes the same bytes as the font attribute, so the decoded name binds the two.
func structuredFontName(font *nativetrace.Node) (string, bool) {
	if name, named := font.Attr("name"); named {
		return name, true
	}
	encoded, ok := font.Attr("hexname")
	if !ok {
		return "", false
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		rec.Fail("Invalid structured-text font hexname")
	}
	return string(decoded), true
}

// optional is element[name]: the value, or nil without the attribute.
func optional(value string, present bool) any {
	if !present {
		return nil
	}
	return value
}

func finite(values []float64) bool {
	for _, value := range values {
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return false
		}
	}
	return true
}

func floatList(values []float64) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

// hasSpace is text.match?(/\s/): an ASCII whitespace character anywhere.
func hasSpace(text string) bool { return strings.ContainsAny(text, " \t\n\v\f\r") }

// wordsFor is PDFLayout#words_for: runs of glyphs on one line between whitespace.
func wordsFor(glyphs []rec.Map, page int64) []any {
	words := []any{}
	current := []rec.Map{}
	for _, glyph := range glyphs {
		text := rec.FetchString(glyph, "text")
		if hasSpace(text) || (len(current) > 0 && !rec.Equal(rec.Fetch(current[len(current)-1], "line"), rec.Fetch(glyph, "line"))) {
			if len(current) > 0 {
				words = append(words, wordData(current, page, len(words)+1))
			}
			current = []rec.Map{}
		}
		if !hasSpace(text) {
			current = append(current, glyph)
		}
	}
	if len(current) > 0 {
		words = append(words, wordData(current, page, len(words)+1))
	}
	return words
}

func wordData(glyphs []rec.Map, page int64, index int) rec.Map {
	offset := int64(0)
	mapping := make([]any, len(glyphs))
	var text strings.Builder
	xs, ys := []float64{}, []float64{}
	for position, glyph := range glyphs {
		glyphText := rec.FetchString(glyph, "text")
		length := int64(utf8.RuneCountInString(norm.NFKC.String(glyphText)))
		mapping[position] = rec.Map{"glyph": rec.Fetch(glyph, "id"), "characters": []any{offset, offset + length}}
		offset += length
		text.WriteString(glyphText)
		quad := rec.FetchList(glyph, "ink_quad")
		for corner := 0; corner < 8; corner += 2 {
			x, _ := rec.Number(quad[corner])
			y, _ := rec.Number(quad[corner+1])
			xs, ys = append(xs, x), append(ys, y)
		}
	}
	joined := text.String()
	first := glyphs[0]
	return rec.Map{"id": fmt.Sprintf("p%d-w%d", page, index), "page": page, "line": rec.Fetch(first, "line"), "text": joined,
		"normalized_text": norm.NFKC.String(joined),
		"boxes":           []any{[]any{geometry.Minimum(xs), geometry.Minimum(ys), geometry.Maximum(xs), geometry.Maximum(ys)}},
		"box_kind":        "ink", "baseline": rec.Fetch(first, "baseline"), "characters_to_glyphs": mapping}
}
