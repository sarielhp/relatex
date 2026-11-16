package layout

import (
	"strconv"

	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// MuPDF's structured text drops a character that repeats the one before it at the same origin,
// taking it for an overstruck (fake bold) copy, and marks the kept one bold. A ligature whose
// ToUnicode spells several letters gives its last letter a zero-width continuation at the pen
// after the ligature; when the next glyph is that same letter (the "fi" ligature and "i" of
// "Rafii", papers4/1123), MuPDF drops the real glyph as an overstrike of the continuation and
// fills the gap its advance leaves with a synthetic space. The paint trace still has the glyph,
// and the page SVG its outline. Such a synthetic space is the dropped glyph: it takes the
// glyph's text when it is exactly that glyph's advance box after a bold continuation of the same
// letter in the same font, and the paint trace has that glyph there; any other synthetic space
// stays a space.

// Structured-text character flags (MuPDF's FZ_STEXT_*).
const (
	syntheticFlag = 4
	boldFlag      = 8
)

// charFlags are a page's structured-text characters' flags, in glyphsFor's order.
func charFlags(page *nativetrace.Node) []int64 {
	flags := []int64{}
	for _, line := range page.Descendants("line") {
		for _, char := range line.Descendants("char") {
			value, _ := char.Attr("flags")
			number, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				number = 0
			}
			flags = append(flags, number)
		}
	}
	return flags
}

// recoverOverstruck gives each synthetic space that stands for a dropped overstruck glyph that
// glyph's text.
func recoverOverstruck(glyphs []rec.Map, flags []int64, painted []rec.Map) {
	if len(flags) != len(glyphs) {
		rec.Fail("Metric glyph flags differ")
	}
	for index := 1; index < len(glyphs); index++ {
		glyph, before := glyphs[index], glyphs[index-1]
		if flags[index]&syntheticFlag == 0 || flags[index-1]&boldFlag == 0 || rec.FetchString(glyph, "text") != " " ||
			!rec.Equal(glyph["line"], before["line"]) || !rec.Equal(glyph["font"], before["font"]) || !zeroWidth(before) {
			continue
		}
		if paint := droppedPaint(glyph, rec.FetchString(before, "text"), painted); paint != nil {
			glyph["text"] = rec.FetchString(paint, "text")
		}
	}
}

// zeroWidth: a record whose quad has no width, as a continuation's has.
func zeroWidth(record rec.Map) bool {
	quad := rec.FetchList(record, "metric_quad")
	return within(quad[0:2], quad[2:4]) && within(quad[4:6], quad[6:8])
}

// droppedPaint is the one paint of text in the space's font at its origin whose horizontal
// advance ends where the space's box does, or nil.
func droppedPaint(space rec.Map, text string, painted []rec.Map) rec.Map {
	quad := rec.FetchList(space, "metric_quad")
	var found rec.Map
	for _, paint := range painted {
		if rec.FetchString(paint, "text") != text || paintFont(paint["font"]) != rec.ToS(space["font"]) ||
			!within(rec.FetchList(paint, "baseline"), rec.FetchList(space, "baseline")) {
			continue
		}
		baseline, advance := rec.FetchList(paint, "baseline"), rec.FetchList(paint, "advance")
		x, _ := rec.Number(baseline[0])
		dx, _ := rec.Number(advance[0])
		dy, _ := rec.Number(advance[1])
		if !within([]any{dy, quad[2], quad[6]}, []any{0.0, x + dx, x + dx}) {
			continue
		}
		if found != nil {
			return nil
		}
		found = paint
	}
	return found
}
