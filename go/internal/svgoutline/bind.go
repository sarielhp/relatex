package svgoutline

import (
	"math"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// pointIndex is PointIndex: entries by a 2D point, in cells of one unit. near returns, in
// their original order, the entries in the point's cell or a neighboring one, a superset of
// every entry closer than one unit on both axes; callers filter it with their exact test.
type pointIndex struct {
	cells map[[2]int64][]indexedEntry
}

type indexedEntry struct {
	index int
	entry rec.Map
}

func newPointIndex(entries []rec.Map, point func(rec.Map) []any) *pointIndex {
	p := &pointIndex{cells: map[[2]int64][]indexedEntry{}}
	for index, entry := range entries {
		cell := cellOf(point(entry))
		p.cells[cell] = append(p.cells[cell], indexedEntry{index, entry})
	}
	return p
}

func cellOf(point []any) [2]int64 {
	x, _ := rec.Number(point[0])
	y, _ := rec.Number(point[1])
	return [2]int64{int64(math.Floor(x)), int64(math.Floor(y))}
}

func (p *pointIndex) near(point []any) []rec.Map {
	if p == nil {
		return nil
	}
	center := cellOf(point)
	found := []indexedEntry{}
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			found = append(found, p.cells[[2]int64{center[0] + dx, center[1] + dy}]...)
		}
	}
	sort.Slice(found, func(a, b int) bool { return found[a].index < found[b].index })
	result := make([]rec.Map, len(found))
	for index, item := range found {
		result[index] = item.entry
	}
	return result
}

// binder is PDFGlyphOutlines.new(svg, paint:) for bind: the validation state, the painted
// glyphs by font and text, and the SVG uses by text, each indexed by position.
type binder struct {
	*validator
	painted     map[string][]rec.Map
	all         []rec.Map
	paintedNear map[string]*pointIndex
	candidates  map[string]*pointIndex
	// byIndex is every paint by its index (stroke runs, overpaint.go); pending is the structured glyphs
	// by key in binding order, and seen how many of them are bound or being bound (inPaintOrder).
	byIndex map[int64]rec.Map
	pending map[string][]rec.Map
	seen    map[string]int
}

func newValidator(svg []byte) *validator {
	return &validator{svg: For(svg), used: map[int64]bool{}, paintUsed: map[int64]bool{}, fonts: map[string]any{},
		paths: map[string][]any{}, glyphPaths: map[string]string{}, usePaint: map[int64]rec.Map{},
		useGlyph: map[int64]rec.Map{}}
}

// paintKey is a paint's (or a structured glyph's) font name as the structured text writes it, with
// its text.
func paintKey(font, text string) string { return font + "\x00" + spaceText(text) }

func glyphKey(glyph rec.Map) string {
	return paintKey(glyphFont(rec.Fetch(glyph, "font")), text(rec.Fetch(glyph, "text")))
}

func lastTwo(list []any) []any { return list[len(list)-2:] }

// Bind is PDFGlyphOutlines.new(svg, paint: painted).bind(glyphs): each glyph bound to the
// paint that drew it and to its SVG outline use, or kept as a synthetic space.
func Bind(svg []byte, painted, glyphs []rec.Map) []rec.Map {
	b := &binder{validator: newValidator(svg), painted: map[string][]rec.Map{}, paintedNear: map[string]*pointIndex{},
		candidates: map[string]*pointIndex{}, all: painted, byIndex: map[int64]rec.Map{}, pending: map[string][]rec.Map{},
		seen: map[string]int{}}
	for _, glyph := range painted {
		key := paintKey(paintFont(rec.Fetch(glyph, "font")), text(rec.Fetch(glyph, "text")))
		b.painted[key] = append(b.painted[key], glyph)
		b.byIndex[rec.FetchInt(glyph, "index")] = glyph
	}
	for _, glyph := range glyphs {
		key := glyphKey(glyph)
		b.pending[key] = append(b.pending[key], glyph)
	}
	for key, entries := range b.painted {
		b.paintedNear[key] = newPointIndex(entries, func(entry rec.Map) []any { return rec.FetchList(entry, "baseline") })
	}
	uses := map[string][]rec.Map{}
	for _, use := range b.svg.Uses {
		name := spaceText(rec.FetchString(use, "text"))
		uses[name] = append(uses[name], use)
	}
	for name, entries := range uses {
		b.candidates[name] = newPointIndex(entries, func(entry rec.Map) []any { return lastTwo(rec.FetchList(entry, "matrix")) })
	}
	result := make([]rec.Map, 0, len(glyphs))
	for _, glyph := range glyphs {
		measured := b.bindGlyph(glyph)
		b.validateBinding(measured)
		result = append(result, measured)
	}
	b.verifyCoverage()
	return result
}

func (b *binder) bindGlyph(glyph rec.Map) rec.Map {
	paint, origin := b.physicalGlyph(glyph)
	var binding rec.Map
	if paint != nil {
		binding = b.invisibleBinding(paint)
		if binding == nil {
			binding = b.offPageBinding(glyph, paint)
		}
		if binding == nil {
			binding = b.bindPaint(paint, rec.Fetch(glyph, "id"))
		}
		binding["origin_binding"] = origin
	} else {
		binding = syntheticSpace(glyph)
	}
	measured := rec.Map{}
	for key, value := range glyph {
		measured[key] = value
	}
	measured["outline_binding"] = binding
	measured["ink_quad"] = Quad(binding["bounds"], rec.Fetch(glyph, "baseline"))
	return measured
}

func (b *binder) physicalGlyph(glyph rec.Map) (rec.Map, string) {
	glyphText := text(rec.Fetch(glyph, "text"))
	key := glyphKey(glyph)
	b.seen[key]++
	baseline := rec.FetchList(glyph, "baseline")
	// A paint an earlier glyph is bound to is no candidate: a paint binds one glyph (validatePaint).
	matches := []rec.Map{}
	for _, entry := range b.paintedNear[key].near(baseline) {
		if Close(rec.FetchList(entry, "baseline"), baseline) && !b.paintUsed[rec.FetchInt(entry, "index")] {
			matches = append(matches, entry)
		}
	}
	if b.strokedFill(matches) {
		strokeWidths(matches[0], matches[1])
		return matches[0], "paint_origin"
	}
	if len(matches) == 1 || repaintedSpace(glyphText, matches) || repaints(matches) || invisibleRun(matches) ||
		b.inPaintOrder(key, baseline, matches) || b.stackedPieces(key, baseline, matches) {
		return matches[0], "paint_origin"
	}
	if len(matches) == 0 {
		combining := []rec.Map{}
		for _, entry := range b.painted[key] {
			if !b.paintUsed[rec.FetchInt(entry, "index")] && b.combiningOrigin(glyph, entry) {
				combining = append(combining, entry)
			}
		}
		if len(combining) == 1 {
			return combining[0], "previous_paint_pen"
		}
		if blank(glyphText) {
			return nil, ""
		}
	}
	rec.Fail("Missing or ambiguous native glyph paint: %s (%d matches)", rec.ToS(rec.Fetch(glyph, "id")), len(matches))
	return nil, ""
}

// inPaintOrder: as many structured glyphs still to bind lie at this origin as there are paints left
// there. MuPDF's structured text keeps one character per paint in paint order, so each glyph takes
// the first paint left: two glyphs of one font that both read as U+FFFD at one origin (a LaTeX line
// font piece and a control character, papers5/1457). The SVG side tells their outlines apart
// (ownOutlines), and verifyCoverage still requires every outline to be bound.
func (b *binder) inPaintOrder(key string, baseline []any, matches []rec.Map) bool {
	if len(matches) < 2 {
		return false
	}
	count := 0
	for _, glyph := range b.pending[key][b.seen[key]-1:] {
		if Close(rec.FetchList(glyph, "baseline"), baseline) {
			count++
		}
	}
	return count == len(matches)
}

// stackedPieces: several paints of one font and character at this origin, drawn as different outlines,
// under fewer structured glyphs than paints. MuPDF keeps one character for glyphs stacked at one
// origin (a LaTeX line font piece drawn over another, both read as U+FFFD, the only text that names no glyph; papers5/1457), so the
// glyph takes the first paint; stackedFontCompanion covers the outlines left over.
func (b *binder) stackedPieces(key string, baseline []any, matches []rec.Map) bool {
	if len(matches) < 2 || !strings.HasSuffix(key, "\x00\uFFFD") {
		return false
	}
	for _, entry := range matches[1:] {
		if rec.Equal(rec.Fetch(entry, "glyph"), rec.Fetch(matches[0], "glyph")) {
			return false
		}
	}
	count := 0
	for _, glyph := range b.pending[key][b.seen[key]-1:] {
		if Close(rec.FetchList(glyph, "baseline"), baseline) {
			count++
		}
	}
	return count == 1
}

// repaintedSpace: a space glyph painted twice at one spot, as when an ocgx2 link ends at a
// word space: once in the link's clip and once after it.
func repaintedSpace(glyphText string, matches []rec.Map) bool {
	if !blank(glyphText) || len(matches) == 0 {
		return false
	}
	first := rec.Fetch(matches[0], "matrix")
	for _, entry := range matches[1:] {
		if !rec.Equal(rec.Fetch(entry, "matrix"), first) {
			return false
		}
	}
	return true
}

// repeatedUses: every match is the first one, painted again (or its stroke companion).
func (b *binder) repeatedUses(matches []rec.Map, paint rec.Map) bool {
	for _, entry := range matches {
		if !(repaintedBlank(entry, matches[0]) || entry["use"] == matches[0]["use"] ||
			b.svg.repaintedUse(entry, matches[0]) || b.svg.strokedUse(entry, matches[0], paint)) {
			return false
		}
	}
	return true
}

func (b *binder) bindPaint(paint rec.Map, id any) rec.Map {
	operation := rec.Fetch(paint, "operation")
	if operation != "fill_text" && operation != "clip_text" {
		rec.Fail("Unsupported stroked or invisible glyph outline")
	}
	first := drawnFirst(paint)
	matrix := rec.FetchList(paint, "matrix")
	matches := []rec.Map{}
	for _, entry := range b.candidates[first].near(lastTwo(matrix)) {
		if Close(rec.FetchList(entry, "matrix"), matrix) && !b.used[rec.FetchInt(entry, "use")] {
			matches = append(matches, entry)
		}
	}
	matches = b.ownOutlines(paint, matches)
	if b.repeatedUses(matches, paint) && len(matches) > 1 {
		matches = matches[:1]
	} else if top := b.svg.topCopy(matches, paint); len(matches) > 1 && top != nil {
		matches = []rec.Map{top}
	} else if len(matches) > 1 {
		if stacked := b.stackedFontUse(paint, first); stacked != nil {
			matches = []rec.Map{stacked}
		} else if own := b.svg.filledAs(matches, paint); len(own) < len(matches) {
			// The paint's own color tells the uses apart; what is left may be copies of one ink.
			matches = own
			if b.repeatedUses(matches, paint) {
				matches = matches[:1]
			}
		}
	}
	if len(matches) != 1 {
		rec.Fail("Native glyph has no SVG outline placement: %s (%d matches)", rec.ToS(id), len(matches))
	}
	binding := rec.Map{"paint": paint}
	for key, value := range matches[0] {
		binding[key] = value
	}
	return binding
}

// ownOutlines is the matches the paint may bind to by the page's glyph mapping (bindPath): a use
// whose outline an earlier binding gave another font glyph, or another outline than the one it gave
// this paint's glyph, is not the paint's. Two glyphs at one placement are told apart so (papers5/1457).
// When no match is left, the matches stay as they are for the failure to report.
func (b *binder) ownOutlines(paint rec.Map, matches []rec.Map) []rec.Map {
	if len(matches) < 2 {
		return matches
	}
	identity := []any{rec.Fetch(paint, "font"), rec.Fetch(paint, "glyph")}
	kept := []rec.Map{}
	for _, entry := range matches {
		path := rec.FetchString(entry, "path")
		known, seen := b.paths[path]
		glyphPath, mapped := b.glyphPaths[glyphPathKey(identity, path)]
		if (!seen || rec.Equal(known, identity)) && (!mapped || glyphPath == path) {
			kept = append(kept, entry)
		}
	}
	if len(kept) == 0 {
		return matches
	}
	return kept
}
