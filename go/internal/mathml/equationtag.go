package mathml

import (
	"regexp"
	"strings"
)

// trailingTag is an equation's printed number set as the last text of a display formula.
var trailingTag = regexp.MustCompile(`<mtext>\s*(\([^()<>\s]{1,16}\)|\[[^\[\]<>\s]{1,16}\])\s*</mtext>((?:</[a-z]+>)+)\z`)

// positionedTag is the same number laid out in positioned boxes: glyph by glyph, or as one run of
// text (a text box inside a formula reads as words, relatex-external-boxes.lua). Empty
// zero-width boxes may follow it: an overfull equation+split sets the number in a cell of its own,
// and the cell's empty boxes after the number are placeholders (papers2/0262).
var positionedTag = regexp.MustCompile(`((?:<mpadded [^>]*><mtext>[^<\s]{1,18}</mtext></mpadded>){1,18})` +
	`(?:<mpadded [^>]*><mspace [^>]*width="0(?:\.0+)?pt"[^>]*/></mpadded>)*((?:</[a-z]+>)+)\z`)

var (
	formulaToken = regexp.MustCompile(`<m(?:i|n|o)[ >]|<mtext>`)
	glyphText    = regexp.MustCompile(`<mtext>([^<]+)</mtext>`)
	tagText      = regexp.MustCompile(`\A(?:\([^()\s]{1,16}\)|\[[^\[\]\s]{1,16}\])\z`)
)

// SplitEquationTag separates the printed number a flush-left (fleqn) display carries inside
// its line box: the display formula ends with the number as an mtext (or as one positioned
// mtext per glyph). It returns the formula without the number and the number; a formula that
// is nothing but the number, or no display, is returned as it is with no number.
func SplitEquationTag(source string) (formula, tag string) {
	// A display row that carries its number as its label cell already has it: text that ends the
	// row (an \eqref in a \text{} annotation: amsldoc's "by (3.24)") is the formula's own.
	if !strings.Contains(source, `display="block"`) || strings.Contains(source, `intent=":equation-label"`) {
		return source, ""
	}
	if found := trailingTag.FindStringSubmatchIndex(source); found != nil {
		return keepRest(source, found[0], source[found[4]:found[5]], source[found[2]:found[3]])
	}
	found := positionedTag.FindStringSubmatchIndex(source)
	if found == nil {
		return source, ""
	}
	var glyphs strings.Builder
	for _, match := range glyphText.FindAllStringSubmatch(source[found[2]:found[3]], -1) {
		glyphs.WriteString(match[1])
	}
	if !tagText.MatchString(glyphs.String()) {
		return source, ""
	}
	return keepRest(source, found[0], source[found[4]:found[5]], glyphs.String())
}

// scriptClosers is a closing element that makes the last text a script or a part of a
// construct (an \underbrace label, a subscript, a fraction term): the number of a display is
// set in its line box, never inside one of these.
var scriptClosers = regexp.MustCompile(`</(?:munder|mover|munderover|msub|msup|msubsup|mmultiscripts|mfrac|msqrt|mroot)>`)

// keepRest drops the number at start: the formula keeps what precedes it and the closing
// elements after it; a formula that holds nothing but the number stays whole.
func keepRest(source string, start int, closers, tag string) (string, string) {
	if !formulaToken.MatchString(source[:start]) || scriptClosers.MatchString(closers) {
		return source, ""
	}
	return source[:start] + closers, tag
}
