package pdfpaint

import (
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// classicTolerance is PDFTableGeometry::TOLERANCE_BP.
const classicTolerance = 0.002

var unsupportedClassic = []string{"clip_path", "clip_stroke_path", "clip_text", "clip_stroke_text", "clip_image_mask",
	"begin_group", "begin_mask", "begin_tile", "group", "mask", "tile"}

var whitespace = regexp.MustCompile(`\s`)

// bpDenominator is 65_536 * 72.27, multiplied at run time as Ruby does.
var bpDenominator = spPerPoint * texPointsPerInch

var spPerPoint, texPointsPerInch = 65_536.0, 72.27

// classicGeometry is PDFTableGeometry: an isolated classic table matched against
// independently interpreted PDF paint.
type classicGeometry struct {
	grid rec.Map
	page int64
}

func newClassicGeometry(grid rec.Map, page int64) *classicGeometry {
	tablemodel.ValidateShape(grid)
	return &classicGeometry{grid: grid, page: page}
}

type stroke struct {
	kind              string
	x1, x2, y1, y2    float64
	width             float64
	colorspace, color string
}

func (g *classicGeometry) verify(data []byte) (result rec.Map) {
	defer rescueArgument("Invalid PDF table paint: ")
	document, err := parseTree(data)
	if err != nil {
		rec.RaiseArgument("%s", firstLine(err.Error()))
	}
	pages := []int64{}
	for _, page := range document.Descendants("page") {
		pages = append(pages, rec.ToI(page.Attrs["number"]))
	}
	if !slices.Equal(pages, []int64{g.page}) {
		rec.Fail("Unexpected PDF paint page")
	}
	for _, name := range unsupportedClassic {
		if len(document.Descendants(name)) > 0 {
			rec.Fail("Unsupported clipped or composited table paint")
		}
	}
	native := []stroke{}
	for _, path := range document.Descendants("stroke_path") {
		native = append(native, strokeOf(path))
	}
	expected := g.expectedRules()
	var top *stroke
	for index := range expected {
		if expected[index].kind == "horizontal" && (top == nil || expected[index].y1 < top.y1) {
			top = &expected[index]
		}
	}
	if top == nil {
		rec.Fail("Table paint needs horizontal anchors")
	}
	matches := []rec.Map{}
	for _, anchor := range native {
		if found := g.tableMatch(document, native, expected, *top, anchor); found != nil {
			matches = append(matches, found)
		}
	}
	if len(matches) != 1 {
		rec.Fail("PDF table paint has missing or ambiguous geometry/text")
	}
	return matches[0]
}

// tableMatch: a page may contain other tables or horizontal rules; only a complete, unique
// geometry-and-text match establishes ownership.
func (g *classicGeometry) tableMatch(document *node, native, expected []stroke, top, anchor stroke) (result rec.Map) {
	if anchor.kind != "horizontal" || math.Abs(anchor.width-top.width) > classicTolerance ||
		math.Abs((anchor.x2-anchor.x1)-top.x2) > classicTolerance {
		return nil
	}
	defer rescueToNil(&result)
	origin := [2]float64{anchor.x1 - top.x1, anchor.y1 - top.y1}
	box := strokeBounds(expected)
	box = [4]float64{box[0] + origin[0], box[1] + origin[0], box[2] + origin[1], box[3] + origin[1]}
	rules := []stroke{}
	for _, rule := range native {
		if strokeIntersects(rule, box) {
			rules = append(rules, rule)
		}
	}
	if len(rules) != len(expected) {
		rec.Fail("PDF rule coverage differs from table")
	}
	errors := matchStrokes(expected, rules, origin)
	for _, rule := range rules {
		classicBlack(rule.colorspace, rule.color)
	}
	return rec.Map{"rules": int64(len(rules)), "glyphs": g.verifyText(document, rules), "maximum_error_bp": paintmodel.Maximum(errors)}
}

// rescueToNil is rescue Failure; nil.
func rescueToNil(result *rec.Map) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(rec.Failure); !ok || failure.Lookup {
		panic(recovered)
	}
	*result = nil
}

// strokeBounds is [x1 min, x2 max, y1 min, y2 max].
func strokeBounds(rules []stroke) [4]float64 {
	box := [4]float64{rules[0].x1, rules[0].x2, rules[0].y1, rules[0].y2}
	for _, rule := range rules[1:] {
		if rule.x1 < box[0] {
			box[0] = rule.x1
		}
		if rule.x2 > box[1] {
			box[1] = rule.x2
		}
		if rule.y1 < box[2] {
			box[2] = rule.y1
		}
		if rule.y2 > box[3] {
			box[3] = rule.y2
		}
	}
	return box
}

func strokeIntersects(rule stroke, box [4]float64) bool {
	margin := float64(rule.width/2) + classicTolerance
	return rule.x1 <= box[1]+margin && rule.x2 >= box[0]-margin && rule.y1 <= box[3]+margin && rule.y2 >= box[2]-margin
}

func classicBlack(space, value string) {
	if !black(space, floats(value)) {
		rec.Fail("Unsupported nonblack table paint")
	}
}

func (g *classicGeometry) verifyText(document *node, rules []stroke) int64 {
	box := strokeBounds(rules)
	native := ""
	glyphs := int64(0)
	for _, span := range spansOf(document) {
		text, count, ok := classicRun(span, box)
		if ok {
			native += text
			glyphs += count
		}
	}
	expected := ""
	for _, row := range rec.FetchList(g.grid, "rows") {
		for _, cell := range rec.AsList(row) {
			expected += rec.FetchString(rec.AsMap(cell), "text")
		}
	}
	native = whitespace.ReplaceAllString(native, "")
	expected = whitespace.ReplaceAllString(expected, "")
	if expected == "" || native != expected {
		rec.Fail("PDF table text coverage differs")
	}
	return glyphs
}

// spansOf is //fill_text/span.
func spansOf(document *node) []*node {
	result := []*node{}
	for _, text := range document.Descendants("fill_text") {
		result = append(result, text.Named("span")...)
	}
	return result
}

func classicRun(span *node, box [4]float64) (string, int64, bool) {
	parent := span.Parent
	transform := floats(parent.FetchAttr("transform"))
	if len(transform) != 6 {
		rec.Fail("Unsupported PDF text transform")
	}
	characters := []*node{}
	for _, glyph := range span.Children {
		x, y := rubyFloat(glyph.FetchAttr("x")), rubyFloat(glyph.FetchAttr("y"))
		px := float64(transform[0]*x) + float64(transform[2]*y) + transform[4]
		py := float64(transform[1]*x) + float64(transform[3]*y) + transform[5]
		if px >= box[0] && px <= box[1] && py >= box[2] && py <= box[3] {
			characters = append(characters, glyph)
		}
	}
	if len(characters) == 0 {
		return "", 0, false
	}
	if !identityPrefix(transform) {
		rec.Fail("Unsupported rotated table text")
	}
	if alpha, ok := parent.Attr("alpha"); ok && !opaque(alpha) {
		rec.Fail("Unsupported translucent table text")
	}
	classicBlack(parent.FetchAttr("colorspace"), parent.FetchAttr("color"))
	var text strings.Builder
	for _, glyph := range characters {
		text.WriteString(glyph.FetchAttr("unicode"))
	}
	return text.String(), int64(len(characters)), true
}

func strokeOf(path *node) stroke {
	transform := floats(path.FetchAttr("transform"))
	finite := len(transform) == 6
	for _, value := range transform {
		finite = finite && !math.IsInf(value, 0) && !math.IsNaN(value)
	}
	if !identityPrefix(transform) || !finite {
		rec.Fail("Unsupported ordinary PDF stroke transform")
	}
	if linecap, _ := path.Attr("linecap"); linecap != "0,0,0" {
		rec.Fail("Unsupported ordinary PDF stroke caps")
	}
	if alpha, ok := path.Attr("alpha"); ok && !opaque(alpha) {
		rec.Fail("Unsupported translucent ordinary PDF stroke")
	}
	if !slices.Equal(path.Names(), []string{"moveto", "lineto"}) {
		rec.Fail("Unsupported ordinary PDF stroke path")
	}
	xs, ys := []float64{}, []float64{}
	for _, command := range path.Children {
		xs = append(xs, rubyFloat(command.FetchAttr("x"))+transform[4])
		ys = append(ys, -rubyFloat(command.FetchAttr("y"))+transform[5])
	}
	x1, x2 := minmax(xs)
	y1, y2 := minmax(ys)
	if x1 != x2 && y1 != y2 {
		rec.Fail("Non-axis-aligned ordinary PDF stroke")
	}
	width := rubyFloat(path.FetchAttr("linewidth"))
	if math.IsInf(width, 0) || math.IsNaN(width) || !(width > 0) {
		rec.Fail("Unsupported PDF hairline or nonfinite stroke")
	}
	kind := "vertical"
	if y1 == y2 {
		kind = "horizontal"
	}
	return stroke{kind: kind, x1: x1, x2: x2, y1: y1, y2: y2, width: width,
		colorspace: path.FetchAttr("colorspace"), color: path.FetchAttr("color")}
}

// minmax is Array#minmax: the first of equal values.
func minmax(values []float64) (float64, float64) {
	low, high := values[0], values[0]
	for _, value := range values[1:] {
		if value < low {
			low = value
		}
		if value > high {
			high = value
		}
	}
	return low, high
}

func matchStrokes(expected, native []stroke, origin [2]float64) []float64 {
	unmatched := slices.Clone(native)
	errors := []float64{}
	for _, rule := range expected {
		positioned := rule
		positioned.x1, positioned.x2 = rule.x1+origin[0], rule.x2+origin[0]
		positioned.y1, positioned.y2 = rule.y1+origin[1], rule.y2+origin[1]
		candidates := []int{}
		for index, observed := range unmatched {
			if observed.kind == rule.kind && strokeClose(positioned, observed) {
				candidates = append(candidates, index)
			}
		}
		if len(candidates) != 1 {
			rec.Fail("Ordinary PDF rule geometry differs from model")
		}
		matched := unmatched[candidates[0]]
		unmatched = deleteStrokes(unmatched, matched)
		errors = append(errors, math.Abs(positioned.x1-matched.x1), math.Abs(positioned.x2-matched.x2),
			math.Abs(positioned.y1-matched.y1), math.Abs(positioned.y2-matched.y2), math.Abs(positioned.width-matched.width))
	}
	return errors
}

// deleteStrokes is Array#delete: every element equal to the matched one.
func deleteStrokes(list []stroke, matched stroke) []stroke {
	result := []stroke{}
	for _, item := range list {
		if item != matched {
			result = append(result, item)
		}
	}
	return result
}

func strokeClose(a, b stroke) bool {
	return math.Abs(a.x1-b.x1) <= classicTolerance && math.Abs(a.x2-b.x2) <= classicTolerance &&
		math.Abs(a.y1-b.y1) <= classicTolerance && math.Abs(a.y2-b.y2) <= classicTolerance &&
		math.Abs(a.width-b.width) <= classicTolerance
}

// bp is TableGridRules#bp.
func bp(value float64) float64 { return value * 72.0 / bpDenominator }

func spNumber(m rec.Map, key string) float64 {
	value, ok := rec.Number(rec.Fetch(m, key))
	if !ok {
		rec.Fail("undefined method '+' for %T", m[key])
	}
	return value
}

// expectedRules is TableGridRules#expected_rules: the classic grid's rule strokes in bp.
func (g *classicGeometry) expectedRules() []stroke {
	rules := []stroke{}
	borders := rec.FetchList(g.grid, "horizontal_borders")
	y := 0.0
	for rowIndex, item := range rec.FetchList(g.grid, "rows") {
		row := rec.AsList(item)
		x := 0.0
		for _, value := range row {
			cell := rec.AsMap(value)
			for _, entry := range rec.FetchList(cell, "borders") {
				border := rec.AsMap(entry)
				center := x + spNumber(border, "x") + float64(spNumber(border, "width")/2.0)
				rules = append(rules, stroke{kind: "vertical", x1: bp(center), x2: bp(center), y1: bp(y),
					y2: bp(y + spNumber(border, "height") + spNumber(border, "depth")), width: bp(spNumber(border, "width"))})
			}
			x += spNumber(cell, "width_sp")
		}
		first := rec.AsMap(rec.Index(row, 0))
		y += spNumber(first, "height_sp") + spNumber(first, "depth_sp")
		for _, entry := range borders {
			border := rec.AsMap(entry)
			if rec.Equal(border["after_row"], int64(rowIndex+1)) {
				y += spNumber(border, "height")
			}
		}
	}
	for _, entry := range borders {
		rules = append(rules, g.horizontalRule(rec.AsMap(entry)))
	}
	return rules
}

func (g *classicGeometry) horizontalRule(border rec.Map) stroke {
	after := rec.FetchInt(border, "after_row")
	height := spNumber(border, "height")
	y := 0.0
	rows := rec.FetchList(g.grid, "rows")
	for _, item := range rows[:min(int(after), len(rows))] {
		first := rec.AsMap(rec.Index(rec.AsList(item), 0))
		y += spNumber(first, "height_sp") + spNumber(first, "depth_sp")
	}
	for _, entry := range rec.FetchList(g.grid, "horizontal_borders") {
		rule := rec.AsMap(entry)
		if other := rec.AsInt(rule["after_row"]); other > 0 && other < after {
			y += spNumber(rule, "height")
		}
	}
	if after == 0 {
		y += float64(-height / 2.0)
	} else {
		y += float64(height / 2.0)
	}
	return stroke{kind: "horizontal", x1: 0.0, x2: bp(spNumber(border, "width")), y1: bp(y), y2: bp(y), width: bp(height)}
}
