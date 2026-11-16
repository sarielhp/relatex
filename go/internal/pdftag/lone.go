package pdftag

import (
	"bytes"
	"fmt"
	"math"
	"strconv"

	"github.com/sarielhp/relatex/internal/pdffonts"
)

// Glyphs written by themselves (decision 148): a glyph whose text is the document's character
// (/ActualText, actualtext.go), and a glyph of a font the tagged PDF embeds in place of a viewer's
// substitute whose width changed, which a viewer drew stretched to the old width and is now scaled
// to it (Tz). Each is its own TJ, so its marked content and its scaling cover it alone.

// lone is how one glyph is written alone.
type lone struct {
	actual  string // the document's character, "" to keep the font's
	stretch pdffonts.Stretch
	scaled  bool // stretch applies
}

// stretchOf is a one-byte code's stretch in a show operator, when it is to be scaled.
func stretchOf(stretched map[int]pdffonts.Stretch, entry item, code []byte) (pdffonts.Stretch, bool) {
	if len(code) != 1 || entry.text.size == 0 || entry.text.scale == 0 {
		return pdffonts.Stretch{}, false
	}
	s, ok := stretched[int(code[0])]
	return s, ok && s.Factor > 0
}

// stretchesGlyphs: the operator shows a code to be scaled.
func stretchesGlyphs(entry item, stretched map[int]pdffonts.Stretch) bool {
	if len(stretched) == 0 {
		return false
	}
	for _, part := range entry.elements {
		for _, code := range part.codes {
			if _, ok := stretchOf(stretched, entry, code); ok {
				return true
			}
		}
	}
	return false
}

// stretchKern is the TJ kern after a glyph shown at horizontal scaling Factor that ends it where
// the viewer's stretched glyph ended: (New - k)/1000·Tfs + Tc + Tw, scaled by Factor, equals the
// old Old/1000·Tfs + Tc + Tw (Tw for code 32 alone; the operator's own scaling multiplies both).
func stretchKern(entry item, code []byte, s pdffonts.Stretch) float64 {
	size, spacing := entry.text.size, entry.text.charSpace
	if code[0] == ' ' {
		spacing += entry.text.wordSpace
	}
	original := float64(s.Old*size)/1000 + spacing
	return s.New + float64(spacing*1000)/size - float64(original*1000)/float64(size*s.Factor)
}

// writeLone writes one glyph as its own TJ through show (which wraps a drawing font's glyph),
// inside its /ActualText and between the horizontal scalings that stretch it; a glyph not scaled
// keeps the kern that undoes its width change (moved).
func writeLone(out *bytes.Buffer, entry item, code []byte, glyph lone, moved map[int]float64, show func([]byte)) {
	if glyph.actual != "" {
		out.WriteString(actualTextBegin(glyph.actual))
	}
	kern := compensation(moved, code)
	if glyph.scaled {
		fmt.Fprintf(out, "%s Tz\n", decimal(float64(entry.text.scale*glyph.stretch.Factor)*100))
		kern = ""
		if k := stretchKern(entry, code, glyph.stretch); math.Abs(k) >= 1e-6 {
			kern = decimal(k) + " "
		}
	}
	show(fmt.Appendf(nil, "[%s %s] TJ", hexString(code), kern))
	if glyph.scaled {
		fmt.Fprintf(out, "%s Tz\n", decimal(entry.text.scale*100))
	}
	if glyph.actual != "" {
		out.WriteString("EMC\n")
	}
}

// decimal writes a number with at most six decimals.
func decimal(value float64) string {
	return strconv.FormatFloat(math.Round(value*1e6)/1e6, 'f', -1, 64)
}

// lone says whether a page's glyph is written alone, and how: its document character when it
// is in a text leaf and TeX spells it otherwise, its stretch when its font's width changed.
func (t *pageTags) lone(entry item, ordinal int, code []byte, key string) (lone, bool) {
	var glyph lone
	if t.spelled != nil && key != artifactKey && key != "" && !t.drawn[entry.font] && spelledRole(t.roleOf(key)) {
		glyph.actual = t.spelled(ordinal)
	}
	glyph.stretch, glyph.scaled = stretchOf(t.stretched[entry.font], entry, code)
	return glyph, glyph.actual != "" || glyph.scaled
}

// anyLone: the operator shows a glyph to be written alone.
func (e *emitter) anyLone(entry item) bool {
	ordinal := entry.first
	for _, part := range entry.elements {
		for _, code := range part.codes {
			if _, ok := e.tags.lone(entry, ordinal, code, e.tags.keyOf(entry.font, ordinal)); ok {
				return true
			}
			ordinal++
		}
	}
	return false
}
