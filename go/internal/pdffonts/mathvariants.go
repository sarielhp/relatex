package pdffonts

import (
	"encoding/binary"
	"slices"
)

// Glyphs with no character of their own (decision 118, amendment 1): an OpenType math font keeps
// size variants and assembly parts of delimiters, radicals and big operators as glyphs no cmap
// reaches. The MATH table's MathVariants says which encoded glyph each one belongs to, and GSUB's
// single and alternate substitutions say which glyph a variant form (ssty, flac, ...) replaces.

// derivation is one step back from a glyph to the glyph it is made from.
type derivation struct {
	from int
	part bool // an assembly part of from's construction, not a whole variant of it
}

// derivations reads the reverse map of a font: glyph -> the glyphs it derives from, by the MATH
// table's glyph constructions (variants and assembly parts) and GSUB single and alternate substitutions.
func derivations(math, gsub []byte) map[int][]derivation {
	found := map[int][]derivation{}
	readConstructions(math, found)
	readSubstitutions(gsub, found)
	return found
}

// u16 reads a big-endian 16-bit value, 0 beyond the data.
func u16(data []byte, at int) int {
	if at < 0 || at+2 > len(data) {
		return 0
	}
	return int(binary.BigEndian.Uint16(data[at:]))
}

// readConstructions reads MathVariants: for each covered base glyph (vertical and horizontal),
// its variant glyphs and the parts of its glyph assembly.
func readConstructions(math []byte, found map[int][]derivation) {
	if len(math) < 10 || u16(math, 0) != 1 {
		return
	}
	variants := int(binary.BigEndian.Uint16(math[8:]))
	if variants == 0 || variants+10 > len(math) {
		return
	}
	vertical, horizontal := u16(math, variants+6), u16(math, variants+8)
	bases := [][]int{coverage(math, variants+u16(math, variants+2)), coverage(math, variants+u16(math, variants+4))}
	for direction, count := range []int{vertical, horizontal} {
		for i := 0; i < count && i < len(bases[direction]); i++ {
			construction := u16(math, variants+10+2*(vertical*direction+i))
			if construction != 0 {
				readConstruction(math, variants+construction, bases[direction][i], found)
			}
		}
	}
}

// readConstruction reads one MathGlyphConstruction of a base glyph.
func readConstruction(math []byte, at, base int, found map[int][]derivation) {
	count := u16(math, at+2)
	for i := 0; i < count && at+4+4*i+2 <= len(math); i++ {
		found = addDerivation(found, u16(math, at+4+4*i), derivation{from: base})
	}
	if assembly := u16(math, at); assembly != 0 {
		parts := u16(math, at+assembly+4)
		for i := 0; i < parts && at+assembly+6+10*i+2 <= len(math); i++ {
			addDerivation(found, u16(math, at+assembly+6+10*i), derivation{from: base, part: true})
		}
	}
}

func addDerivation(found map[int][]derivation, glyph int, d derivation) map[int][]derivation {
	if glyph != d.from && !slices.Contains(found[glyph], d) {
		found[glyph] = append(found[glyph], d)
	}
	return found
}

// readSubstitutions reads the single (type 1) and alternate (type 3) substitutions of GSUB,
// type 7 extensions included: output glyph -> input glyph.
func readSubstitutions(gsub []byte, found map[int][]derivation) {
	if len(gsub) < 10 {
		return
	}
	list := int(binary.BigEndian.Uint16(gsub[8:]))
	for _, lookup := range offsets(gsub, list) {
		at := list + lookup
		kind := u16(gsub, at)
		for _, sub := range offsets(gsub, at+4) {
			table, subKind := at+sub, kind
			if kind == 7 && table+8 <= len(gsub) {
				subKind = u16(gsub, table+2)
				table += int(binary.BigEndian.Uint32(gsub[table+4:]))
			}
			switch subKind {
			case 1:
				readSingle(gsub, table, found)
			case 3:
				readAlternate(gsub, table, found)
			}
		}
	}
}

// readSingle reads a SingleSubstFormat1 (delta) or 2 (list) table.
func readSingle(gsub []byte, at int, found map[int][]derivation) {
	inputs := coverage(gsub, at+u16(gsub, at+2))
	switch u16(gsub, at) {
	case 1:
		delta := int(int16(u16(gsub, at+4)))
		for _, in := range inputs {
			addDerivation(found, (in+delta)&0xFFFF, derivation{from: in})
		}
	case 2:
		for i, in := range inputs {
			if i < u16(gsub, at+4) {
				addDerivation(found, u16(gsub, at+6+2*i), derivation{from: in})
			}
		}
	}
}

// readAlternate reads an AlternateSubstFormat1 table: each alternate derives from its input.
func readAlternate(gsub []byte, at int, found map[int][]derivation) {
	if u16(gsub, at) != 1 {
		return
	}
	inputs := coverage(gsub, at+u16(gsub, at+2))
	for i, set := range offsets(gsub, at+4) {
		if i >= len(inputs) {
			return
		}
		for j := 0; j < u16(gsub, at+set); j++ {
			addDerivation(found, u16(gsub, at+set+2+2*j), derivation{from: inputs[i]})
		}
	}
}

// maxDerivation is how many steps back a glyph is followed (variant of a variant of a substitution).
const maxDerivation = 4

// derivedText reads a glyph without a character by the glyphs it derives from: exact only. Every
// route back must end at an encoded glyph and all must read the same text. A glyph reached only as an
// assembly part reads nothing: a stacked delimiter is drawn as several part glyphs (top, extenders,
// bottom), and giving each the base character would read "(((" for one parenthesis, while which
// single part to name is not recoverable from the glyph (an extender is shared by many bases).
// The reason is returned when there is no text.
func (o *originalFont) derivedText(gid, depth int) (text, why string) {
	routes := o.derived[gid]
	if len(routes) == 0 || depth >= maxDerivation {
		return "", ""
	}
	whole := slices.ContainsFunc(routes, func(d derivation) bool { return !d.part })
	if !whole {
		return "", "the matching glyph is only an assembly part of a larger delimiter or operator"
	}
	for _, route := range routes {
		found := o.exactText(route.from)
		if found == "" {
			found, _ = o.derivedText(route.from, depth+1)
		}
		switch {
		case found == "":
			return "", "the matching glyph derives from a glyph with no character"
		case text != "" && found != text:
			return "", "the matching glyph derives from several different characters"
		}
		text = found
	}
	return text, ""
}
