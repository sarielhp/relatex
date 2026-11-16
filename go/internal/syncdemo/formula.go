package syncdemo

import (
	"cmp"
	"slices"
	"strings"
)

// formulaRanges is the source range of a formula: one range per source line its glyphs came from,
// from the first column to the last, widened over the delimiters ($ $, \( \)) right around it. A
// line the map knows without columns stays a line.
func (d *Data) formulaRanges(origins []Origin) []Origin {
	type key struct{ file, line int }
	merged := map[key]Origin{}
	for _, origin := range origins {
		k := key{origin.File, origin.Line}
		previous, ok := merged[k]
		switch {
		case !ok:
			merged[k] = origin
		case previous.Start < 0 || origin.Start < 0:
			merged[k] = Origin{File: origin.File, Line: origin.Line, Start: -1, End: -1}
		default:
			merged[k] = Origin{File: origin.File, Line: origin.Line, Start: min(previous.Start, origin.Start), End: max(previous.End, origin.End)}
		}
	}
	ranges := make([]Origin, 0, len(merged))
	for _, origin := range merged {
		ranges = append(ranges, d.widen(origin))
	}
	slices.SortFunc(ranges, func(a, b Origin) int { return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line)) })
	return ranges
}

// widen extends a range over a math delimiter that touches it on either side.
func (d *Data) widen(origin Origin) Origin {
	if origin.Start < 0 || origin.File >= len(d.Sources) {
		return origin
	}
	lines := strings.Split(d.Sources[origin.File].Text, "\n")
	if origin.Line < 1 || origin.Line > len(lines) {
		return origin
	}
	line := []rune(lines[origin.Line-1])
	start, end := origin.Start, origin.End
	if start > len(line) || end > len(line) {
		return origin
	}
	switch {
	case start >= 1 && line[start-1] == '$':
		start--
	case start >= 2 && string(line[start-2:start]) == `\(`:
		start -= 2
	}
	probe := end
	for probe < len(line) && (line[probe] == '}' || line[probe] == ' ') {
		probe++ // closing braces and spaces typeset nothing, so no glyph ends there
	}
	switch {
	case probe < len(line) && line[probe] == '$':
		end = probe + 1
	case probe+2 <= len(line) && string(line[probe:probe+2]) == `\)`:
		end = probe + 2
	}
	origin.Start, origin.End = start, end
	return origin
}
