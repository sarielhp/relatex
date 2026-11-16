package pdftag

// The layout numbers a page's glyphs by MuPDF's paint trace, which lists every text paint: a glyph
// shown in rendering mode 2 (fill, then stroke: R's plotting symbols, slides 0011's figures) is
// painted twice, so the trace has a stroke_text after its fill_text and the layout an index it binds
// no glyph to. MuPDF buffers the shown text and paints it when it flushes the buffer (at ET, at an
// XObject, when the rendering mode changes, at the end of a content stream): the fills of the
// buffered glyphs, then their strokes. The interpreter follows that to give each glyph its paint
// index, so the glyph mapping (pages.go) checks and maps glyphs by the trace's numbering. The clip
// modes (4 to 7), which MuPDF paints again at ET, are not modelled: their pages still fail the
// position check and stay artifacts.

// strokeMode is the rendering mode whose glyphs MuPDF fills and then strokes.
const strokeMode = 2

// tracePaints are a content stream's glyphs' paint indices in the trace.
type tracePaints struct {
	of    []int // glyph ordinal → paint index of its fill (its only paint outside mode 2)
	total int   // paints of the stream
}

// append adds a form's paints after the stream's.
func (p *tracePaints) append(inner tracePaints) {
	for _, index := range inner.of {
		p.of = append(p.of, p.total+index)
	}
	p.total += inner.total
}

// span is the paints from glyph ordinal first up to the paint of ordinal end (the total at the
// end).
func (p tracePaints) span(first, end int) int {
	if first >= len(p.of) {
		return 0
	}
	if end >= len(p.of) {
		return p.total - p.of[first]
	}
	return p.of[end] - p.of[first]
}

// trailing is the paints after the last glyph's fill: the strokes of the last flush.
func (p tracePaints) trailing() int {
	if len(p.of) == 0 {
		return 0
	}
	return p.total - p.of[len(p.of)-1] - 1
}

// paint gives the glyphs just shown their paint indices.
func (in *interpreter) paint(count int) {
	if in.state.render != in.buffered {
		in.flush()
		in.buffered = in.state.render
	}
	for range count {
		in.trace.of = append(in.trace.of, in.trace.total)
		in.trace.total++
	}
	if in.state.render == strokeMode {
		in.strokes += count
	}
}

// flush adds the strokes of the buffered text.
func (in *interpreter) flush() {
	in.trace.total += in.strokes
	in.strokes = 0
}
