package mathexport

import (
	"slices"
	"strings"
)

// groupFences groups each matched pair of fixed fences with what it encloses in an mrow
// (decision 100). Only fences TeX fixed (stretchy="false", explicit lspace and rspace) pair, and
// only around operators that cannot stretch: their form and spacing then stay what they were.
func groupFences(row *Node) {
	children := row.Elements()
	pairs := fencePairs(children)
	// A row that is one pair already groups it.
	if closing, ok := pairs[0]; ok && closing == len(children)-1 && row.Name == "mrow" {
		delete(pairs, 0)
	}
	if len(pairs) == 0 {
		return
	}
	row.setElements(regroup(row, children, pairs, 0, len(children)))
}

// fencePairs are the matched pairs, opening index => closing index; a closing fence that
// matches no open one ends every pair still open.
func fencePairs(children []*Node) map[int]int {
	pairs := map[int]int{}
	open := []int{}
	for index, child := range children {
		switch fenceRole(child) {
		case "open":
			open = append(open, index)
		case "close":
			if len(open) == 0 || speechPairs[tokenString(children[open[len(open)-1]])] != tokenString(child) {
				open = open[:0]
				continue
			}
			opened := open[len(open)-1]
			open = open[:len(open)-1]
			if fixedInside(children[opened+1 : index]) {
				pairs[opened] = index
			}
		}
	}
	return pairs
}

func fenceRole(node *Node) string {
	if node.Name != "mo" || node.Get("stretchy") != "false" || !node.Has("lspace") || !node.Has("rspace") {
		return ""
	}
	text := tokenString(node)
	if _, ok := speechPairs[text]; ok {
		return "open"
	}
	for _, closing := range speechPairs {
		if closing == text {
			return "close"
		}
	}
	return ""
}

func fixedInside(nodes []*Node) bool {
	for _, node := range nodes {
		core := operatorCore(node)
		if core != nil && core.Get("stretchy") != "false" && strings.Contains(speechStretchy, tokenString(core)) {
			return false
		}
	}
	return true
}

func operatorCore(node *Node) *Node {
	for node != nil {
		if node.Name == "mo" {
			return node
		}
		if !baseWrapper(node) {
			return nil
		}
		node = node.Elements()[0]
	}
	return nil
}

func regroup(row *Node, children []*Node, pairs map[int]int, start, stop int) []*Node {
	result := []*Node{}
	for index := start; index < stop; {
		closing, ok := pairs[index]
		if !ok {
			result = append(result, children[index])
			index++
			continue
		}
		group := &Node{Name: "mrow", Parent: row}
		inner := append([]*Node{children[index]}, regroup(group, children, pairs, index+1, closing)...)
		group.setElements(append(inner, children[closing]))
		result = append(result, group)
		index = closing + 1
	}
	return result
}

// setElements replaces the node's children with these elements.
func (n *Node) setElements(elements []*Node) {
	n.Children = slices.Clone(elements)
	for _, child := range n.Children {
		child.Parent = n
	}
}
