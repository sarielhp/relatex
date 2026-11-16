package figscan

import (
	"math"
	"strconv"
	"strings"
)

// Part shapes.
const (
	ShapePolygon  = "polygon"
	ShapePolyline = "polyline"
	ShapeEllipse  = "ellipse"
	ShapeArc      = "arc"
	ShapeSpline   = "spline"
	// ShapeCurve is a subpath mixing arcs with lines or Bézier segments.
	ShapeCurve = "curve"
)

// subpath collects one subpath of an Ipe path while it is read.
type subpath struct {
	points []Point // in the path's own coordinates
	arcs   []matrix
	lines  int
	curved bool
	closed bool
}

// pathReader reads Ipe's path syntax (m l c q e a s u h, and 7.2's C and L) into parts mapped by
// the matrix toBox (path coordinates to normalized box coordinates).
type pathReader struct {
	toBox   matrix
	parts   []Part
	current *subpath
	stack   []float64
	// star is the stack length at a "*" marker (-1 without one): a spiro spline's knots end there
	// and its precomputed Bézier control points follow (Shape::load).
	star int
}

// parsePath reads an Ipe path's text into normalized parts.
func parsePath(text string, toBox matrix) []Part {
	reader := &pathReader{toBox: toBox, star: -1}
	for _, field := range strings.Fields(text) {
		if value, err := strconv.ParseFloat(field, 64); err == nil {
			reader.stack = append(reader.stack, value)
			continue
		}
		if field == "*" {
			reader.star = len(reader.stack)
			continue
		}
		reader.operator(field)
		reader.stack, reader.star = reader.stack[:0], -1
	}
	reader.finish()
	return reader.parts
}

func (r *pathReader) operator(op string) {
	switch op {
	case "m":
		r.finish()
		r.current = &subpath{}
		r.addPairs()
	case "l":
		r.ensure()
		r.current.lines++
		r.addPairs()
	case "c", "q", "s", "C", "L":
		if op == "L" && r.star >= 0 {
			r.stack = r.stack[:r.star]
		}
		r.ensure()
		r.current.curved = true
		r.addPairs()
	case "u":
		// A closed spline is a subpath of its own, never part of the one before.
		r.finish()
		r.ensure()
		r.current.curved, r.current.closed = true, true
		r.addPairs()
		r.finish()
	case "e":
		r.finish()
		r.ellipse()
	case "a":
		r.arc()
	case "h":
		if r.current != nil {
			r.current.closed = true
		}
		r.finish()
	}
}

func (r *pathReader) ensure() {
	if r.current == nil {
		r.current = &subpath{}
	}
}

func (r *pathReader) addPairs() {
	for i := 0; i+1 < len(r.stack); i += 2 {
		r.current.points = append(r.current.points, Point{r.stack[i], r.stack[i+1]})
	}
}

func (r *pathReader) ellipse() {
	if len(r.stack) < 6 {
		return
	}
	full := matrix(r.stack[len(r.stack)-6:]).then(r.toBox)
	center := roundPoint(Point{full[4], full[5]})
	major, minor, angle := full.axes()
	radii := [2]float64{round1(major), round1(minor)}
	r.parts = append(r.parts, Part{Shape: ShapeEllipse, Closed: true, Center: &center, Radii: &radii, Angle: round1(angle)})
}

func (r *pathReader) arc() {
	if len(r.stack) < 8 || r.current == nil || len(r.current.points) == 0 {
		return
	}
	values := r.stack[len(r.stack)-8:]
	r.current.arcs = append(r.current.arcs, matrix(values[:6]))
	r.current.points = append(r.current.points, Point{values[6], values[7]})
}

// finish turns the subpath read so far into a part.
func (r *pathReader) finish() {
	sub := r.current
	r.current = nil
	if sub == nil || len(sub.points) == 0 {
		return
	}
	if len(sub.arcs) == 1 && sub.lines == 0 && !sub.curved && len(sub.points) == 2 {
		r.parts = append(r.parts, r.arcPart(sub.arcs[0], sub.points[0], sub.points[1]))
		return
	}
	part := Part{Closed: sub.closed, Points: make([]Point, len(sub.points))}
	for i, p := range sub.points {
		part.Points[i] = roundPoint(r.toBox.apply(p[0], p[1]))
	}
	switch {
	case len(sub.arcs) > 0:
		part.Shape = ShapeCurve
	case sub.curved:
		part.Shape = ShapeSpline
	case sub.closed:
		part.Shape = ShapePolygon
	default:
		part.Shape = ShapePolyline
	}
	if part.Shape == ShapePolygon || part.Shape == ShapePolyline {
		part.Points = withoutRepeats(part.Points)
	}
	r.parts = append(r.parts, part)
}

// withoutRepeats drops vertices equal (after rounding) to the one before: a doubled click in
// the editor, not a shape.
func withoutRepeats(points []Point) []Point {
	out := points[:1]
	for _, p := range points[1:] {
		if p != out[len(out)-1] {
			out = append(out, p)
		}
	}
	return out
}

// arcPart is the arc of the ellipse e (unit circle image) from start counterclockwise to end, in
// the ellipse's own frame, as Ipe draws it.
func (r *pathReader) arcPart(e matrix, start, end Point) Part {
	inverse := e.inverse()
	from, to := inverse.apply(start[0], start[1]), inverse.apply(end[0], end[1])
	t0, t1 := math.Atan2(from[1], from[0]), math.Atan2(to[1], to[0])
	sweep := math.Mod(t1-t0+4*math.Pi, 2*math.Pi)
	if sweep == 0 {
		sweep = 2 * math.Pi
	}
	middle := t0 + float64(sweep/2)
	mid := e.apply(math.Cos(middle), math.Sin(middle))
	full := e.then(r.toBox)
	center := roundPoint(Point{full[4], full[5]})
	through := roundPoint(r.toBox.apply(mid[0], mid[1]))
	major, minor, angle := full.axes()
	radii := [2]float64{round1(major), round1(minor)}
	return Part{Shape: ShapeArc, Center: &center, Radii: &radii, Angle: round1(angle), Through: &through,
		Points: []Point{roundPoint(r.toBox.apply(start[0], start[1])), roundPoint(r.toBox.apply(end[0], end[1]))}}
}
