package reading

// The skips TeX set a typeset line with (the glyph map's flags 16, 32 and 64 on the line's last glyph):
// what TeX knew about the paragraph's alignment, where its geometry alone cannot tell a short
// line centred from one that happens to end there (a list item, a bibliography entry, the last line
// of a justified paragraph, an abstract's single line).
const (
	// SkipUnknown: the line is no broken line of a paragraph (a \centerline's box, a table's row).
	SkipUnknown = iota
	// SkipCentre: \leftskip and \rightskip both stretch with fil glue (\centering, center).
	SkipCentre
	// SkipRight: only \leftskip does (\raggedleft, flushright).
	SkipRight
	// SkipPlain: neither does (a justified or \raggedright paragraph).
	SkipPlain
)

// Confirm is the alignment the geometry read, kept when the skips of every line TeX broke agree
// with it: a centred paragraph has fil skips on both sides of every line, a flush-right one on the
// left alone, and a line broken with neither is flush left whatever its extent. A line whose skips
// are unknown (not broken from a paragraph) says nothing.
func Confirm(alignment string, skips []int) string {
	want := map[string]int{Centred: SkipCentre, Right: SkipRight}[alignment]
	if want == SkipUnknown {
		return ""
	}
	for _, skip := range skips {
		if skip != SkipUnknown && skip != want {
			return ""
		}
	}
	return alignment
}
