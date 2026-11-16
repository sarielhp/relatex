package figscan

import (
	"math"
	"testing"
)

func sineLine(n int) []Point {
	points := make([]Point, n)
	for i := range points {
		x := float64(i) * 100 / float64(n-1)
		points[i] = Point{x, 50 + 20*math.Sin(x/10)}
	}
	return points
}

func TestSimplifyLongPolyline(t *testing.T) {
	long := sineLine(701)
	part := simplifyPart(Part{Shape: ShapeCurve, Points: long})
	if part.Original != 701 || len(part.Points) > simplifyLimit || len(part.Points) < 5 {
		t.Fatalf("original %d, kept %d", part.Original, len(part.Points))
	}
	for _, p := range long {
		if d := polylineDistance(p, part.Points); d > 3 {
			t.Fatalf("point %v is %.2f from the simplified line", p, d)
		}
	}
	if part.Points[0] != long[0] || part.Points[len(part.Points)-1] != long[700] {
		t.Fatal("end points moved")
	}
}

func TestSimplifyKeepsShortShapes(t *testing.T) {
	square := Part{Shape: ShapePolygon, Closed: true, Points: []Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}}}
	if got := simplifyPart(square); got.Original != 0 || len(got.Points) != 4 {
		t.Fatalf("short polygon changed: %+v", got)
	}
}

func TestSimplifyClosedKeepsCorners(t *testing.T) {
	corners := []Point{{0, 0}, {40, 0}, {40, 40}, {0, 40}}
	var points []Point
	for i, c := range corners {
		next := corners[(i+1)%len(corners)]
		for k := 0; k < 5; k++ {
			f := float64(k) / 5
			points = append(points, Point{c[0] + f*(next[0]-c[0]), c[1] + f*(next[1]-c[1])})
		}
	}
	part := simplifyPart(Part{Shape: ShapePolygon, Closed: true, Points: points})
	if part.Original != 20 || len(part.Points) != 4 {
		t.Fatalf("original %d, kept %v", part.Original, part.Points)
	}
}
