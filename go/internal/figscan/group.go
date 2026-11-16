package figscan

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	// minRun is the fewest alike siblings that collapse into one node.
	minRun = 3
	// maxMembers is the most member positions a collapsed node keeps.
	maxMembers = 40
	// smallShape is the largest extent (normalized units) of a shape that groups like a mark.
	smallShape = 3.0
)

// collapse replaces, at every level, the alike siblings (same kind, shape, size and style; marks
// and small closed shapes such as dots and little squares) by one node standing for all of them,
// placed where the first was. Fewer than minRun alike siblings stay as they are.
func collapse(nodes []*Node) []*Node {
	for _, n := range nodes {
		if n.Kind == KindGroup {
			n.Children = collapse(n.Children)
		}
	}
	nodes = mergeRepeats(nodes)
	runs := map[string][]*Node{}
	for _, n := range nodes {
		if key := signature(n); key != "" {
			runs[key] = append(runs[key], n)
		}
	}
	var out []*Node
	for _, n := range nodes {
		key := signature(n)
		members := runs[key]
		switch {
		case key == "" || len(members) < minRun:
			out = append(out, n)
		case members[0] == n:
			out = append(out, collapsed(members))
		}
	}
	return out
}

// mergeRepeats folds siblings that draw exactly the same thing (Ipe files often stack copies) into
// the first, counting the copies in Repeats.
func mergeRepeats(nodes []*Node) []*Node {
	first := map[string]*Node{}
	var out []*Node
	for _, n := range nodes {
		if n.Kind == KindGroup || n.Kind == KindText {
			out = append(out, n)
			continue
		}
		bare := *n
		bare.Layer = ""
		data, _ := json.Marshal(&bare)
		key := string(data)
		if earlier, ok := first[key]; ok {
			earlier.Repeats = max(earlier.Repeats, 1) + 1
			continue
		}
		first[key] = n
		out = append(out, n)
	}
	return out
}

// signature is what alike siblings share; "" for a node that never collapses.
func signature(n *Node) string {
	style := fmt.Sprintf("%s|%s|%g|%s|%g", n.Stroke, n.Fill, n.Pen, n.Dash, n.Opacity)
	if n.Kind == KindMark {
		return "mark|" + n.Shape + "|" + n.Size + "|" + style
	}
	if n.Kind != KindPath || len(n.Parts) != 1 || n.Arrows != "" {
		return ""
	}
	part := n.Parts[0]
	if !part.Closed {
		return ""
	}
	box := extent(outline(part))
	width, height := box[2]-box[0], box[3]-box[1]
	if math.Max(width, height) > smallShape {
		return ""
	}
	return fmt.Sprintf("small|%s|%.0f|%.0f|%s", part.Shape, width*2, height*2, style)
}

// collapsed is one node for alike members: their count, positions and extent.
func collapsed(members []*Node) *Node {
	first := members[0]
	shape := first.Shape
	if first.Kind == KindPath {
		shape = "small " + first.Parts[0].Shape
	}
	node := &Node{Kind: KindGroup, Layer: first.Layer, Shape: shape, Style: first.Style, Size: first.Size,
		Count: len(members)}
	centers := make([]Point, len(members))
	var contrast, fill float64
	for i, m := range members {
		centers[i] = center(m)
		contrast, fill = contrast+m.contrast, fill+m.fillShade
	}
	box := extent(centers)
	node.Box = &box
	node.contrast, node.fillShade = contrast, fill
	if len(members) <= maxMembers {
		node.Members = centers
	}
	return node
}

// center of a mark or a small shape.
func center(n *Node) Point {
	if n.Point != nil {
		return *n.Point
	}
	part := n.Parts[0]
	if part.Center != nil {
		return *part.Center
	}
	box := extent(part.Points)
	return roundPoint(Point{(box[0] + box[2]) / 2, (box[1] + box[3]) / 2})
}
