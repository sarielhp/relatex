package assets

import (
	"errors"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/nativetrace"
)

// Decision 137: a picture TeX drew (a TikZ or PGF picture, cropped from the PDF) is shown as SVG
// whose drawing stays MuPDF's paths and whose labels are SVG text in the document's fonts.

// wordGap is the gap between two glyphs of a label, in em of the first, from which the label reads
// a space there: TeX's interword glue is about a third of an em, a kern a tenth at most.
const wordGap = 0.15

// sameScale is how far a glyph's vertical scale may differ from its horizontal one and still be
// written without a transform.
const sameScale = 0.001

// pictureGlyph is one glyph of a picture's display SVG (a <use data-text> outline) with what the
// PDF's trace says of it.
type pictureGlyph struct {
	node   *rxNode
	matrix [6]float64
	text   string
	font   string
	adv    float64
}

// tracedGlyph is one painted glyph of mutool's trace: its font, text and advance (em). Its origin is
// the outline's (the trace is in PDF space, the SVG in its own), which the renderings compare.
type tracedGlyph struct {
	font, text string
	adv        float64
	// first is the character the glyph's outline carries (MuPDF's data-text): text without the
	// characters a multi-character mapping adds (a ligature's).
	first string
}

// errNoText reports a picture without glyphs: nothing to convert.
var errNoText = errors.New("no text")

// tracedGlyphs is every glyph the page fills or strokes, in painting order. A glyph that maps to
// several characters (a ligature, a TeX size variant marked with a variation selector) is traced as
// its glyph and a continuation per further character (no glyph name, no advance, the same origin):
// the continuations' characters belong to the glyph before them.
func tracedGlyphs(page *nativetrace.Node) []tracedGlyph {
	glyphs := []tracedGlyph{}
	for _, child := range page.Children {
		if child.Name != "fill_text" && child.Name != "stroke_text" {
			glyphs = append(glyphs, tracedGlyphs(child)...)
			continue
		}
		for _, span := range child.Named("span") {
			var previous *nativetrace.Node // the span's glyph a continuation can belong to
			for _, glyph := range span.Named("g") {
				if continuation(glyph, previous) {
					glyphs[len(glyphs)-1].text += glyph.Attrs["unicode"]
					continue
				}
				glyphs = append(glyphs, tracedGlyph{font: span.Attrs["font"], text: glyph.Attrs["unicode"],
					adv: parseNumber(glyph.Attrs["adv"]), first: glyph.Attrs["unicode"]})
				previous = glyph
			}
		}
	}
	return glyphs
}

// continuation tells whether a traced glyph is a further character of the glyph before it in its
// span: no glyph name, no advance, at that glyph's origin.
func continuation(glyph, previous *nativetrace.Node) bool {
	if previous == nil {
		return false
	}
	_, named := glyph.Attrs["glyph"]
	return !named && parseNumber(glyph.Attrs["adv"]) == 0 && glyph.Attrs["x"] == previous.Attrs["x"] &&
		glyph.Attrs["y"] == previous.Attrs["y"]
}

func parseNumber(text string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return math.NaN()
	}
	return value
}

// textPicture is the display SVG with every painted glyph outline replaced by SVG text, or why it
// cannot be: a glyph the trace does not pair, one without text, a font no stack names.
func textPicture(svg []byte, traced []tracedGlyph, fonts pictureFonts) ([]byte, error) {
	root := parseREXML(string(svg))
	glyphs, err := pairGlyphs(paintedUses(root, nil), traced)
	if err != nil {
		return nil, err
	}
	raisePrimes(root, glyphs)
	byNode := map[*rxNode]*pictureGlyph{}
	for index := range glyphs {
		if !drawingFont(glyphs[index].font) {
			byNode[glyphs[index].node] = &glyphs[index]
		}
	}
	if len(byNode) == 0 {
		return nil, errNoText
	}
	if err = replaceGlyphs(root, byNode, fonts); err != nil {
		return nil, err
	}
	var out strings.Builder
	root.write(&out)
	return []byte(out.String()), nil
}

// paintedUses is every glyph outline the page paints: <use data-text> outside definitions
// (a clip path's glyphs clip, they are not read).
func paintedUses(node *rxNode, found []*rxNode) []*rxNode {
	for _, child := range node.children {
		if child.kind != "element" {
			continue
		}
		switch localName(child.name) {
		case "defs", "clipPath", "mask", "pattern", "symbol":
			continue
		case "use":
			if _, ok := child.attribute("data-text"); ok {
				found = append(found, child)
				continue
			}
		}
		found = paintedUses(child, found)
	}
	return found
}

// pairGlyphs pairs the SVG's glyph outlines with the trace's glyphs, in order: each pair of one
// text.
func pairGlyphs(uses []*rxNode, traced []tracedGlyph) ([]pictureGlyph, error) {
	if len(uses) != len(traced) {
		return nil, fmt.Errorf("trace: %d glyphs painted, %d outlines in the SVG", len(traced), len(uses))
	}
	glyphs := make([]pictureGlyph, len(uses))
	for index, use := range uses {
		raw, _ := use.attribute("transform")
		matrix, ok := parseMatrix(raw)
		if !ok {
			return nil, fmt.Errorf("glyph %d: transform %q", index+1, raw)
		}
		text, _ := use.attribute("data-text")
		text = html.UnescapeString(text)
		trace := traced[index]
		// The outline carries the glyph's first character; the text is all of the glyph's.
		if text != trace.first {
			return nil, fmt.Errorf("glyph %d: the SVG's %q at %g,%g is not the trace's %q", index+1, text, matrix[4], matrix[5], trace.first)
		}
		glyphs[index] = pictureGlyph{node: use, matrix: matrix, text: text, font: trace.font, adv: trace.adv}
	}
	return glyphs, nil
}

// parseMatrix reads "matrix(a,b,c,d,e,f)" (commas or spaces).
func parseMatrix(raw string) ([6]float64, bool) {
	var matrix [6]float64
	inner, found := strings.CutPrefix(strings.TrimSpace(raw), "matrix(")
	inner, closed := strings.CutSuffix(inner, ")")
	fields := strings.FieldsFunc(inner, func(r rune) bool { return r == ',' || r == ' ' })
	if !found || !closed || len(fields) != 6 {
		return matrix, false
	}
	for index, field := range fields {
		matrix[index] = parseNumber(field)
		if math.IsNaN(matrix[index]) {
			return matrix, false
		}
	}
	return matrix, true
}

// replaceGlyphs replaces each run of glyph outlines, siblings in one font on one baseline, with one
// SVG text element, in every element of the tree.
func replaceGlyphs(node *rxNode, glyphs map[*rxNode]*pictureGlyph, fonts pictureFonts) error {
	children := make([]*rxNode, 0, len(node.children))
	var run []*pictureGlyph
	flush := func() error {
		if len(run) == 0 {
			return nil
		}
		text, err := textRun(run, fonts)
		if err != nil {
			return err
		}
		children = append(children, text)
		run = nil
		return nil
	}
	for _, child := range node.children {
		// Layout whitespace between two glyphs of a run is dropped: the run's text holds its spaces.
		if len(run) > 0 && child.kind == "text" && strings.TrimSpace(child.text) == "" {
			continue
		}
		glyph, ok := glyphs[child]
		if ok && (len(run) == 0 || continues(run[len(run)-1], glyph)) {
			run = append(run, glyph)
			continue
		}
		if err := flush(); err != nil {
			return err
		}
		if ok {
			run = append(run, glyph)
			continue
		}
		if child.kind == "element" {
			if err := replaceGlyphs(child, glyphs, fonts); err != nil {
				return err
			}
		}
		children = append(children, child)
	}
	if err := flush(); err != nil {
		return err
	}
	node.children = children
	return nil
}

// continues: the glyph follows the run's last one in its font, size and turn, on its baseline and
// after it, with the same other attributes (colour, opacity).
func continues(last, glyph *pictureGlyph) bool {
	if last.font != glyph.font || paintAttributes(last.node) != paintAttributes(glyph.node) {
		return false
	}
	for index := range 4 {
		if math.Abs(last.matrix[index]-glyph.matrix[index]) > 1e-6 {
			return false
		}
	}
	along, across := baselineOffset(last.matrix, glyph.matrix)
	return along > 0 && math.Abs(across) <= 1e-3
}

// glyphSize is a glyph's size: the length of its baseline's unit.
func glyphSize(matrix [6]float64) float64 { return math.Hypot(matrix[0], matrix[1]) }

// baselineOffset is where a glyph's origin lies from another's, along that glyph's baseline and across it.
func baselineOffset(from, to [6]float64) (along, across float64) {
	size := glyphSize(from)
	dx, dy := to[4]-from[4], to[5]-from[5]
	ux, uy := from[0]/size, from[1]/size
	return float64(dx*ux) + float64(dy*uy), float64(ux*dy) - float64(uy*dx)
}

// uniform: a glyph set upright, unslanted and unmirrored, at one scale across and down (TeX's y-up
// outline on SVG's y-down page): its text needs no transform.
func uniform(matrix [6]float64) bool {
	return matrix[1] == 0 && matrix[2] == 0 && matrix[0] > 0 && math.Abs(-matrix[3]/matrix[0]-1) <= sameScale
}

// paintAttributes is a glyph outline's attributes other than its glyph and place, as text.
func paintAttributes(node *rxNode) string {
	parts := []string{}
	for _, attr := range node.attrs {
		switch localName(attr[0]) {
		case "href", "transform", "data-text", "x", "y":
			continue
		}
		parts = append(parts, attr[0]+"="+attr[1])
	}
	return strings.Join(parts, " ")
}

// textRun is one SVG text element for a run of glyphs: each glyph's character at its own origin,
// a space where the gap is a word space, in the run's face. An upright run is placed by x and y;
// another is turned (and scaled) by a transform from its first glyph's origin, its size the glyph's.
func textRun(run []*pictureGlyph, fonts pictureFonts) (*rxNode, error) {
	first := run[0]
	face, ok := fonts.face(first.font)
	if !ok {
		return nil, fmt.Errorf("font %s: no family names it", subsetTag.ReplaceAllString(first.font, ""))
	}
	text := &rxNode{kind: "element", name: "text"}
	for _, attr := range first.node.attrs {
		switch localName(attr[0]) {
		case "href", "transform", "data-text", "x", "y":
			continue
		}
		text.attrs = append(text.attrs, attr)
	}
	characters, positions, err := runCharacters(run, face)
	if err != nil {
		return nil, err
	}
	setFace(text, face)
	matrix := first.matrix
	size := glyphSize(matrix)
	text.setAttribute("font-size", formatNumber(size))
	start := 0.0
	if uniform(matrix) {
		start = matrix[4]
		text.setAttribute("y", formatNumber(matrix[5]))
	} else {
		text.setAttribute("transform", "matrix("+joinNumbers(matrix[0]/size, matrix[1]/size, -matrix[2]/size, -matrix[3]/size,
			matrix[4], matrix[5])+")")
		text.setAttribute("y", "0")
	}
	// Each character is a span at its own x (renderers such as librsvg read one x per span, not a list).
	for index, character := range []rune(characters) {
		span := &rxNode{kind: "element", name: "tspan", attrs: [][2]string{{"x", formatNumber(start + positions[index])}}}
		span.children = []*rxNode{{kind: "text", text: escapeText(string(character))}}
		text.children = append(text.children, span)
	}
	return text, nil
}

// runCharacters is the run's text and each character's distance along the baseline from the first
// glyph's origin: a glyph's characters start at its origin, a space between two glyphs at the first
// one's advance.
func runCharacters(run []*pictureGlyph, face pictureFace) (string, []float64, error) {
	var characters strings.Builder
	positions := []float64{}
	size := glyphSize(run[0].matrix)
	previousEnd := 0.0
	for index, glyph := range run {
		text := glyph.text
		if text == "" || strings.ContainsRune(text, 0xFFFD) || math.IsNaN(glyph.adv) {
			return "", nil, fmt.Errorf("glyph %q of %s: no character", text, subsetTag.ReplaceAllString(glyph.font, ""))
		}
		if face.alphabet != "" {
			text = mathLetters(text, face.alphabet)
		}
		origin, _ := baselineOffset(run[0].matrix, glyph.matrix)
		if index > 0 && origin-previousEnd > float64(wordGap*size) {
			characters.WriteByte(' ')
			positions = append(positions, previousEnd)
		}
		// A ligature's characters share its advance.
		count := float64(len([]rune(text)))
		for part := range []rune(text) {
			positions = append(positions, origin+float64(float64(part)*glyph.adv*size/count))
		}
		characters.WriteString(text)
		previousEnd = origin + float64(glyph.adv*size)
	}
	return characters.String(), positions, nil
}

func setFace(text *rxNode, face pictureFace) {
	text.setAttribute("font-family", face.family)
	if face.weight != "" {
		text.setAttribute("font-weight", face.weight)
	}
	if face.style != "" {
		text.setAttribute("font-style", face.style)
	}
	text.setAttribute("xml:space", "preserve")
}

func escapeText(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

// formatNumber is a coordinate to a ten-thousandth of a point.
func formatNumber(value float64) string {
	rounded := math.Round(value*10000) / 10000
	if rounded == 0 {
		rounded = 0
	}
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

func joinNumbers(values ...float64) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = formatNumber(value)
	}
	return strings.Join(parts, " ")
}
