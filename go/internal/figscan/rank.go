package figscan

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const (
	// budget is the most shape nodes (everything but labels) a geometry file lists.
	budget = 40
	// nearestCount is how many primitives each label lists.
	nearestCount = 3
	// markWeight is a mark's measure (a mark has no length or area worth comparing).
	markWeight = 8.0
	// areaWeight scales fill area (units²) against stroke length (units).
	areaWeight = 0.1
	// labelBonus is added to a primitive's measure for each label it is among the nearest of.
	labelBonus = 10.0
)

// finish groups alike siblings, numbers the nodes, lists the labels and applies the size rule.
func finish(figure *Figure) {
	figure.Nodes = collapse(figure.Nodes)
	number(figure.Nodes, new(int))
	all := flatten(figure.Nodes)
	texts, shapes := split(all)
	for _, n := range shapes {
		n.score = measure(n)
	}
	for _, label := range labels(texts, shapes) {
		for _, near := range label.Nearest {
			byID(shapes, near.ID).score += labelBonus
		}
	}
	figure.Total = countShapes(figure.Nodes)
	keep := chooseKept(figure.Nodes, shapes)
	figure.Nodes = prune(figure.Nodes, keep, &figure.Cut)
	figure.Shown = countShapes(figure.Nodes)
	_, kept := split(flatten(figure.Nodes))
	figure.Labels = labels(texts, kept)
	setGroupBoxes(figure.Nodes)
}

func number(nodes []*Node, next *int) {
	for _, n := range nodes {
		*next++
		n.ID = fmt.Sprintf("n%d", *next)
		number(n.Children, next)
	}
}

// flatten lists every node in preorder.
func flatten(nodes []*Node) []*Node {
	var all []*Node
	for _, n := range nodes {
		all = append(all, n)
		all = append(all, flatten(n.Children)...)
	}
	return all
}

// split separates the labels from the primitives (nodes that draw: no Ipe group).
func split(all []*Node) (texts, primitives []*Node) {
	for _, n := range all {
		switch {
		case n.Kind == KindText:
			texts = append(texts, n)
		case n.Kind != KindGroup || n.Count > 0:
			primitives = append(primitives, n)
		}
	}
	return texts, primitives
}

func byID(nodes []*Node, id string) *Node {
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// measure is a primitive's geometric weight: stroke length times contrast plus fill area times
// contrast; marks count a fixed amount each.
func measure(n *Node) float64 {
	if n.Count > 0 {
		return float64(markWeight*n.contrast) + n.fillShade
	}
	switch n.Kind {
	case KindMark:
		return markWeight * n.contrast
	case KindImage:
		return areaWeight * (n.Box[2] - n.Box[0]) * (n.Box[3] - n.Box[1])
	}
	total := 0.0
	for _, part := range n.Parts {
		total += float64(polylineLength(outline(part))*n.contrast) + float64(areaWeight*area(part)*n.fillShade)
	}
	return total
}

// labels lists each text with its nearest primitives (ties by node order).
func labels(texts, primitives []*Node) []Label {
	list := make([]Label, 0, len(texts))
	for _, t := range texts {
		label := Label{ID: t.ID, Text: t.Text, Math: t.Math, Printed: t.Printed, Note: t.PrintedNote, Point: *t.Point, Nearest: []Nearest{}}
		type candidate struct {
			node     *Node
			distance float64
		}
		var candidates []candidate
		for _, p := range primitives {
			if d := distance(*t.Point, p); !math.IsInf(d, 1) {
				candidates = append(candidates, candidate{p, round1(d)})
			}
		}
		slices.SortStableFunc(candidates, func(a, b candidate) int {
			if a.distance != b.distance {
				return compareFloat(a.distance, b.distance)
			}
			return compareID(a.node.ID, b.node.ID)
		})
		for _, c := range candidates[:min(nearestCount, len(candidates))] {
			label.Nearest = append(label.Nearest, Nearest{ID: c.node.ID, Kind: kindName(c.node), Distance: c.distance})
		}
		list = append(list, label)
	}
	return list
}

func compareFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// compareID orders node ids n1, n2 … by number.
func compareID(a, b string) int {
	var x, y int
	fmt.Sscanf(a, "n%d", &x)
	fmt.Sscanf(b, "n%d", &y)
	return x - y
}

// kindName is a primitive's kind as the labels section names it.
func kindName(n *Node) string {
	switch {
	case n.Count > 0:
		return fmt.Sprintf("%d %s", n.Count, plural(n.Shape))
	case n.Kind == KindPath && len(n.Parts) == 1 && n.Parts[0].Shape == ShapePolyline && len(n.Parts[0].Points) == 2:
		return "segment"
	case n.Kind == KindPath && len(n.Parts) == 1:
		return n.Parts[0].Shape
	case n.Kind == KindPath:
		return "path"
	case n.Kind == KindMark:
		return n.Shape + " mark"
	case n.Kind == KindImage && n.Shape != "":
		return n.Shape
	}
	return n.Kind
}

func plural(shape string) string {
	if shape == "segment" {
		return "segments"
	}
	if strings.HasPrefix(shape, "small ") {
		return shape + "s"
	}
	return shape + " marks"
}

// countShapes counts the nodes that are not labels (groups included).
func countShapes(nodes []*Node) int {
	count := 0
	for _, n := range nodes {
		if n.Kind != KindText {
			count++
		}
		count += countShapes(n.Children)
	}
	return count
}

// chooseKept picks the primitives the size rule keeps: all when the forest has at most budget
// shape nodes, else the highest measures (ties by order) while they and the groups holding them
// fit in the budget.
func chooseKept(roots, primitives []*Node) map[*Node]bool {
	keep := map[*Node]bool{}
	if countShapes(roots) <= budget {
		for _, n := range primitives {
			keep[n] = true
		}
		return keep
	}
	parents := map[*Node]*Node{}
	for _, n := range flatten(roots) {
		for _, child := range n.Children {
			parents[child] = n
		}
	}
	ranked := slices.Clone(primitives)
	slices.SortStableFunc(ranked, func(a, b *Node) int { return compareFloat(b.score, a.score) })
	used := 0
	for _, n := range ranked {
		needed := 1
		for p := parents[n]; p != nil && !keep[p]; p = parents[p] {
			needed++
		}
		if used+needed > budget {
			continue
		}
		used += needed
		for p := n; p != nil && !keep[p]; p = parents[p] {
			keep[p] = true
		}
	}
	return keep
}

// prune drops the primitives keep leaves out, and the groups left with nothing, counting what is
// cut at each level by kind. Labels always stay.
func prune(nodes []*Node, keep map[*Node]bool, cut *map[string]int) []*Node {
	var out []*Node
	for _, n := range nodes {
		if n.Kind == KindGroup && n.Count == 0 {
			n.Children = prune(n.Children, keep, &n.Cut)
			if len(n.Children) > 0 {
				out = append(out, n)
				continue
			}
		}
		if n.Kind == KindText || keep[n] {
			out = append(out, n)
			continue
		}
		if *cut == nil {
			*cut = map[string]int{}
		}
		(*cut)[cutKind(n)]++
	}
	return out
}

func cutKind(n *Node) string {
	if n.Count > 0 {
		return "alike runs"
	}
	return n.Kind + "s"
}

func setGroupBoxes(nodes []*Node) {
	for _, n := range nodes {
		if n.Kind != KindGroup || n.Count > 0 {
			continue
		}
		setGroupBoxes(n.Children)
		if points := nodePoints(n); len(points) > 0 {
			box := extent(points)
			n.Box = &box
		}
	}
}
