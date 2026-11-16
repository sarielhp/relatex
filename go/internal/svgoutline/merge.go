package svgoutline

import (
	"math"

	"github.com/sarielhp/relatex/internal/rec"
)

// MuPDF 1.28.2's structured text drops a glyph it reads as a copy of a character it already has
// (source/fitz/stext-device.c, fz_add_stext_char_imp); the paint trace and the page SVG keep the
// copy, so its use has no text glyph to bind to. Two rules, both at a tenth of the copy's size
// (FAKE_BOLD_MAX_DIST, the size being the expansion of its text matrix):
//
//   - fake bold (check_for_fake_bold, on since mutool draw collects styles for structured text): a
//     glyph of the same character in the same font as any character already on the page, its
//     origin nearer on each axis. The kept character is flagged bold. A figure that draws its
//     panels several times under clips puts a label's copies a point apart (papers2/0254), and a
//     grey enlarged digit painted over a black one (papers2/0352) is swallowed the same way.
//   - repeated character (the older check, kept for compatibility): a glyph of the same character
//     as the last one added, on its line's direction, whose origin lies nearer to that one's, in
//     any font. A TeX-mathx "r" (a wide accent's piece) and the CMMI10 "r" set at its origin a
//     fifth of a point away (papers2/0387) are one character, unflagged.
//
// A merged copy is admitted only by these rules, and only off the kept use's placement: a copy at
// that very placement could itself have been bound, and repaint.go and overpaint.go decide which
// copy binds there (a later copy in another color shows, so the glyph binds to it). Its own ink is
// drawn by the page SVG; only the binding of text to ink lacks it.

// fakeBoldDistance is FAKE_BOLD_MAX_DIST.
const fakeBoldDistance = 0.1

// mergedCopies is the unbound uses MuPDF merged into a bound character.
func (v *validator) mergedCopies(unbound []rec.Map) map[int64]bool {
	byCharacter := map[string][]rec.Map{}
	for _, entry := range v.svg.Uses {
		if v.used[rec.FetchInt(entry, "use")] {
			key := characterKey(entry)
			byCharacter[key] = append(byCharacter[key], entry)
		}
	}
	continued := v.continuations()
	merged := map[int64]bool{}
	for _, entry := range unbound {
		use := rec.FetchInt(entry, "use")
		if fakeBold(entry, byCharacter[characterKey(entry)]) || fakeBoldOfContinuation(entry, continued[characterKey(entry)]) ||
			v.repeatedCharacter(entry) {
			merged[use] = true
		}
	}
	return merged
}

// characterKey is a use's font (its outline's prefix) and character.
func characterKey(entry rec.Map) string {
	return prefix(rec.FetchString(entry, "path")) + "\x00" + spaceText(rec.FetchString(entry, "text"))
}

// fakeBold: entry copies an earlier bound use of its font and character at its origin.
func fakeBold(entry rec.Map, bound []rec.Map) bool {
	use := rec.FetchInt(entry, "use")
	matrix := matrixValues(entry)
	size := expansion(matrix)
	for _, other := range bound {
		if rec.FetchInt(other, "use") >= use || Close(rec.FetchList(other, "matrix"), rec.FetchList(entry, "matrix")) {
			continue
		}
		origin := matrixValues(other)
		if math.Abs(origin[4]-matrix[4])/fakeBoldDistance < size && math.Abs(origin[5]-matrix[5])/fakeBoldDistance < size {
			return true
		}
	}
	return false
}

// continuation is a character of the page that has no use of its own: one of a glyph's characters
// after its first (unicode_components), when its ToUnicode gives several. MuPDF adds them, zero-width,
// at the glyph's pen, and check_for_fake_bold weighs them like any other character: a CambriaMath
// glyph read as "𝑝𝑝" puts its second 𝑝 0.011 pt from the next glyph's origin, which reads 𝑝𝑝
// too, and that glyph is dropped (papers5/1289).
type continuation struct {
	use    int64
	origin []any
}

// continuations is, by characterKey, the continuation characters of the bound uses' glyphs.
func (v *validator) continuations() map[string][]continuation {
	result := map[string][]continuation{}
	for use, glyph := range v.useGlyph {
		parts, _ := glyph["unicode_components"].([]any)
		for _, value := range parts[min(1, len(parts)):] {
			part := rec.AsMap(value)
			key := prefix(rec.FetchString(v.svg.Uses[use], "path")) + "\x00" + spaceText(text(rec.Fetch(part, "text")))
			result[key] = append(result[key], continuation{use, rec.FetchList(part, "baseline")})
		}
	}
	return result
}

// fakeBoldOfContinuation: entry copies, in fakeBold's sense, a continuation character added before it.
func fakeBoldOfContinuation(entry rec.Map, characters []continuation) bool {
	matrix := matrixValues(entry)
	size := expansion(matrix)
	for _, character := range characters {
		if character.use >= rec.FetchInt(entry, "use") || len(character.origin) != 2 {
			continue
		}
		x, xok := rec.Number(character.origin[0])
		y, yok := rec.Number(character.origin[1])
		if xok && yok && math.Abs(x-matrix[4])/fakeBoldDistance < size && math.Abs(y-matrix[5])/fakeBoldDistance < size {
			return true
		}
	}
	return false
}

// repeatedCharacter: entry repeats the last character added before it, the nearest earlier bound
// use, along that one's direction and at its origin.
func (v *validator) repeatedCharacter(entry rec.Map) bool {
	var last rec.Map
	for index := rec.FetchInt(entry, "use") - 1; index >= 0; index-- {
		if v.used[index] {
			last = v.svg.Uses[index]
			break
		}
	}
	if last == nil || Close(rec.FetchList(last, "matrix"), rec.FetchList(entry, "matrix")) ||
		spaceText(rec.FetchString(last, "text")) != spaceText(rec.FetchString(entry, "text")) {
		return false
	}
	matrix, previous := matrixValues(entry), matrixValues(last)
	size := expansion(matrix)
	if size <= 0 || direction(matrix, previous) < 0.999 {
		return false
	}
	return math.Hypot(matrix[4]-previous[4], matrix[5]-previous[5])/size < fakeBoldDistance
}

// direction is the cosine between two text matrices' baselines.
func direction(matrix, other []float64) float64 {
	length, otherLength := math.Hypot(matrix[0], matrix[1]), math.Hypot(other[0], other[1])
	if length == 0 || otherLength == 0 {
		return 0
	}
	return (float64(matrix[0]*other[0]) + float64(matrix[1]*other[1])) / (length * otherLength)
}

// expansion is fz_matrix_expansion: the square root of the matrix's area scale.
func expansion(matrix []float64) float64 {
	return math.Sqrt(math.Abs(float64(matrix[0]*matrix[3]) - float64(matrix[1]*matrix[2])))
}

func matrixValues(entry rec.Map) []float64 {
	list := rec.FetchList(entry, "matrix")
	values := make([]float64, len(list))
	for index, value := range list {
		number, ok := rec.Number(value)
		if !ok {
			rec.Fail("Invalid SVG glyph matrix")
		}
		values[index] = number
	}
	if len(values) != 6 {
		rec.Fail("Invalid SVG glyph matrix")
	}
	return values
}
