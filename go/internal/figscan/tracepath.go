package figscan

import (
	"encoding/json"
	"math"

	"github.com/sarielhp/relatex/internal/nativetrace"
)

const (
	// fitTolerance is the largest radial error, as a share of the radius, of a Bézier loop read as
	// a circle or an ellipse.
	fitTolerance = 0.015
	// fitMinCurves and fitMaxCurves bound the cubic segments of a loop read as an ellipse.
	fitMinCurves = 4
	fitMaxCurves = 8
	// closeEnough is the distance (normalized) at which a subpath's end meets its start.
	closeEnough = 0.05
)

// traceSubpath collects one subpath of a traced path, in normalized coordinates.
type traceSubpath struct {
	points  []Point // the on-curve vertices
	samples []Point // points along the curves, for the ellipse fit
	lines   int
	curves  int
	closed  bool
}

// path reads a fill_path or stroke_path; a stroke of the shape just filled joins its node.
func (r *traceReader) path(e *nativetrace.Node, nodes []*Node) []*Node {
	ctm := parseMatrix(e.Attrs["transform"])
	parts := traceParts(e.Children, ctm.then(r.toBox), e.Name == "fill_path")
	if len(parts) == 0 {
		return nodes
	}
	r.paints++
	name, contrast := traceColor(e.Attrs["color"])
	alpha := traceAlpha(e.Attrs["alpha"])
	node := &Node{Kind: KindPath, Parts: parts}
	if e.Name == "fill_path" && !flat(parts) {
		node.Fill, node.fillShade = name, contrast*alpha
		if alpha < 1 {
			node.Opacity = round1(alpha*100) / 100
		}
		return append(nodes, node)
	}
	if e.Name == "fill_path" {
		return nodes
	}
	target := node
	if last := lastFill(nodes); last != nil && sameParts(last.Parts, parts) {
		target = last
	}
	target.Stroke, target.contrast = name, contrast*alpha
	target.Pen = math.Round(traceFloat(e.Attrs["linewidth"])*math.Sqrt(math.Abs(ctm.determinant()))*1000) / 1000
	if e.Attrs["dash"] != "" {
		target.Dash = "dashed"
	}
	if target == node {
		nodes = append(nodes, node)
	}
	return nodes
}

// flat reports whether every part is an open line: a fill of it paints nothing.
func flat(parts []Part) bool {
	for _, part := range parts {
		if part.Closed || len(part.Points) > 2 {
			return false
		}
	}
	return true
}

func lastFill(nodes []*Node) *Node {
	if len(nodes) == 0 {
		return nil
	}
	last := nodes[len(nodes)-1]
	if last.Kind != KindPath || last.Fill == "" || last.Stroke != "" {
		return nil
	}
	return last
}

// sameParts reports whether a fill and a stroke trace the same outline (the fill closes it).
func sameParts(fill, stroke []Part) bool {
	open := make([]Part, len(fill))
	for i, part := range fill {
		part.Closed = stroke[min(i, len(stroke)-1)].Closed
		if part.Shape == ShapePolygon && !part.Closed {
			part.Shape = ShapePolyline
		}
		open[i] = part
	}
	left, _ := json.Marshal(open)
	right, _ := json.Marshal(stroke)
	return string(left) == string(right)
}

func traceFloat(text string) float64 {
	value, _ := parseNumber(text)
	return value
}

func traceAlpha(text string) float64 {
	if value, ok := parseNumber(text); ok {
		return math.Max(0, math.Min(1, value))
	}
	return 1
}

func parseNumber(text string) (float64, bool) {
	values := numbers(text)
	if len(values) != 1 {
		return 0, false
	}
	return values[0], true
}

// traceParts reads the path steps (moveto, lineto, curveto, closepath) into normalized parts; a
// filled path's subpaths are closed, as filling closes them.
func traceParts(steps []*nativetrace.Node, toBox matrix, filled bool) []Part {
	var parts []Part
	var sub *traceSubpath
	var current Point
	flush := func() {
		if sub != nil {
			if part, ok := sub.part(filled); ok {
				parts = append(parts, part)
			}
		}
		sub = nil
	}
	for _, step := range steps {
		points := stepPoints(step, toBox)
		switch step.Name {
		case "moveto":
			flush()
			sub = &traceSubpath{points: points, samples: points}
		case "lineto":
			sub = ensureSubpath(sub, current)
			sub.lines++
			sub.points = append(sub.points, points...)
			sub.samples = append(sub.samples, points...)
		case "curveto":
			sub = ensureSubpath(sub, current)
			sub.curve(current, points)
		case "closepath":
			if sub != nil {
				sub.closed = true
			}
		}
		if len(points) > 0 {
			current = points[len(points)-1]
		}
	}
	flush()
	return parts
}

func ensureSubpath(sub *traceSubpath, current Point) *traceSubpath {
	if sub == nil {
		return &traceSubpath{points: []Point{current}, samples: []Point{current}}
	}
	return sub
}

// stepPoints are a path step's points, normalized: the end point last.
func stepPoints(step *nativetrace.Node, toBox matrix) []Point {
	var names []string
	switch step.Name {
	case "moveto", "lineto":
		names = []string{"x", "y"}
	case "curveto":
		names = []string{"x1", "y1", "x2", "y2", "x3", "y3"}
	default:
		return nil
	}
	points := make([]Point, 0, len(names)/2)
	for i := 0; i < len(names); i += 2 {
		points = append(points, toBox.apply(traceFloat(step.Attrs[names[i]]), traceFloat(step.Attrs[names[i+1]])))
	}
	return points
}

// curve adds a cubic segment from p0 with control points c[0], c[1] and end c[2].
func (s *traceSubpath) curve(p0 Point, c []Point) {
	if len(c) != 3 {
		return
	}
	s.curves++
	for _, t := range []float64{0.25, 0.5, 0.75, 1} {
		u := 1 - t
		a, b, d, e := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		s.samples = append(s.samples, Point{float64(a*p0[0]) + float64(b*c[0][0]) + float64(d*c[1][0]) + float64(e*c[2][0]),
			float64(a*p0[1]) + float64(b*c[0][1]) + float64(d*c[1][1]) + float64(e*c[2][1])})
	}
	s.points = append(s.points, c[2])
}

// part is the subpath as a model part; false for a lone point.
func (s *traceSubpath) part(filled bool) (Part, bool) {
	if len(s.points) < 2 {
		return Part{}, false
	}
	first, last := s.points[0], s.points[len(s.points)-1]
	closed := s.closed || filled || (len(s.points) > 2 && math.Hypot(last[0]-first[0], last[1]-first[1]) < closeEnough)
	if closed && s.curves >= fitMinCurves && s.curves <= fitMaxCurves && s.lines == 0 {
		if part, ok := fitEllipse(s.samples); ok {
			return part, true
		}
	}
	points := make([]Point, len(s.points))
	for i, p := range s.points {
		points[i] = roundPoint(p)
	}
	points = withoutRepeats(points)
	if closed && len(points) > 2 && points[len(points)-1] == points[0] {
		points = points[:len(points)-1]
	}
	part := Part{Shape: ShapePolyline, Points: points, Closed: closed}
	switch {
	case s.curves > 0:
		part.Shape = ShapeCurve
	case closed:
		part.Shape = ShapePolygon
	}
	if part.Shape == ShapePolygon && len(points) < 3 {
		part.Shape, part.Closed = ShapePolyline, false
	}
	return simplifyPart(part), true
}

// fitEllipse reads a closed Bézier loop as an ellipse when every sample lies within fitTolerance
// of one: first axis-aligned (the samples' bounding box), then rotated (their second moments).
func fitEllipse(samples []Point) (Part, bool) {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range samples {
		minX, minY, maxX, maxY = math.Min(minX, p[0]), math.Min(minY, p[1]), math.Max(maxX, p[0]), math.Max(maxY, p[1])
	}
	center := Point{(minX + maxX) / 2, (minY + maxY) / 2}
	if part, ok := ellipseIfFits(samples, center, (maxX-minX)/2, (maxY-minY)/2, 0); ok {
		return part, true
	}
	return rotatedFit(samples)
}

// rotatedFit fits an ellipse by the samples' mean and covariance (a sampled ellipse has
// covariance radius²/2 along each axis).
func rotatedFit(samples []Point) (Part, bool) {
	var cx, cy float64
	for _, p := range samples {
		cx, cy = cx+p[0], cy+p[1]
	}
	n := float64(len(samples))
	cx, cy = cx/n, cy/n
	var sxx, sxy, syy float64
	for _, p := range samples {
		dx, dy := p[0]-cx, p[1]-cy
		sxx, sxy, syy = sxx+float64(dx*dx), sxy+float64(dx*dy), syy+float64(dy*dy)
	}
	sxx, sxy, syy = sxx/n, sxy/n, syy/n
	root := math.Sqrt(float64((sxx-syy)*(sxx-syy)/4) + float64(sxy*sxy))
	major, minor := float64((sxx+syy)/2)+root, float64((sxx+syy)/2)-root
	angle := 0.5 * math.Atan2(2*sxy, sxx-syy)
	return ellipseIfFits(samples, Point{cx, cy}, math.Sqrt(2*major), math.Sqrt(2*math.Max(minor, 0)), angle)
}

// ellipseIfFits is the ellipse (radii a, b; first axis at angle radians) when the samples fit it.
func ellipseIfFits(samples []Point, center Point, a, b, angle float64) (Part, bool) {
	if a <= 0 || b <= 0 || math.Min(a, b) < 0.02*math.Max(a, b) {
		return Part{}, false
	}
	cos, sin := math.Cos(angle), math.Sin(angle)
	for _, p := range samples {
		dx, dy := p[0]-center[0], p[1]-center[1]
		u, v := float64(dx*cos)+float64(dy*sin), float64(-dx*sin)+float64(dy*cos)
		if math.Abs(math.Hypot(u/a, v/b)-1) > fitTolerance {
			return Part{}, false
		}
	}
	degrees := angle * 180 / math.Pi
	if b > a {
		a, b, degrees = b, a, degrees+90
	}
	degrees = math.Mod(degrees+360, 180)
	if round1(a) == round1(b) || round1(degrees) == 180 {
		degrees = 0
	}
	c := roundPoint(center)
	radii := [2]float64{round1(a), round1(b)}
	return Part{Shape: ShapeEllipse, Center: &c, Radii: &radii, Angle: round1(degrees)}, true
}

// traceColor names a traced colour (gray, RGB or CMYK components) and gives its contrast.
func traceColor(text string) (string, float64) {
	values := numbers(text)
	var rgb [3]float64
	switch len(values) {
	case 1:
		rgb = [3]float64{values[0], values[0], values[0]}
	case 3:
		rgb = [3]float64{values[0], values[1], values[2]}
	case 4:
		k := 1 - values[3]
		rgb = [3]float64{(1 - values[0]) * k, (1 - values[1]) * k, (1 - values[2]) * k}
	default:
		return "unknown", 1
	}
	if rgb[0] == rgb[1] && rgb[1] == rgb[2] {
		return numericColorName(rgb[:1]), contrastOf(rgb)
	}
	return numericColorName(rgb[:]), contrastOf(rgb)
}
