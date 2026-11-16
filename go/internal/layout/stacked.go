package layout

import (
	"fmt"
	"math"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// TeX builds a tall bar (\left| around a tall formula) from copies of an extensible delimiter's
// extension piece stacked in one column; cmex's vextendsingle reads as U+20D3, a nonspacing mark.
// MuPDF 1.28.2's structured text places each such mark zero-width at the pen of the glyph before
// the column, and check_for_fake_bold (FAKE_BOLD_MAX_DIST, a tenth of the size on each axis)
// compares each new glyph's own origin with the characters already kept: when the pen lies that
// near a later piece's origin (the bar right after "dudy", papers/0204 page 15: the pen at x 488.596,
// the column at 489.029, the last piece 0.6 pt under the baseline), that piece is dropped and the
// kept mark flagged bold. The paint trace and the page SVG keep every piece, so the layout had no
// glyph for the dropped paint and the tagged PDF could not bind it (the page's last paint, nothing
// after it to check its place). A dropped piece is given back as a copy of the run's last mark, so
// the layout keeps one glyph per piece as it does for a column MuPDF kept whole; the binder then
// checks the copy at the previous paint's pen (svgoutline combiningOrigin) like the others.

// recoverStackedMarks inserts, after each run of bold-flagged stacked marks, a copy for every piece of
// its paint column that MuPDF dropped, and numbers the page's glyphs again when it inserted any.
func recoverStackedMarks(glyphs []rec.Map, flags []int64, painted []rec.Map, number int64) []rec.Map {
	if len(flags) != len(glyphs) {
		rec.Fail("Metric glyph flags differ")
	}
	result := make([]rec.Map, 0, len(glyphs))
	inserted := false
	for index := 0; index < len(glyphs); {
		end := markRunEnd(glyphs, index)
		if end == index {
			result = append(result, glyphs[index])
			index++
			continue
		}
		result = append(result, glyphs[index:end]...)
		if anyBold(flags[index:end]) {
			last := glyphs[end-1]
			for missing := len(stackedColumn(last, painted)) - (end - index); missing > 0; missing-- {
				result = append(result, copyRecord(last))
				inserted = true
			}
		}
		index = end
	}
	if inserted {
		for position, glyph := range result {
			glyph["id"] = fmt.Sprintf("p%d-g%d", number, position+1)
		}
	}
	return result
}

// markRunEnd is the end of the run of zero-width marks of one character, font, line and origin that
// starts at index; index itself when glyphs[index] is no such mark.
func markRunEnd(glyphs []rec.Map, index int) int {
	first := glyphs[index]
	if !nonspacingMark(rec.FetchString(first, "text")) {
		return index
	}
	end := index
	for end < len(glyphs) && sameMark(glyphs[end], first) {
		end++
	}
	return end
}

func sameMark(glyph, first rec.Map) bool {
	return rec.FetchString(glyph, "text") == rec.FetchString(first, "text") && rec.Equal(glyph["font"], first["font"]) &&
		rec.Equal(glyph["line"], first["line"]) && within(rec.FetchList(glyph, "baseline"), rec.FetchList(first, "baseline")) &&
		zeroWidth(glyph)
}

func nonspacingMark(text string) bool {
	character, size := utf8.DecodeRuneInString(text)
	return size > 0 && size == len(text) && unicode.Is(unicode.Mn, character)
}

func anyBold(flags []int64) bool {
	for _, flag := range flags {
		if flag&boldFlag != 0 {
			return true
		}
	}
	return false
}

// stackedColumn is the column of paints a mark's dropped piece belongs to: the consecutive paints of
// its character and font glyph at one horizontal place around the paints whose own origins lie
// within check_for_fake_bold's distance of the mark's origin. Nil when no paint lies that near, or
// the near paints are not in one column.
func stackedColumn(mark rec.Map, painted []rec.Map) []rec.Map {
	reach := fakeBoldDistance * nativeSize(mark)
	near := []int{}
	for index, paint := range painted {
		if sameCharacter(paint, mark) && nearOrigin(rec.FetchList(paint, "baseline"), rec.FetchList(mark, "baseline"), reach) {
			near = append(near, index)
		}
	}
	if len(near) == 0 {
		return nil
	}
	first, last := near[0], near[0]
	for first > 0 && sameColumn(painted[first-1], painted[near[0]]) {
		first--
	}
	for last+1 < len(painted) && sameColumn(painted[last+1], painted[near[0]]) {
		last++
	}
	if near[len(near)-1] > last {
		return nil
	}
	return painted[first : last+1]
}

// fakeBoldDistance is MuPDF's FAKE_BOLD_MAX_DIST, a fraction of the glyph's size.
const fakeBoldDistance = 0.1

func nativeSize(record rec.Map) float64 {
	size, _ := rec.Number(record["size"])
	return size
}

func sameCharacter(paint, mark rec.Map) bool {
	return rec.FetchString(paint, "text") == rec.FetchString(mark, "text") && paintFont(paint["font"]) == rec.ToS(mark["font"])
}

func sameColumn(paint, other rec.Map) bool {
	return rec.Equal(paint["font"], other["font"]) && rec.Equal(paint["glyph"], other["glyph"]) &&
		rec.FetchString(paint, "text") == rec.FetchString(other, "text") &&
		within(rec.FetchList(paint, "baseline")[:1], rec.FetchList(other, "baseline")[:1])
}

// nearOrigin: each axis less than reach apart, as check_for_fake_bold compares.
func nearOrigin(origin, other []any, reach float64) bool {
	for axis := range 2 {
		a, _ := rec.Number(origin[axis])
		b, _ := rec.Number(other[axis])
		if !(math.Abs(a-b) < reach) {
			return false
		}
	}
	return true
}

func copyRecord(record rec.Map) rec.Map {
	copied := make(rec.Map, len(record))
	for key, value := range record {
		copied[key] = value
	}
	return copied
}
