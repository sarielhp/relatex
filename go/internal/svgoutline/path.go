// Package svgoutline is the Go port of Relatex::SVGPathBounds, SVGTransform, SVGGlyphOutlines
// and the validation half of PDFGlyphOutlines: the glyph outlines of a page SVG and the
// check that a layout's glyphs are bound to them. Only native text uses participate; figure
// paths, control points and font bounding rectangles never substitute for a glyph outline.
package svgoutline

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
)

// number is SVGPathBounds::NUMBER.
var number = regexp.MustCompile(`\A[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?`)

var arity = map[string]int{"M": 2, "L": 2, "H": 1, "V": 1, "C": 6, "S": 4, "Q": 4, "T": 2, "Z": 0}

// rubyFloat is Kernel#Float on a token number matched: a dot must be followed by a digit.
func rubyFloat(token string) (float64, bool) {
	if dot := strings.IndexByte(token, '.'); dot >= 0 && (dot+1 >= len(token) || token[dot+1] < '0' || token[dot+1] > '9') {
		return 0, false
	}
	value, err := strconv.ParseFloat(token, 64)
	if err != nil && !math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// pathBounds is SVGPathBounds: the bounds of the transformed curve, rather than transformed
// local bounding-box corners. MuPDF outlines use M/L/H/V/C/Z; quadratic and smooth paths are
// also supported. An unrecognized command fails instead of returning control bounds.
type pathBounds struct {
	segments [][][2]float64
	position [2]float64
	start    *[2]float64
	control  *[2]float64
	previous string
	local    []float64
	measured bool
}

func newPathBounds(path string) *pathBounds {
	p := &pathBounds{}
	p.parse(path)
	return p
}

// bounds is SVGPathBounds#bounds; nil for an outline without segments.
func (p *pathBounds) bounds(matrix []float64) []float64 {
	if matrix[1] == 0 && matrix[2] == 0 {
		if !p.measured {
			p.local = p.curveBounds(geometry.Identity)
			p.measured = true
		}
		if p.local == nil {
			return nil
		}
		return geometry.TransformedBox(matrix, p.local)
	}
	return p.curveBounds(matrix)
}

func (p *pathBounds) curveBounds(matrix []float64) []float64 {
	xs, ys := []float64{}, []float64{}
	for _, segment := range p.segments {
		transformed := make([][2]float64, len(segment))
		for index, position := range segment {
			x, y := geometry.Point(matrix, position[0], position[1])
			transformed[index] = [2]float64{x, y}
		}
		for _, point := range extrema(transformed) {
			xs, ys = append(xs, point[0]), append(ys, point[1])
		}
	}
	if len(xs) == 0 {
		return nil
	}
	return geometry.Box([]any{minimum(xs), minimum(ys), maximum(xs), maximum(ys)})
}

func minimum(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func maximum(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

func isSpace(character byte) bool {
	return character == ' ' || character == '\t' || character == '\n' || character == '\v' || character == '\f' ||
		character == '\r' || character == ','
}

func isLetter(character byte) bool {
	return (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z')
}

func tokens(path string) []string {
	result := []string{}
	for index := 0; index < len(path); {
		if isSpace(path[index]) {
			index++
			continue
		}
		if isLetter(path[index]) {
			result = append(result, path[index:index+1])
			index++
			continue
		}
		token := number.FindString(path[index:])
		if token == "" {
			rec.Fail("Malformed SVG outline path")
		}
		result = append(result, token)
		index += len(token)
	}
	return result
}

func (p *pathBounds) parse(path string) {
	data := tokens(path)
	command := ""
	for index := 0; index < len(data); {
		if len(data[index]) == 1 && isLetter(data[index][0]) {
			command = data[index]
			index++
		}
		size, ok := arity[strings.ToUpper(command)]
		if !ok {
			rec.Fail("Unsupported SVG outline command")
		}
		values := []any{}
		for _, token := range data[index:min(index+size, len(data))] {
			value, valid := rubyFloat(token)
			if !valid {
				rec.Fail("Invalid SVG outline coordinate")
			}
			values = append(values, value)
		}
		p.draw(command, geometry.Numbers(values, size))
		index += size
		command = implicitCommand(command)
		if size > 0 && index > len(data) {
			rec.Fail("Incomplete SVG outline segment")
		}
	}
}

func implicitCommand(command string) string {
	switch command {
	case "Z", "z":
		return ""
	case "M":
		return "L"
	case "m":
		return "l"
	}
	return command
}

func (p *pathBounds) absolute(values []float64, relative bool) [][2]float64 {
	points := [][2]float64{}
	for index := 0; index+1 < len(values); index += 2 {
		point := [2]float64{values[index], values[index+1]}
		if relative {
			point = [2]float64{point[0] + p.position[0], point[1] + p.position[1]}
		}
		points = append(points, point)
	}
	return points
}

func (p *pathBounds) draw(command string, values []float64) {
	kind := strings.ToUpper(command)
	relative := command != kind
	if p.start == nil && kind != "M" {
		rec.Fail("SVG outline must begin with moveto")
	}
	var points [][2]float64
	if kind != "H" && kind != "V" && kind != "Z" {
		points = p.absolute(values, relative)
	}
	switch kind {
	case "M":
		start := points[0]
		p.start, p.position = &start, start
	case "L":
		p.segments = append(p.segments, [][2]float64{p.position, points[0]})
	case "H":
		x := values[0]
		if relative {
			x += p.position[0]
		}
		points = [][2]float64{{x, p.position[1]}}
	case "V":
		y := values[0]
		if relative {
			y += p.position[1]
		}
		points = [][2]float64{{p.position[0], y}}
	case "C", "Q":
		p.segments = append(p.segments, append([][2]float64{p.position}, points...))
	case "S", "T":
		p.segments = append(p.segments, append([][2]float64{p.position, p.reflected(kind)}, points...))
	case "Z":
		p.segments = append(p.segments, [][2]float64{p.position, *p.start})
	}
	if kind == "H" || kind == "V" {
		p.segments = append(p.segments, [][2]float64{p.position, points[0]})
	}
	p.control = nil
	if kind == "C" || kind == "Q" || kind == "S" || kind == "T" {
		last := p.segments[len(p.segments)-1]
		control := last[len(last)-2]
		p.control = &control
	}
	p.previous = kind
	if kind == "Z" {
		p.position = *p.start
	} else {
		p.position = points[len(points)-1]
	}
}

func (p *pathBounds) reflected(kind string) [2]float64 {
	compatible := p.previous == "Q" || p.previous == "T"
	if kind == "S" {
		compatible = p.previous == "C" || p.previous == "S"
	}
	if p.control == nil || !compatible {
		return p.position
	}
	return [2]float64{(2 * p.position[0]) - p.control[0], (2 * p.position[1]) - p.control[1]}
}

func extrema(points [][2]float64) [][2]float64 {
	if len(points) == 2 {
		return points
	}
	roots := []float64{}
	for axis := range 2 {
		values := make([]float64, len(points))
		for index, point := range points {
			values[index] = point[axis]
		}
		roots = append(roots, derivativeRoots(values)...)
	}
	result := [][2]float64{points[0], points[len(points)-1]}
	for _, time := range roots {
		if time > 0 && time < 1 {
			result = append(result, evaluate(points, time))
		}
	}
	return result
}

func derivativeRoots(values []float64) []float64 {
	if len(values) == 3 {
		return linearRoot(values[0]-(2*values[1])+values[2], values[1]-values[0])
	}
	a, b, c, d := values[0], values[1], values[2], values[3]
	return quadraticRoots(-a+float64(3*b)-float64(3*c)+d, 2*(a-float64(2*b)+c), b-a)
}

func linearRoot(slope, intercept float64) []float64 {
	if slope == 0 {
		return nil
	}
	return []float64{-intercept / slope}
}

func quadraticRoots(quadratic, linear, constant float64) []float64 {
	if quadratic == 0 {
		return linearRoot(linear, constant)
	}
	discriminant := float64(linear*linear) - float64(4*quadratic*constant)
	if discriminant < 0 {
		return nil
	}
	root := math.Sqrt(discriminant)
	if linear < 0 {
		root = -root
	}
	q := -0.5 * (linear + root)
	if q == 0 {
		return []float64{0.0}
	}
	return []float64{q / quadratic, constant / q}
}

func evaluate(points [][2]float64, time float64) [2]float64 {
	current := points
	for len(current) > 1 {
		next := make([][2]float64, len(current)-1)
		for index := range next {
			left, right := current[index], current[index+1]
			next[index] = [2]float64{float64(left[0]*(1-time)) + float64(right[0]*time), float64(left[1]*(1-time)) + float64(right[1]*time)}
		}
		current = next
	}
	return current[0]
}

// parseTransform is SVGTransform.parse.
func parseTransform(text string) []float64 {
	result := geometry.Identity
	for index := 0; index < len(text); {
		if isSpace(text[index]) {
			index++
			continue
		}
		start := index
		for index < len(text) && isLetter(text[index]) {
			index++
		}
		name := text[start:index]
		for index < len(text) && isWhitespace(text[index]) {
			index++
		}
		if name == "" || index >= len(text) || text[index] != '(' {
			rec.Fail("Malformed SVG transform")
		}
		var values []float64
		values, index = transformParameters(text, index+1)
		result = geometry.Compose(result, transformMatrix(name, values))
	}
	return result
}

func isWhitespace(character byte) bool { return isSpace(character) && character != ',' }

func transformParameters(text string, index int) ([]float64, int) {
	values := []any{}
	for {
		for index < len(text) && isSpace(text[index]) {
			index++
		}
		if index < len(text) && text[index] == ')' {
			index++
			break
		}
		token := number.FindString(text[index:])
		if token == "" {
			rec.Fail("Invalid SVG transform parameter")
		}
		value, ok := rubyFloat(token)
		if !ok {
			rec.Fail("Invalid SVG transform parameter")
		}
		values = append(values, value)
		index += len(token)
	}
	return geometry.Numbers(values, len(values)), index
}

func transformMatrix(name string, values []float64) []float64 {
	switch name {
	case "matrix":
		return geometry.Numbers(floats(values), 6)
	case "translate":
		transformArity(values, 1, 2)
		y := 0.0
		if len(values) > 1 {
			y = values[1]
		}
		return []float64{1, 0, 0, 1, values[0], y}
	case "scale":
		transformArity(values, 1, 2)
		y := values[0]
		if len(values) > 1 {
			y = values[1]
		}
		return []float64{values[0], 0, 0, y, 0, 0}
	case "rotate":
		return rotation(values)
	case "skewX", "skewY":
		transformArity(values, 1)
		tangent := math.Tan(values[0] * math.Pi / 180)
		if name == "skewX" {
			return []float64{1, 0, tangent, 1, 0, 0}
		}
		return []float64{1, tangent, 0, 1, 0, 0}
	}
	rec.Fail("Unsupported SVG transform")
	return nil
}

func floats(values []float64) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func transformArity(values []float64, allowed ...int) {
	for _, size := range allowed {
		if len(values) == size {
			return
		}
	}
	rec.Fail("SVG transform arity differs")
}

func rotation(values []float64) []float64 {
	transformArity(values, 1, 3)
	radians := values[0] * math.Pi / 180
	matrix := []float64{math.Cos(radians), math.Sin(radians), -math.Sin(radians), math.Cos(radians), 0, 0}
	if len(values) == 1 {
		return matrix
	}
	x, y := values[1], values[2]
	return geometry.Compose([]float64{1, 0, 0, 1, x, y}, geometry.Compose(matrix, []float64{1, 0, 0, 1, -x, -y}))
}
