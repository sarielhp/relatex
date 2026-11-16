package figscan

// Polyline simplification (plan: polylines simplified to a dozen points): Douglas–Peucker on the
// normalized coordinates, deterministic, with the tolerance raised until the result is short.
const (
	// simplifyLimit is the most points a polyline, polygon or curve keeps.
	simplifyLimit = 12
	// simplifyTolerance is the first Douglas–Peucker tolerance, in units of the 0–100 box.
	simplifyTolerance = 0.5
	// simplifyGrowth multiplies the tolerance while the result is still too long.
	simplifyGrowth = 1.5
)

// simplifyPart shortens a part listing more than simplifyLimit points and records the original
// count. Shorter parts, and parts that are not point lists, are returned unchanged.
func simplifyPart(p Part) Part {
	switch p.Shape {
	case ShapePolyline, ShapePolygon, ShapeCurve:
	default:
		return p
	}
	if len(p.Points) <= simplifyLimit {
		return p
	}
	points := simplifyOpen(p.Points)
	if p.Closed {
		points = simplifyRing(p.Points)
	}
	p.Original = len(p.Points)
	p.Points = points
	return p
}

func simplifyOpen(points []Point) []Point {
	for tolerance := simplifyTolerance; ; tolerance *= simplifyGrowth {
		out := douglasPeucker(points, tolerance)
		if len(out) <= simplifyLimit {
			return out
		}
	}
}

// simplifyRing simplifies a closed part's points (the first is not repeated at the end): the
// first point and the one farthest from it anchor two open halves.
func simplifyRing(points []Point) []Point {
	far, best := 1, -1.0
	for i, p := range points {
		if d := sqDist(p, points[0]); d > best {
			far, best = i, d
		}
	}
	back := append(append([]Point{}, points[far:]...), points[0])
	for tolerance := simplifyTolerance; ; tolerance *= simplifyGrowth {
		first := douglasPeucker(points[:far+1], tolerance)
		second := douglasPeucker(back, tolerance)
		out := append(first, second[1:len(second)-1]...)
		if len(out) <= simplifyLimit {
			return out
		}
	}
}

func sqDist(a, b Point) float64 {
	return float64((a[0]-b[0])*(a[0]-b[0])) + float64((a[1]-b[1])*(a[1]-b[1]))
}

// douglasPeucker keeps the end points and every point that strays more than tolerance from the
// chord of its span, found recursively at the farthest point (the first of equals).
func douglasPeucker(points []Point, tolerance float64) []Point {
	keep := make([]bool, len(points))
	keep[0], keep[len(points)-1] = true, true
	type span struct{ from, to int }
	stack := []span{{0, len(points) - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		index, worst := -1, tolerance
		for i := s.from + 1; i < s.to; i++ {
			if d := segmentDistance(points[i], points[s.from], points[s.to]); d > worst {
				index, worst = i, d
			}
		}
		if index >= 0 {
			keep[index] = true
			stack = append(stack, span{s.from, index}, span{index, s.to})
		}
	}
	out := make([]Point, 0, len(points))
	for i, p := range points {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}
