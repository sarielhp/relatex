package svgoutline

import (
	"github.com/sarielhp/relatex/internal/rec"
)

// A stacked piece is one of the copies of an extensible delimiter's extension glyph that TeX
// stacks to build a tall bar (\left| or \left\| around a tall formula, \bigg|): one outline, at
// one scale and one horizontal placement, each copy overlapping the next. The page SVG and the
// paint trace keep every copy; MuPDF's structured text measures these zero-advance glyphs as
// overlapping repeats of one character and merges them (it flags the kept one bold), so some
// copies have no text glyph to bind to (papers/0124: three pieces, two characters). The page
// SVG still draws them; only the binding of text to ink lacks them. A copy is a stacked piece
// when it touches, within the overprint tolerance, a bound copy or a copy already admitted, so
// the admitted copies form one unbroken column with a glyph the text binds.

// stackedPieces is the uses of unbound that continue a column of a bound use of their outline.
func stackedPieces(unbound []rec.Map, bound map[string][]rec.Map) map[int64]bool {
	admitted := map[int64]bool{}
	column := map[string][]rec.Map{}
	for path, entries := range bound {
		column[path] = append([]rec.Map(nil), entries...)
	}
	for changed := true; changed; {
		changed = false
		for _, entry := range unbound {
			use := rec.FetchInt(entry, "use")
			path := rec.FetchString(entry, "path")
			if admitted[use] || !continuesColumn(entry, column[path]) {
				continue
			}
			admitted[use] = true
			column[path] = append(column[path], entry)
			changed = true
		}
	}
	return admitted
}

// continuesColumn: entry is a copy of one of the column's uses at its scale and horizontal place
// whose outline box touches it vertically.
func continuesColumn(entry rec.Map, column []rec.Map) bool {
	box, measured := entry["bounds"].([]any)
	if !measured {
		return false
	}
	matrix := rec.FetchList(entry, "matrix")
	for _, other := range column {
		otherBox, ok := other["bounds"].([]any)
		otherMatrix := rec.FetchList(other, "matrix")
		if !ok || !Close(matrix[:5], otherMatrix[:5]) || Close(matrix[5:], otherMatrix[5:]) {
			continue
		}
		if touching(boundsValue(box[1]), boundsValue(box[3]), boundsValue(otherBox[1]), boundsValue(otherBox[3])) {
			return true
		}
	}
	return false
}

// touching: two intervals overlap or lie at most the overprint tolerance apart.
func touching(top, bottom, otherTop, otherBottom float64) bool {
	return top <= otherBottom+overprint && otherTop <= bottom+overprint
}

func boundsValue(value any) float64 {
	result, ok := rec.Number(value)
	if !ok {
		rec.Fail("Invalid SVG glyph outline bounds")
	}
	return result
}
