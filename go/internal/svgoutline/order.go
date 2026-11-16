package svgoutline

import (
	"sort"

	"github.com/sarielhp/relatex/internal/rec"
)

// Stacked fonts are glyphs of different fonts painted at one placement with the same text: xy-pic
// draws an arrow tip as one glyph of xyatip10 and one of xybtip10 at one point, both read as
// U+FFFD (papers/0102). Their SVG uses name different fonts (font_17_2 and font_18_2) and nothing
// but the order of painting tells which use is which paint: MuPDF's paint trace and its SVG device
// draw one display list, so the k-th of those paints (by paint index) is the k-th of those uses (by
// use index). The pairing holds only when the paints are as many as the uses and come from
// distinct fonts drawn by distinct SVG fonts; the validator's font identity (bindFont) then checks
// it against every other glyph of those fonts on the page.

// stackedFontUse is the use a paint binds to among several matches by paint order, or nil.
func (b *binder) stackedFontUse(paint rec.Map, first string) rec.Map {
	if first == "" {
		return nil
	}
	matrix := rec.FetchList(paint, "matrix")
	paints := b.placementPaints(first, matrix)
	uses := []rec.Map{}
	for _, entry := range b.candidates[first].near(lastTwo(matrix)) {
		if Close(rec.FetchList(entry, "matrix"), matrix) {
			uses = append(uses, entry)
		}
	}
	if len(paints) != len(uses) || len(uses) < 2 || !distinct(paints, func(entry rec.Map) string { return paintFont(rec.Fetch(entry, "font")) }) ||
		!distinct(uses, func(entry rec.Map) string { return prefix(rec.FetchString(entry, "path")) }) {
		return nil
	}
	sort.Slice(paints, func(a, c int) bool { return rec.FetchInt(paints[a], "index") < rec.FetchInt(paints[c], "index") })
	sort.Slice(uses, func(a, c int) bool { return rec.FetchInt(uses[a], "use") < rec.FetchInt(uses[c], "use") })
	for rank, entry := range paints {
		if rec.FetchInt(entry, "index") == rec.FetchInt(paint, "index") && !b.used[rec.FetchInt(uses[rank], "use")] {
			return uses[rank]
		}
	}
	return nil
}

// placementPaints are the page's text paints whose text starts with first, at the matrix.
func (b *binder) placementPaints(first string, matrix []any) []rec.Map {
	found := []rec.Map{}
	for _, entry := range b.all {
		operation := rec.Fetch(entry, "operation")
		if operation != "fill_text" && operation != "clip_text" {
			continue
		}
		entryText := spaceText(text(rec.Fetch(entry, "text")))
		if entryText == "" || []rune(entryText)[0] != []rune(first)[0] {
			continue
		}
		if Close(rec.FetchList(entry, "matrix"), matrix) {
			found = append(found, entry)
		}
	}
	return found
}

// distinct: no two entries share a key.
func distinct(entries []rec.Map, key func(rec.Map) string) bool {
	seen := map[string]bool{}
	for _, entry := range entries {
		name := key(entry)
		if seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

// stackedFontCompanion: entry is an unbound use stacked on a bound use of another SVG font, with
// the same text at one placement. MuPDF's structured text keeps one character for the stacked
// zero-advance glyphs (the xyatip half of the arrow tip, not the xybtip half), so the other has
// no text glyph to bind to; the page SVG still draws it, as for stacked pieces (stack.go).
func stackedFontCompanion(entry rec.Map, bound []rec.Map) bool {
	matrix := rec.FetchList(entry, "matrix")
	font := prefix(rec.FetchString(entry, "path"))
	for _, other := range bound {
		if other["text"] == entry["text"] && (prefix(rec.FetchString(other, "path")) != font || other["path"] != entry["path"]) &&
			Close(rec.FetchList(other, "matrix"), matrix) {
			return true
		}
	}
	return false
}
