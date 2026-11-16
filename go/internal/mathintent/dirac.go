package mathintent

import "slices"

// The Dirac notation the author's own macros fix (decision 114, amendment 2026-11-10): a ket is a
// bar then the state then an angle, a bra the angle then the state then the bar, and a braket both
// around a middle bar. Only a macro whose NAME says which one it is and whose BODY, sizers, a
// phantom and braces aside, is exactly that shape states a concept: a bare |x⟩ and the physics or
// braket packages' \ket (not kept macros) say nothing.

var (
	ketNames    = []string{"ket", "Ket"}
	braNames    = []string{"bra", "Bra"}
	braketNames = []string{"braket", "Braket"}
)

// diracConcept: the ket, bra or braket a macro's name and body state together, "" else.
func diracConcept(name string, macro Macro) Concept {
	switch {
	case macro.Parameters == 1 && slices.Contains(ketNames, name) && barShape(macro.Body, "⟩"):
		return Ket
	case macro.Parameters == 1 && slices.Contains(braNames, name) && barShape(macro.Body, "⟨"):
		return Bra
	case macro.Parameters == 2 && slices.Contains(braketNames, name) && braketShape(macro.Body):
		return InnerProduct
	}
	return ""
}

// barShape: the body is the closing angle and a bar on either side of #1 (⟩ after it for a ket,
// ⟨ before it for a bra), and nothing else.
func barShape(body, closing string) bool {
	kept := shapeTokens(body)
	if len(kept) != 3 || !kept[1].is('#', "1") {
		return false
	}
	if angleOf(kept[0]) == closing {
		return barOf(kept[2]) == "|"
	}
	return barOf(kept[0]) == "|" && angleOf(kept[2]) == closing
}

// braketShape: the body is ⟨ #1 | #2 ⟩.
func braketShape(body string) bool {
	kept := shapeTokens(body)
	if len(kept) != 5 || !kept[1].is('#', "1") || !kept[3].is('#', "2") {
		return false
	}
	return angleOf(kept[0]) == "⟨" && barOf(kept[2]) == "|" && angleOf(kept[4]) == "⟩"
}

// shapeTokens is a macro body read, with what a Dirac body may carry aside: the sizers (\left,
// \right, a lone \left., \bigl...), thin spaces, a \vphantom{...} group and braces.
func shapeTokens(body string) []token {
	kept := []token{}
	tokens := tokenize(body)
	for index := 0; index < len(tokens); index++ {
		item := tokens[index]
		switch {
		case item.kind == 'c' && slices.Contains(sizers, item.text):
		case item.kind == 'c' && spacing(item):
		case item.kind == 'c' && item.text == "vphantom":
			index = skipGroup(tokens, index+1)
		case item.is('x', ".") && index > 0 && tokens[index-1].is('c', "left"):
			// A lone \left. is no delimiter at all.
		case item.kind == '{', item.kind == '}':
		default:
			kept = append(kept, item)
		}
	}
	return kept
}

// skipGroup is the index of the '}' closing the group that opens at index (a command's argument),
// len(tokens) when it never closes.
func skipGroup(tokens []token, index int) int {
	for index < len(tokens) && tokens[index].kind == 'c' && spacing(tokens[index]) {
		index++
	}
	depth := 0
	for stop := index; stop < len(tokens); stop++ {
		switch tokens[stop].kind {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return stop
			}
		}
	}
	return len(tokens)
}

// angleOf is the angle a token typesets (⟨ or ⟩, or < and >), "" for anything else.
func angleOf(item token) string {
	switch {
	case item.is('c', "langle"), item.is('x', "⟨"), item.is('x', "<"):
		return "⟨"
	case item.is('c', "rangle"), item.is('x', "⟩"), item.is('x', ">"):
		return "⟩"
	}
	return ""
}
