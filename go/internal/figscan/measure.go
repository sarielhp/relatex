package figscan

import "math"

// ellipseSamples is how many points stand for an ellipse in distances.
const ellipseSamples = 32

// outline is a part as polylines in normalized coordinates: what distances and lengths use.
func outline(p Part) []Point {
	switch p.Shape {
	case ShapeEllipse:
		return ellipsePoints(*p.Center, *p.Radii, p.Angle)
	case ShapeArc:
		return []Point{p.Points[0], *p.Through, p.Points[1]}
	}
	if p.Closed && len(p.Points) > 1 {
		return append(append([]Point{}, p.Points...), p.Points[0])
	}
	return p.Points
}

func ellipsePoints(center Point, radii [2]float64, angle float64) []Point {
	theta := angle * math.Pi / 180
	cos, sin := math.Cos(theta), math.Sin(theta)
	points := make([]Point, 0, ellipseSamples+1)
	for i := 0; i <= ellipseSamples; i++ {
		t := 2 * math.Pi * float64(i) / ellipseSamples
		x, y := radii[0]*math.Cos(t), radii[1]*math.Sin(t)
		points = append(points, Point{center[0] + float64(x*cos) - float64(y*sin), center[1] + float64(x*sin) + float64(y*cos)})
	}
	return points
}

func polylineLength(points []Point) float64 {
	total := 0.0
	for i := 1; i < len(points); i++ {
		total += math.Hypot(points[i][0]-points[i-1][0], points[i][1]-points[i-1][1])
	}
	return total
}

// area of a closed part; 0 for an open one.
func area(p Part) float64 {
	if p.Shape == ShapeEllipse {
		return math.Pi * p.Radii[0] * p.Radii[1]
	}
	if !p.Closed || len(p.Points) < 3 {
		return 0
	}
	sum := 0.0
	for i, a := range p.Points {
		b := p.Points[(i+1)%len(p.Points)]
		sum += float64(a[0]*b[1]) - float64(b[0]*a[1])
	}
	return math.Abs(sum) / 2
}

// segmentDistance is the distance from p to the segment ab.
func segmentDistance(p, a, b Point) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	length := float64(dx*dx) + float64(dy*dy)
	t := 0.0
	if length > 0 {
		t = math.Max(0, math.Min(1, (float64((p[0]-a[0])*dx)+float64((p[1]-a[1])*dy))/length))
	}
	return math.Hypot(p[0]-a[0]-float64(t*dx), p[1]-a[1]-float64(t*dy))
}

func polylineDistance(p Point, points []Point) float64 {
	if len(points) == 1 {
		return math.Hypot(p[0]-points[0][0], p[1]-points[0][1])
	}
	best := math.Inf(1)
	for i := 1; i < len(points); i++ {
		best = math.Min(best, segmentDistance(p, points[i-1], points[i]))
	}
	return best
}

// distance from p to what a node draws; +Inf for a node that draws nothing measurable.
func distance(p Point, n *Node) float64 {
	best := math.Inf(1)
	for _, part := range n.Parts {
		best = math.Min(best, polylineDistance(p, outline(part)))
	}
	if n.Kind == KindMark && n.Point != nil {
		best = math.Min(best, polylineDistance(p, []Point{*n.Point}))
	}
	for _, member := range n.Members {
		best = math.Min(best, polylineDistance(p, []Point{member}))
	}
	if n.Box != nil && (n.Kind == KindImage || (n.Count > 0 && len(n.Members) == 0)) {
		b := n.Box
		best = math.Min(best, polylineDistance(p, []Point{{b[0], b[1]}, {b[2], b[1]}, {b[2], b[3]}, {b[0], b[3]}, {b[0], b[1]}}))
	}
	return best
}

// nodePoints are the points that bound what a node draws, for group extents.
func nodePoints(n *Node) []Point {
	var points []Point
	for _, part := range n.Parts {
		points = append(points, outline(part)...)
	}
	if n.Point != nil {
		points = append(points, *n.Point)
	}
	points = append(points, n.Members...)
	if n.Box != nil {
		points = append(points, Point{n.Box[0], n.Box[1]}, Point{n.Box[2], n.Box[3]})
	}
	for _, child := range n.Children {
		points = append(points, nodePoints(child)...)
	}
	return points
}
