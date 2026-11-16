package pdftag

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"
)

// The document's character where the font spells a glyph otherwise (decision 148): such a glyph
// (spelling.go) is written inside a marked-content sequence of its own whose /ActualText is the
// document's character. A reader then reads the document's text, and so does the order check,
// which reads the marked content as a reader does (readGlyphs).

// actualText is an /ActualText marked-content sequence: its text, and its sequence number from 1
// in its content stream (0: none).
type actualText struct {
	sequence int
	text     string
}

// textState is the text state a show operator shows with: the font size, the horizontal scaling
// (Tz/100), the character and word spacing.
type textState struct {
	size, scale, charSpace, wordSpace float64
}

// openActual reads a BDC's /ActualText, numbering it; a BDC without one leaves the enclosing
// sequence's text in force.
func (in *interpreter) openActual(op operation) actualText {
	value, ok := propertyToken(op, "/ActualText")
	if !ok || (value.kind != tokString && value.kind != tokHex) {
		return in.currentActual()
	}
	in.sequences++
	return actualText{sequence: in.sequences, text: pdfTextString(stringBytes(value))}
}

// currentActual is the innermost open /ActualText.
func (in *interpreter) currentActual() actualText {
	if len(in.actuals) == 0 {
		return actualText{}
	}
	return in.actuals[len(in.actuals)-1]
}

// propertyToken is the value of key in a BDC's property dictionary.
func propertyToken(op operation, key string) (token, bool) {
	if len(op.operands) < 2 || op.operands[1].kind != tokDict {
		return token{}, false
	}
	l := &lexer{data: op.operands[1].raw[2 : len(op.operands[1].raw)-2]}
	previous := ""
	for {
		entry, err := l.next()
		if err != nil {
			return token{}, false
		}
		if previous == key {
			return entry, true
		}
		previous = ""
		if entry.kind == tokName {
			previous = string(entry.raw)
		}
	}
}

// actualTextBegin opens the marked-content sequence that gives one glyph the document's text, as a
// UTF-16BE text string.
func actualTextBegin(text string) string {
	var hex strings.Builder
	hex.WriteString("FEFF")
	for _, unit := range utf16.Encode([]rune(text)) {
		fmt.Fprintf(&hex, "%04X", unit)
	}
	return "/Span <</ActualText <" + hex.String() + ">>> BDC\n"
}

// spelledRole: the elements whose glyphs get the document's character, the text leaves the order
// check compares and the Links whose text is their host's.
func spelledRole(role string) bool { return textRoles[role] || role == "Link" }

// spelledAt is the document's character of a content ordinal's glyph where the font spells it
// otherwise, "" elsewhere.
func (t *tagger) spelledAt(page *prepared, ordinal int) string {
	if ordinal >= len(page.mapper.mapping) || page.mapper.mapping[ordinal] < 0 {
		return ""
	}
	return t.spelled[glyphRef{page.number, page.mapper.mapping[ordinal]}]
}

// readGlyphs is a page's painted glyphs as a reader reads the tagged PDF's content: a glyph inside
// an /ActualText sequence reads that text, the sequence's first glyph all of it and the others
// nothing (ISO 32000-2 14.9.4); the others read as the layout spells them.
func readGlyphs(items []item, glyphs []paintedGlyph, mapping []int) []paintedGlyph {
	var read []paintedGlyph
	seen := map[int]bool{}
	for _, entry := range items {
		if entry.actual.sequence == 0 || (entry.kind != kindShow && entry.kind != kindObject) {
			continue
		}
		if read == nil {
			read = slices.Clone(glyphs)
		}
		for ordinal := entry.first; ordinal < entry.first+entry.count && ordinal < len(mapping); ordinal++ {
			index := mapping[ordinal]
			if index < 0 || index >= len(read) {
				continue
			}
			read[index].text = ""
			if !seen[entry.actual.sequence] {
				read[index].text = entry.actual.text
				seen[entry.actual.sequence] = true
			}
		}
	}
	if read == nil {
		return glyphs
	}
	return read
}
