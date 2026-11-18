package mathexport

import "strings"

// A \left( ... \right) pair around content of ordinary height reaches the browser as stretchy
// fences, and Chromium draws a stretchy fence from a size variant with side bearings: "G ( m )"
// where TeX sets "G(m)". TeX itself takes the plain delimiter whenever the content is not taller
// than it, so such fences are marked stretchy="false" and the browser draws the plain glyph.
var (
	softContent = map[string]bool{"mi": true, "mn": true, "mo": true, "mtext": true, "mspace": true,
		"mrow": true, "msub": true, "msup": true, "msubsup": true, "mstyle": true, "mpadded": true}
	largeOperators = "∑∏∐∫∬∭∮∯∰⋀⋁⋂⋃⨀⨁⨂⨃⨄⨅⨆"
)

// softFences marks the fences of every \left...\right row with ordinary content as not stretchy.
func softFences(node *Node) {
	if node.Name == "mrow" {
		softRow(node.Elements())
	}
	for _, child := range node.Elements() {
		softFences(child)
	}
}

func softRow(children []*Node) {
	if len(children) < 2 {
		return
	}
	first, last := children[0], children[len(children)-1]
	if !texFence(first) || !texFence(last) {
		return
	}
	for _, child := range children[1 : len(children)-1] {
		if !softInside(child) {
			return
		}
	}
	first.SetAttribute("stretchy", "false")
	last.SetAttribute("stretchy", "false")
}

// texFence is a delimiter written by \left or \right: fence and symmetric, no explicit size.
func texFence(node *Node) bool {
	return node.Name == "mo" && node.Get("fence") == "true" && node.Get("symmetric") == "true" &&
		!node.Has("stretchy") && !node.Has("minsize") && !node.Has("maxsize")
}

func softInside(node *Node) bool {
	if !softContent[node.Name] {
		return false
	}
	if node.Name == "mo" {
		text, _ := node.TextValue()
		if strings.ContainsAny(text, largeOperators) {
			return false
		}
	}
	for _, child := range node.Elements() {
		if !softInside(child) {
			return false
		}
	}
	return true
}
