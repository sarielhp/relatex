package svgoutline

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubyuri"
)

// overprint is PDFGlyphOutlines::OVERPRINT: how far apart \pmb overprint copies of one glyph
// may be (points).
const overprint = 0.5

// paintOperations is PDFGlyphOutlines::PAINT.
var paintOperations = []any{"fill_text", "clip_text"}

// fontPrefix is what MuPDF's structured text leaves out of a paint trace's font name: everything
// through the first "+", a subset tag ("ABCDEF+") or another prefix ("CIDFont+F1" is F1 there,
// papers3/0726), once: "BHWZGQ+CIDFont+F1" is CIDFont+F1 there (papers/0217). A structured-text
// font name is already without it, so it is compared as it is.
var fontPrefix = regexp.MustCompile(`\A[^+]*\+`)

// validator is PDFGlyphOutlines.new(svg) for validate!: the binding of a page's glyphs to
// its SVG glyph uses, checked glyph by glyph.
type validator struct {
	svg        *Outlines
	used       map[int64]bool
	paintUsed  map[int64]bool
	fonts      map[string]any
	paths      map[string][]any
	glyphPaths map[string]string
	previous   rec.Map
	// usePaint is the paint bound to each bound use (strokedUse).
	usePaint map[int64]rec.Map
	// useGlyph is the glyph bound to each bound use (merge.go).
	useGlyph map[int64]rec.Map
}

// Validate is PDFGlyphOutlines.new(svg).validate!(glyphs).
func Validate(svg []byte, glyphs []any) {
	v := newValidator(svg)
	for _, glyph := range glyphs {
		v.validateBinding(rec.AsMap(glyph))
	}
	v.verifyCoverage()
}

// paintFont is a paint record's font name as the structured text writes it.
func paintFont(name any) string { return fontPrefix.ReplaceAllString(glyphFont(name), "") }

// glyphFont is a structured-text glyph's font name.
func glyphFont(name any) string {
	text, ok := name.(string)
	if !ok || text == "" {
		rec.Fail("Invalid native outline font name")
	}
	return text
}

// blank is /\A\s+\z/ (Ruby's ASCII \s).
func blank(text string) bool {
	if text == "" {
		return false
	}
	for index := 0; index < len(text); index++ {
		if !strings.ContainsRune(" \t\n\v\f\r", rune(text[index])) {
			return false
		}
	}
	return true
}

// spaceText is a glyph's text as the paint trace spells it: one blank character is a space. A
// matplotlib figure in ArialMT paints its space glyph under an ActualText of a no-break space
// (papers3/0565), or of a newline that ends a label line (papers/0057, papers/0098); the paint
// trace and the page SVG record the glyph's own " ", MuPDF's structured text the ActualText.
// The two are one blank glyph.
func spaceText(text string) string {
	if text == " " || (len(text) == 1 && blank(text)) {
		return " "
	}
	return text
}

// drawnFirst is the first character of the text a paint's glyph draws, as the page SVG spells it:
// its drawn_text when the paint's text is a replacement (/ActualText, layout's actualText), else its
// text.
func drawnFirst(paint rec.Map) string {
	drawn, replaced := paint["drawn_text"]
	if !replaced {
		drawn = rec.Fetch(paint, "text")
	}
	value := spaceText(text(drawn))
	if value == "" {
		return ""
	}
	_, size := utf8.DecodeRuneInString(value)
	return spaceText(value[:size])
}

// mark is /\A\p{Mn}\z/: one nonspacing mark.
func mark(text string) bool {
	character, size := utf8.DecodeRuneInString(text)
	return size == len(text) && size > 0 && unicode.Is(unicode.Mn, character)
}

func syntheticSpace(glyph rec.Map) rec.Map {
	if !blank(text(rec.Fetch(glyph, "text"))) {
		rec.Fail("Only whitespace may lack native glyph paint")
	}
	return rec.Map{"status": "synthetic_space", "bounds": nil}
}

func text(value any) string {
	result, ok := value.(string)
	if !ok {
		rec.Fail("undefined method 'match?' for %T", value)
	}
	return result
}

func (v *validator) validateBinding(glyph rec.Map) {
	binding := rec.FetchMap(glyph, "outline_binding")
	switch binding["status"] {
	case "synthetic_space":
		if !rec.Equal(binding, syntheticSpace(glyph)) {
			rec.Fail("Invalid synthetic space binding")
		}
	case outsidePage:
		v.validateOffPage(glyph, binding)
	case invisibleText:
		v.validateInvisible(glyph, binding)
	default:
		v.validatePainted(glyph, binding)
	}
	expected := Quad(rec.Fetch(binding, "bounds"), rec.Fetch(glyph, "baseline"))
	if !rec.Equal(rec.Fetch(glyph, "ink_quad"), expected) {
		rec.Fail("Glyph ink quad differs from measured outline")
	}
	v.previous = glyph
}

func (v *validator) validatePainted(glyph, binding rec.Map) {
	index, isInt := rec.Fetch(binding, "use").(int64)
	if !isInt || index < 0 || v.used[index] {
		rec.Fail("Invalid or reused SVG glyph placement")
	}
	v.used[index] = true
	if index >= int64(len(v.svg.Uses)) {
		rec.Fail("Dangling SVG glyph placement")
	}
	native := v.svg.Uses[index]
	stripped := rec.Map{}
	for key, value := range binding {
		if key != "paint" && key != "origin_binding" {
			stripped[key] = value
		}
	}
	if !rec.Equal(stripped, native) {
		rec.Fail("Glyph outline differs from retained SVG")
	}
	paint := rec.FetchMap(binding, "paint")
	v.usePaint[index], v.useGlyph[index] = paint, glyph
	v.validatePaint(paint)
	glyphText := spaceText(text(rec.Fetch(glyph, "text")))
	matrix := rec.FetchList(paint, "matrix")
	if paintFont(rec.Fetch(paint, "font")) != glyphFont(rec.Fetch(glyph, "font")) || spaceText(text(rec.Fetch(paint, "text"))) != glyphText ||
		glyphText == "" || spaceText(text(rec.Fetch(native, "text"))) != drawnFirst(paint) || !slices.Contains(paintOperations, rec.Fetch(paint, "operation")) ||
		!Close(matrix, rec.FetchList(native, "matrix")) || !Close(matrix[len(matrix)-2:], rec.FetchList(paint, "baseline")) {
		rec.Fail("SVG outline and native glyph identities differ")
	}
	v.validateOrigin(glyph, paint, rec.Fetch(binding, "origin_binding"))
	path := rec.FetchString(native, "path")
	v.bindFont(rec.Fetch(paint, "font"), path)
	v.bindPath(paint, path)
}

func (v *validator) validatePaint(paint rec.Map) {
	for _, field := range []struct {
		key   string
		count int
	}{{"baseline", 2}, {"advance", 2}, {"matrix", 6}} {
		geometry.Numbers(rec.FetchList(paint, field.key), field.count)
	}
	index, isInt := rec.Fetch(paint, "index").(int64)
	name, isString := rec.Fetch(paint, "glyph").(string)
	if !isInt || index < 0 || v.paintUsed[index] || !isString || name == "" {
		rec.Fail("Invalid or reused native glyph paint identity")
	}
	v.paintUsed[index] = true
}

func (v *validator) validateOrigin(glyph, paint rec.Map, origin any) {
	if unicodeBinding, ok := glyph["unicode_binding"]; ok && !rec.Equal(unicodeBinding, paint) {
		rec.Fail("Glyph outline and Unicode paint bindings differ")
	}
	valid := false
	switch origin {
	case "paint_origin":
		valid = Close(rec.FetchList(paint, "baseline"), rec.FetchList(glyph, "baseline"))
	case "previous_paint_pen":
		valid = v.combiningOrigin(glyph, paint)
	}
	if !valid {
		rec.Fail("Glyph structured/paint origin binding differs")
	}
}

// combiningOrigin: MuPDF 1.28.2 places a nonspacing mark at the previous pen in structured
// text, even when its physical glyph origin differs. Require exact paint adjacency and that
// pen, not a nearest-glyph or distance heuristic.
func (v *validator) combiningOrigin(glyph, paint rec.Map) bool {
	if !mark(text(rec.Fetch(glyph, "text"))) || v.previous == nil || !rec.Equal(rec.Fetch(glyph, "line"), rec.Fetch(v.previous, "line")) {
		return false
	}
	binding, _ := v.previous["outline_binding"].(rec.Map)
	previous, ok := binding["paint"].(rec.Map)
	if !ok {
		return false
	}
	index, _ := rec.Number(rec.Fetch(paint, "index"))
	before, _ := rec.Number(rec.Fetch(previous, "index"))
	if index != before+1 {
		return false
	}
	quad := rec.FetchList(glyph, "metric_quad")
	if !Close(quad[0:2], quad[2:4]) || !Close(quad[4:6], quad[len(quad)-2:]) {
		return false
	}
	// The pieces of an extensible delimiter (\left\|) are marks that MuPDF all places at the
	// pen before the first piece: a mark after a mark keeps its structured position.
	sharedPen := Close(rec.FetchList(v.previous, "baseline"), rec.FetchList(glyph, "baseline"))
	if v.stackedMark() {
		return sharedPen
	}
	// The same holds when the first piece's own origin is that pen (an extensible arrow accent
	// after a comma, books/0003 page 500): the first piece binds at its paint origin.
	if sharedPen && mark(text(rec.Fetch(v.previous, "text"))) {
		return true
	}
	baseline, advance := rec.FetchList(previous, "baseline"), rec.FetchList(previous, "advance")
	pen := make([]any, len(baseline))
	for position := range baseline {
		a, _ := rec.Number(baseline[position])
		b, _ := rec.Number(advance[position])
		pen[position] = a + b
	}
	return Close(pen, rec.FetchList(glyph, "baseline"))
}

func (v *validator) stackedMark() bool {
	binding, _ := v.previous["outline_binding"].(rec.Map)
	return mark(text(rec.Fetch(v.previous, "text"))) && binding["origin_binding"] == "previous_paint_pen"
}

// prefix is path.rpartition('_').first.
func prefix(path string) string {
	if index := strings.LastIndexByte(path, '_'); index >= 0 {
		return path[:index]
	}
	return ""
}

func (v *validator) bindFont(font any, path string) {
	key := prefix(path)
	if known, ok := v.fonts[key]; ok && !rec.Equal(known, font) {
		rec.Fail("SVG glyph font identity conflicts")
	}
	v.fonts[key] = font
}

func inspect(values []any) string {
	if values == nil {
		return "nil"
	}
	parts := make([]string, len(values))
	for index, value := range values {
		if text, ok := value.(string); ok {
			parts[index] = rubyuri.Inspect(text)
		} else {
			parts[index] = fmt.Sprint(value)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// glyphPathKey is a paint glyph identity within the SVG font of path.
func glyphPathKey(identity []any, path string) string {
	return fmt.Sprintf("%v\x00%v\x00%s", identity[0], identity[1], prefix(path))
}

func (v *validator) bindPath(paint rec.Map, path string) {
	identity := []any{rec.Fetch(paint, "font"), rec.Fetch(paint, "glyph")}
	key := glyphPathKey(identity, path)
	known, seen := v.paths[path]
	glyphPath, mapped := v.glyphPaths[key]
	if (seen && !rec.Equal(known, identity)) || (mapped && glyphPath != path) {
		rec.Fail("SVG glyph mapping conflicts: %s %s; previous %s, %s", path, inspect(identity), inspect(known), glyphPath)
	}
	v.paths[path] = identity
	v.glyphPaths[key] = path
}

// verifyCoverage: every SVG glyph placement belongs to a text glyph, except repaints (repaint.go)
// and overprints: \pmb paints its glyph three times, a fraction of a point apart, under one text glyph.
func (v *validator) verifyCoverage() {
	bound, copies := map[string][]rec.Map{}, map[string][]rec.Map{}
	unbound, all := []rec.Map{}, []rec.Map{}
	for _, entry := range v.svg.Uses {
		if v.used[rec.FetchInt(entry, "use")] {
			path, outline := rec.FetchString(entry, "path"), v.svg.outline(entry)
			bound[path] = append(bound[path], entry)
			copies[outline] = append(copies[outline], entry)
			all = append(all, entry)
		} else {
			unbound = append(unbound, entry)
		}
	}
	stacked, merged := stackedPieces(unbound, bound), v.mergedCopies(unbound)
	for _, entry := range unbound {
		same := bound[rec.FetchString(entry, "path")]
		use := rec.FetchInt(entry, "use")
		repainted := func(other rec.Map) bool { return v.svg.repaintedUse(entry, other) }
		if stacked[use] || merged[use] || overprinted(entry, same) || stackedFontCompanion(entry, all) || slices.ContainsFunc(same, func(other rec.Map) bool {
			return repaintedBlank(entry, other) || v.svg.strokedUse(entry, other, v.usePaint[rec.FetchInt(other, "use")]) || v.svg.hiddenUse(entry, other)
		}) || slices.ContainsFunc(copies[v.svg.outline(entry)], repainted) {
			continue
		}
		rec.Fail("SVG glyph placement coverage differs")
	}
}

// repaintedBlank: an outline-less glyph (a space) placed twice at one spot paints nothing.
func repaintedBlank(entry, other rec.Map) bool {
	return entry["status"] == "empty_outline" && entry["path"] == other["path"] &&
		Close(rec.FetchList(entry, "matrix"), rec.FetchList(other, "matrix"))
}

func overprinted(entry rec.Map, bound []rec.Map) bool {
	matrix := rec.FetchList(entry, "matrix")
	scale, shift := matrix[:4], matrix[4:]
	for _, other := range bound {
		otherMatrix := rec.FetchList(other, "matrix")
		otherScale, otherShift := otherMatrix[:4], otherMatrix[4:]
		if !Close(scale, otherScale) || Close(shift, otherShift) {
			continue
		}
		near := true
		for index := range shift {
			a, _ := rec.Number(shift[index])
			b, _ := rec.Number(otherShift[index])
			near = near && abs(a-b) <= overprint
		}
		if near {
			return true
		}
	}
	return false
}
