package layout

import (
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// MuPDF writes a font name with bytes outside printable ASCII as hexname (papers2/0385): the
// glyph's font is the decoded name, as the paint trace spells it.
func TestHexFontNamesAreDecoded(t *testing.T) {
	quad := `quad="0 0 1 0 0 1 1 1" x="0" y="1"`
	page := nativetrace.Parse([]byte(`<document><page><block><line>` +
		`<font hexname="4D5546555A59E5BCAB417269616C4D54" size="4"><char c="a" ` + quad + `/></font>` +
		`<font name="CMR10" size="10"><char c="b" ` + quad + `/></font>` +
		`</line></block></page></document>`)).Descendants("page")[0]
	glyphs := glyphsFor(page, 1)
	if glyphs[0]["font"] != "MUFUZY弫ArialMT" || glyphs[1]["font"] != "CMR10" {
		t.Fatalf("fonts %v %v", glyphs[0]["font"], glyphs[1]["font"])
	}
}

func glyph(id, text string, line int64, quad ...float64) rec.Map {
	values := make([]any, len(quad))
	for index, value := range quad {
		values[index] = value
	}
	return rec.Map{"id": id, "text": text, "line": line, "ink_quad": values, "baseline": []any{quad[0], quad[1]}}
}

// The expectation is PDFLayout#words_for on the same glyphs: whitespace and line changes
// split words, an NFKC ligature counts its letters.
func TestWordsSplitAtSpacesAndLines(t *testing.T) {
	glyphs := []rec.Map{glyph("g1", "ﬁ", 1, 0, 0, 2, 0, 0, 3, 2, 3), glyph("g2", "x", 1, 2, 0, 3, 0, 2, 3, 3, 3),
		glyph("g3", " ", 1, 3, 0, 4, 0, 3, 3, 4, 3), glyph("g4", "y", 2, 0, 5, 1, 5, 0, 8, 1, 8)}
	words := wordsFor(glyphs, 1)
	got, err := canonjson.Marshal(words)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"baseline":[0.0,0.0],"box_kind":"ink","boxes":[[0.0,0.0,3.0,3.0]],"characters_to_glyphs":[{"characters":[0,2],"glyph":"g1"},` +
		`{"characters":[2,3],"glyph":"g2"}],"id":"p1-w1","line":1,"normalized_text":"fix","page":1,"text":"ﬁx"},` +
		`{"baseline":[0.0,5.0],"box_kind":"ink","boxes":[[0.0,5.0,1.0,8.0]],"characters_to_glyphs":[{"characters":[0,1],"glyph":"g4"}],` +
		`"id":"p1-w2","line":2,"normalized_text":"y","page":1,"text":"y"}]`
	if string(got) != want {
		t.Fatalf("got %s", got)
	}
}
