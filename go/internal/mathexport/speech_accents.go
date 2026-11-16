package mathexport

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// An accent TeX cannot place over its base (a capital Greek letter, a bold letter in some font
// setups, a double accent) is set over an empty box of the base's width, and the base follows as
// the next sibling (decision 114, amendment 2026-11-10):
//
//	<mrow><mover accent='true'><mspace width='6.6pt'/><mo>˜</mo></mover>
//	  <mspace width='0pt' style='margin-inline-start:-6.6pt'/></mrow>  [optional <mspace/>]  <mi>k</mi>
//
// MathCAT reads that "empty with tilde above, k"; grouped with its base under a postfix intent it
// reads "k tilde". The accents in order inside to outside, joined with "-", name the intent
// (bar-dot:postfix): MathCAT speaks an unknown concept name with its hyphens as spaces, "bar dot".

// speechAccents are the accent marks TeX sets over an empty box and the name MathCAT speaks for
// them. A mark outside the table leaves its box alone.
var speechAccents = map[string]string{
	"ˇ": "check",
	"¯": "bar",
	"˜": "tilde",
	"^": "hat",
	"ˆ": "hat",
	"˙": "dot",
	"¨": "double-dot",
	"´": "acute",
	"`": "grave",
	"˘": "breve",
	"→": "vector",
}

// lengthPattern reads the number of a MathML length ("0pt", "-7.195pt", "0.5em").
var lengthPattern = regexp.MustCompile(`\A(-?\d+(?:\.\d+)?)`)

// accentNames hands out a formula's accent argument names: speech-a1, speech-a2... ApplyIntent's own
// names (a1, a2...) stay free.
type accentNames struct{ next int }

func (a *accentNames) take() string {
	a.next++
	return "speech-a" + strconv.Itoa(a.next)
}

// applyAccents groups each accent over an empty box with the base that follows it, a new mrow
// carrying the postfix intent; the box, the skipped spaces and the base keep their order and
// attributes.
func applyAccents(row *Node, names *accentNames) {
	children := row.Elements()
	result := []*Node{}
	for index := 0; index < len(children); index++ {
		base, stop, word := accentGroup(children, index)
		if base == nil {
			result = append(result, children[index])
			continue
		}
		wrapper := &Node{Name: "mrow", Parent: row}
		name := names.take()
		wrapper.SetAttribute("intent", word+":postfix($"+name+")")
		inner := accentedBase(base)
		wrapper.setElements(append(slices.Clone(children[index:stop]), inner))
		inner.SetAttribute("arg", name)
		if inner == base {
			result = append(result, wrapper)
		} else {
			base.setElements(append([]*Node{wrapper}, base.Elements()[1:]...))
			result = append(result, base)
		}
		index = stop
	}
	row.setElements(result)
}

// accentGroup is the accent box at index with the base that follows it: the index of the base, the
// last element to group, and the intent's accent name. No base (nil) for anything else.
func accentGroup(children []*Node, index int) (*Node, int, string) {
	word, ok := accentBox(children[index])
	if !ok {
		return nil, 0, ""
	}
	stop := index + 1
	for stop < len(children) && children[stop].Name == "mspace" {
		stop++
	}
	if stop == len(children) {
		return nil, 0, ""
	}
	base := children[stop]
	if base.Name == "mo" || base.Name == "mrow" {
		return nil, 0, ""
	}
	return base, stop, word
}

// accentedBase is the element the accent belongs to: a scripted base's own base (TeX sets
// \check\Lambda_G^{pos} with the check over the Λ and the scripts after it), else the base itself.
// The postfix intent names one argument, so it cannot cover a whole script element without dropping
// its scripts.
func accentedBase(base *Node) *Node {
	if !slices.Contains([]string{"msub", "msup", "msubsup"}, base.Name) || len(base.Elements()) == 0 {
		return base
	}
	first := base.Elements()[0]
	if first.Name == "mo" || first.Name == "mrow" {
		return base
	}
	return first
}

// accentBox: an mrow of an accent mover over an empty box and its compensating space(s). The name
// is the accents inside to outside, joined with - (bar-dot).
func accentBox(node *Node) (string, bool) {
	if node.Name != "mrow" {
		return "", false
	}
	elements := node.Elements()
	if len(elements) < 2 {
		return "", false
	}
	names, ok := accentChain(elements[0])
	if !ok {
		return "", false
	}
	for _, space := range elements[1:] {
		if space.Name != "mspace" || !compensatingSpace(space) {
			return "", false
		}
	}
	reversed := make([]string, len(names))
	for index, name := range names {
		reversed[len(names)-1-index] = name
	}
	return strings.Join(reversed, "-"), true
}

// accentChain: the accents of a mover, outer first, when its base is an empty box through any
// mstyle and nested accent mover; the empty box itself has no names.
func accentChain(node *Node) ([]string, bool) {
	switch node.Name {
	case "mspace":
		return nil, true
	case "mrow":
		for _, child := range node.Elements() {
			if child.Name != "mspace" {
				return nil, false
			}
		}
		return nil, len(node.Elements()) > 0
	case "mstyle":
		if len(node.Elements()) != 1 {
			return nil, false
		}
		return accentChain(node.Elements()[0])
	case "mover":
		return accentMover(node)
	}
	return nil, false
}

// accentMover: a mover accent (on the mover or its mark) whose decoration is a known accent and
// whose base is an empty box, possibly holding another accent mover.
func accentMover(node *Node) ([]string, bool) {
	base, mark := moverParts(node)
	if base == nil || mark == nil {
		return nil, false
	}
	if node.Get("accent") != "true" && mark.Get("accent") != "true" {
		return nil, false
	}
	name, known := speechAccents[tokenString(mark)]
	inner, ok := accentChain(base)
	if !known || !ok {
		return nil, false
	}
	return append([]string{name}, inner...), true
}

// moverParts is a mover's base and accent mark: exactly one element that is not the mo.
func moverParts(node *Node) (base, mark *Node) {
	for _, child := range node.Elements() {
		switch {
		case child.Name == "mo" && mark == nil:
			mark = child
		case child.Name != "mo" && base == nil:
			base = child
		default:
			return nil, nil
		}
	}
	return base, mark
}

// compensatingSpace: an mspace of width zero or less, projected (width 0 and a negative
// margin-inline-start) or raw (a negative width).
func compensatingSpace(node *Node) bool {
	if value := lengthPattern.FindStringSubmatch(node.Get("width")); value != nil {
		if number, err := strconv.ParseFloat(value[1], 64); err == nil {
			return number <= 0
		}
	}
	return strings.Contains(node.Get("style"), "margin-inline-start:-")
}
