package mathexport

import (
	"strings"
	"unicode"
)

// The collector ends a word of \text at a font kern: TeX's font shaping writes the kerning (and
// ligature) break as a discretionary that prints nothing, so \text{otherwise} reaches the page as
// two placed pieces, "other" and "wise", and a speech engine reads "other wise" (sample 81). A
// word space is a space character inside one piece and an explicit \kern is a space or nothing
// inside one piece, but \ , \quad, \hfill and a line's glue leave two pieces with no space
// character either, only a gap. Joining pieces across a gap would glue words ("by(3.24)"), so only
// a gap that is a kern joins. Joining changes no glyph and no space: MathML draws the text pieces
// of a row side by side where their placements put them.

// A piece's offset (lspace) is its x position in the box, not the gap before it, and its width is
// not written, so the gap between two pieces is the offset step less the left piece's advance.
// That advance is estimated from the letters (glyphEm) at the largest text size (textSizeBound,
// pt). A font kern is under kernBound of an em: the 0.08 em letter-kern bound of the word-spacing
// check (lib/relatex/draw7_word_check.rb), widened to 0.1 for the estimate's error; a word space
// is a quarter em and \quad a whole one.
const (
	textSizeBound = 12.0
	kernBound     = 0.1
)

// joinTextRuns joins the placed text pieces of one box that a font kern split.
func joinTextRuns(row *Node) {
	if row.Name != "mrow" || row.Parent == nil || row.Parent.Name != "mpadded" {
		return
	}
	kept := []*Node{}
	var previous *Node
	previousOffset := 0.0
	for _, child := range row.Elements() {
		text, offset, placed := placedText(child)
		if placed && previous != nil && kernBetween(tokenString(previous), tokenString(text), offset-previousOffset) {
			previous.SetText(tokenString(previous) + tokenString(text))
			continue
		}
		kept = append(kept, child)
		previous, previousOffset = text, offset
	}
	row.setElements(kept)
}

// kernBetween reports whether two pieces sit one against the other as a font kern leaves them: no
// space at the edges, no bracket against a word (a bracket is never kerned into a word without a
// space: "by" and "(3.24)"), and an offset step no wider than the left piece's advance and a kern.
func kernBetween(left, right string, step float64) bool {
	if edgeSpace(left, false) || edgeSpace(right, true) || bracketEdge(left, right) {
		return false
	}
	return step <= (glyphEm(left)+kernBound)*textSizeBound
}

// bracketEdge reports an opening bracket after, or a closing bracket before, a letter or digit.
func bracketEdge(left, right string) bool {
	leftRunes, rightRunes := []rune(left), []rune(right)
	last, first := leftRunes[len(leftRunes)-1], rightRunes[0]
	return wordRune(last) && strings.ContainsRune("([{", first) || strings.ContainsRune(")]}", last) && wordRune(first)
}

func wordRune(character rune) bool { return unicode.IsLetter(character) || unicode.IsDigit(character) }

// glyphEm estimates the advance of text in em from the class of each character.
func glyphEm(text string) float64 {
	total := 0.0
	for _, character := range text {
		switch {
		case strings.ContainsRune("ijlI.,;:'!|", character):
			total += 0.26
		case strings.ContainsRune("ftr()[]- ", character):
			total += 0.36
		case strings.ContainsRune("mMwW", character):
			total += 0.85
		case unicode.IsUpper(character):
			total += 0.64
		case unicode.IsDigit(character):
			total += 0.56
		default:
			total += 0.54
		}
	}
	return total
}

// placedText is a text piece the collector pinned in its box: an mpadded of zero size with a
// length offset holding one mtext, as its placed() writes every piece of a \text box. It gives
// the text and the offset.
func placedText(piece *Node) (*Node, float64, bool) {
	if piece.Name != "mpadded" {
		return nil, 0, false
	}
	for _, name := range []string{"width", "height", "depth"} {
		value, ok := pointValue(piece, name)
		if !ok || value != 0 {
			return nil, 0, false
		}
	}
	offset, ok := pointValue(piece, "lspace")
	if !ok {
		return nil, 0, false
	}
	inner := piece.Elements()
	if len(inner) != 1 || inner[0].Name != "mtext" || len(inner[0].Elements()) != 0 {
		return nil, 0, false
	}
	return inner[0], offset, true
}

// edgeSpace reports whether text begins (first) or ends with a space, or is empty: a word
// boundary a kern never stands at.
func edgeSpace(text string, first bool) bool {
	if text == "" {
		return true
	}
	runes := []rune(text)
	character := runes[0]
	if !first {
		character = runes[len(runes)-1]
	}
	return unicode.IsSpace(character)
}
