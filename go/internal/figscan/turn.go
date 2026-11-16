package figscan

import (
	"fmt"
	"math"
	"slices"
)

// Turn turns the figure's geometry by quarter turns counterclockwise (1 to 3; anything else
// leaves it as it is), as the paper prints a figure included with \includegraphics[angle=…]
// (decision 123, amendment 3): every position, box, ellipse and the box's size move with it, so
// the geometry matches the picture as printed. Turned records it for the text form.
func (f *Figure) Turn(quarters int) {
	quarters = ((quarters % 4) + 4) % 4
	if quarters == 0 || f.Source == SourceNone {
		return
	}
	width, height := f.Extent[0], f.Extent[1]
	move := func(p Point) Point { return turnPoint(p, quarters, width, height) }
	turnNodes(f.Nodes, move, quarters)
	for index := range f.Labels {
		f.Labels[index].Point = move(f.Labels[index].Point)
	}
	if quarters%2 == 1 {
		f.Extent = [2]float64{height, width}
		f.Size = [2]float64{f.Size[1], f.Size[0]}
	}
	f.Turned = turnName(quarters)
}

// turnName names a turn as the paper prints it.
func turnName(quarters int) string {
	switch quarters {
	case 1:
		return "90 degrees counterclockwise"
	case 2:
		return "180 degrees"
	}
	return "90 degrees clockwise"
}

// turnPoint turns p in a width × height extent by quarter turns counterclockwise, into the turned
// extent with its origin at the bottom left again.
func turnPoint(p Point, quarters int, width, height float64) Point {
	switch quarters {
	case 1:
		return roundPoint(Point{height - p[1], p[0]})
	case 2:
		return roundPoint(Point{width - p[0], height - p[1]})
	}
	return roundPoint(Point{p[1], width - p[0]})
}

func turnNodes(nodes []*Node, move func(Point) Point, quarters int) {
	for _, n := range nodes {
		n.Parts = turnParts(n.Parts, move, quarters)
		n.Clip = turnParts(n.Clip, move, quarters)
		if n.Point != nil {
			turned := move(*n.Point)
			n.Point = &turned
		}
		if n.Box != nil {
			low, high := move(Point{n.Box[0], n.Box[1]}), move(Point{n.Box[2], n.Box[3]})
			n.Box = &[4]float64{min(low[0], high[0]), min(low[1], high[1]), max(low[0], high[0]), max(low[1], high[1])}
		}
		n.Members = movedPoints(n.Members, move)
		turnNodes(n.Children, move, quarters)
	}
}

// movedPoints are points moved, in a new slice (a slice may be shared), nil for nil.
func movedPoints(points []Point, move func(Point) Point) []Point {
	if points == nil {
		return nil
	}
	moved := make([]Point, len(points))
	for index, p := range points {
		moved[index] = move(p)
	}
	return moved
}

// turnParts are parts turned, in a new slice; nil for nil.
func turnParts(parts []Part, move func(Point) Point, quarters int) []Part {
	if parts == nil {
		return nil
	}
	parts = slices.Clone(parts)
	for index := range parts {
		part := &parts[index]
		part.Points = movedPoints(part.Points, move)
		if part.Center != nil {
			center := move(*part.Center)
			part.Center = &center
		}
		if part.Through != nil {
			through := move(*part.Through)
			part.Through = &through
		}
		if part.Radii != nil && part.Radii[0] != part.Radii[1] {
			part.Angle = round1(math.Mod(part.Angle+float64(90*quarters), 180))
		}
	}
	return parts
}

// turnedLine is the text form's note on a turned figure, "" for one printed as drawn.
func (f *Figure) turnedLine() string {
	if f.Turned == "" {
		return ""
	}
	return fmt.Sprintf("turned %s as the paper prints it; positions and picture are shown as printed\n", f.Turned)
}
