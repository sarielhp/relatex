package svgoutline

import (
	"math"
	"strconv"

	"github.com/sarielhp/relatex/internal/rec"
)

// Overpaints are copies of one glyph at one placement that repaint.go's identical repaints do not
// cover; the structured text keeps one character for them all (plot markers set in ZapfDingbats in
// a figure, slides/0011):
//
//   - A glyph shown with text render mode 2 (fill, then stroke) is painted twice: MuPDF's paint
//     trace records a fill_text and then a stroke_text, its page SVG a filled use and then an
//     unfilled, stroked use of the same outline. The glyph binds to the fill; the stroke is its
//     companion, admitted only when it adds no more than the overprint tolerance of ink around the
//     measured outline (half its line width at most overprint).
//   - Two plot series marking one point paint the same glyph twice, in two colors. A copy is
//     hidden under a later opaque copy of the same shape (outline, stroke geometry, whether it fills
//     and strokes), whatever their colors: the glyph binds to the topmost copy, the one that shows,
//     and the earlier copies add no ink. A later copy in another color than the bound one is not
//     hidden: it is ink the binding does not show (verifyCoverage).

// strokeCompanion is the half line width (points) of a use that strokes its outline without filling
// it, plainly opaque, or -1.
func strokeCompanion(use *element, matrix []any) float64 {
	stroke, stroked := use.attrs["stroke"]
	if use.attrs["fill"] != "none" || !stroked || stroke == "none" || signature(use) == "" {
		return -1
	}
	width := 1.0
	if text, ok := use.attrs["stroke-width"]; ok {
		value, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return -1
		}
		width = value
	}
	values := make([]float64, 4)
	for index := range values {
		values[index], _ = rec.Number(matrix[index])
	}
	return width * math.Sqrt(math.Abs(float64(values[0]*values[3])-float64(values[1]*values[2]))) / 2
}

// strokedUse: entry is a stroke companion that adds at most the overprint tolerance of ink, and the
// filled use it strokes (filledUse) is other, other painted again, or a copy other hides. paint is
// the paint bound to other, or nil: when entry strokes other itself, its ink is strokeInk's.
func (o *Outlines) strokedUse(entry, other, paint rec.Map) bool {
	stroke := int(rec.FetchInt(entry, "use"))
	filled := o.filledUse(stroke)
	if filled == nil {
		return false
	}
	if filled["use"] == other["use"] {
		return o.strokeInk(stroke, paint) <= overprint
	}
	return o.strokes[stroke] <= overprint && (o.repaintedUse(filled, other) || o.hiddenUse(filled, other))
}

// strokeInk is the half line width (points) of the stroke companion at stroke. MuPDF 1.28.2's SVG
// device writes a stroked glyph's stroke-width as the line width in the user space of the text,
// left untransformed by the page CTM: a glyph of a figure scaled to 0.08 stroked with line width
// 2.16 is drawn 2.16 pt wide in the SVG, 0.18 pt in the PDF (papers/0073, papers4/0976). When paint,
// the fill's paint, carries the line width of its stroke in the paint trace (stroke_line_width) and
// the SVG's width is that line width, the ink is the trace's width transformed by the CTM
// (stroke_device_width); otherwise it is the SVG's width.
func (o *Outlines) strokeInk(stroke int, paint rec.Map) float64 {
	drawn := o.strokes[stroke]
	line, lined := rec.Number(paint["stroke_line_width"])
	device, measured := rec.Number(paint["stroke_device_width"])
	if !lined || !measured || !(line > 0) || !(device >= 0) || math.IsInf(device, 0) || math.Abs(2*drawn-line) > 1e-4*line {
		return drawn
	}
	return device / 2
}

// filledUse is the filled use whose stroke companion the use at stroke is (filledUses), or nil.
func (o *Outlines) filledUse(stroke int) rec.Map {
	if stroke < 0 || stroke >= len(o.filled) || o.filled[stroke] < 0 {
		return nil
	}
	return o.Uses[o.filled[stroke]]
}

// filledUses is, by use index, the filled use a stroke companion strokes, or -1: the nearest earlier
// filled use of its outline at its placement whose run the stroke's run repeats (mirroredRun).
func (o *Outlines) filledUses() []int {
	result := make([]int, len(o.Uses))
	fills := map[string][]int{}
	for stroke, entry := range o.Uses {
		result[stroke] = -1
		path := rec.FetchString(entry, "path")
		if o.strokes[stroke] < 0 {
			fills[path] = append(fills[path], stroke)
			continue
		}
		candidates := fills[path]
		for position := len(candidates) - 1; position >= 0; position-- {
			fill := candidates[position]
			if o.companion(fill, stroke) && mirroredRun(fill, stroke-fill, func(at int) bool { return o.companion(at, at+stroke-fill) }) {
				result[stroke] = fill
				break
			}
		}
	}
	return result
}

// companion: the use at stroke strokes the outline the use at fill fills, at its placement; how much
// ink the stroke adds is strokedUse's question.
func (o *Outlines) companion(fill, stroke int) bool {
	if fill < 0 || stroke >= len(o.strokes) || o.strokes[fill] >= 0 || o.strokes[stroke] < 0 {
		return false
	}
	entry, filled := o.Uses[stroke], o.Uses[fill]
	return entry["path"] == filled["path"] && Close(rec.FetchList(entry, "matrix"), rec.FetchList(filled, "matrix"))
}

// hiddenUse: entry is an earlier opaque copy of other's shape at its placement, whatever its color.
func (o *Outlines) hiddenUse(entry, other rec.Map) bool {
	left, right := rec.FetchInt(entry, "use"), rec.FetchInt(other, "use")
	if left < 0 || left >= right || int(right) >= len(o.shapes) {
		return false
	}
	return o.shapes[left] != "" && o.shapes[left] == o.shapes[right] && entry["path"] == other["path"] &&
		Close(rec.FetchList(entry, "matrix"), rec.FetchList(other, "matrix"))
}

// topCopy is the use a glyph painted several times binds to: the last filled match, when every
// other match is hidden under it or is the stroke companion of a copy; nil otherwise. paint is the
// paint the glyph binds.
func (o *Outlines) topCopy(matches []rec.Map, paint rec.Map) rec.Map {
	var top rec.Map
	for _, entry := range matches {
		if index := rec.FetchInt(entry, "use"); index >= 0 && int(index) < len(o.strokes) && o.strokes[index] < 0 {
			top = entry
		}
	}
	if top == nil {
		return nil
	}
	for _, entry := range matches {
		if entry["use"] != top["use"] && !o.hiddenUse(entry, top) && !o.strokedUse(entry, top, paint) {
			return nil
		}
	}
	return top
}

// strokeWidths gives the fill the line width of its stroke from the paint trace, as the layout reads
// it (line_width, and device_line_width through the CTM), for strokeInk.
func strokeWidths(fill, stroke rec.Map) {
	line, lined := stroke["line_width"]
	device, measured := stroke["device_line_width"]
	if lined && measured {
		fill["stroke_line_width"], fill["stroke_device_width"] = line, device
	}
}

// strokedFill: the paints are one glyph filled and then stroked at one placement, once or more often,
// each stroke in the stroke run that repeats the fill's run (mirroredRun).
func (b *binder) strokedFill(matches []rec.Map) bool {
	if len(matches) < 2 || len(matches)%2 != 0 {
		return false
	}
	for index, entry := range matches {
		operation := "fill_text"
		if index%2 == 1 {
			operation = "stroke_text"
		}
		if entry["operation"] != operation || !samePaint(entry, matches[0]) {
			return false
		}
		if index%2 == 1 && !b.strokesFill(rec.FetchInt(matches[index-1], "index"), rec.FetchInt(entry, "index")) {
			return false
		}
	}
	return true
}

// strokesFill: the paint at stroke is the stroke of the fill at fill, in text render mode 2.
func (b *binder) strokesFill(fill, stroke int64) bool {
	distance := stroke - fill
	return distance > 0 && mirroredRun(int(fill), int(distance), func(at int) bool {
		filled, stroked := b.byIndex[int64(at)], b.byIndex[int64(at)+distance]
		return filled != nil && stroked != nil && filled["operation"] == "fill_text" && stroked["operation"] == "stroke_text" &&
			samePaint(stroked, filled)
	})
}

// mirroredRun: a run of distance items holding at is followed by its copy, in the same order, so
// that at's copy is distance items after it; pairs(item) is whether the item distance after item
// copies it. A glyph run shown in text render mode 2 is so (papers/0073, papers5/1289): MuPDF's
// paint trace records the run's fill_text and then its stroke_text, its page SVG the run's filled
// uses and then its stroked ones, so a glyph's stroke is the run's length after its fill.
func mirroredRun(at, distance int, pairs func(int) bool) bool {
	before, after := 0, 0
	for before < distance-1 && pairs(at-1-before) {
		before++
	}
	for after < distance && pairs(at+after) {
		after++
	}
	return before+after >= distance
}

// samePaint: equal font, glyph and text at one placement; the font may be another subset of one
// font (repaint.go).
func samePaint(entry, first rec.Map) bool {
	if paintFont(rec.Fetch(entry, "font")) != paintFont(rec.Fetch(first, "font")) {
		return false
	}
	for _, key := range paintName {
		if !rec.Equal(rec.Fetch(entry, key), rec.Fetch(first, key)) {
			return false
		}
	}
	for _, key := range paintPlacement {
		if !Close(rec.FetchList(entry, key), rec.FetchList(first, key)) {
			return false
		}
	}
	return true
}
