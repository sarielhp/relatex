package pdffonts

import (
	"encoding/binary"
	"errors"
	"maps"
	"slices"
)

// TrueType is the part of an embedded TrueType program that tells which character a glyph stands
// for: its cmap subtables (code -> glyph, by platform and encoding id) and its post table's glyph
// names (format 2 only). Hinting and outlines are not read.
type TrueType struct {
	Cmaps map[[2]int]map[int]int // (platform, encoding) -> code -> glyph
	Names []string               // post format 2: a glyph's name by glyph id, nil without
	Glyph int                    // the glyph count (maxp), 0 when unknown
}

var errTrueType = errors.New("truetype: malformed program")

// ParseTrueType reads the cmap and post tables of a TrueType program. A subtable of a format it
// does not read (2, 8, 10, 13, 14) is skipped; a program with no readable cmap and no names is
// an error only when its table directory is broken.
func ParseTrueType(data []byte) (*TrueType, error) {
	if len(data) < 12 {
		return nil, errTrueType
	}
	tables := tableDirectory(data)
	font := &TrueType{Cmaps: map[[2]int]map[int]int{}}
	if maxp := tables["maxp"]; len(maxp) >= 6 {
		font.Glyph = int(binary.BigEndian.Uint16(maxp[4:]))
	}
	if cmap := tables["cmap"]; len(cmap) >= 4 {
		readCmapTable(font, cmap)
	}
	if post := tables["post"]; len(post) >= 34 && binary.BigEndian.Uint32(post) == 0x00020000 {
		font.Names = readPostNames(post)
	}
	if len(tables) == 0 {
		return nil, errTrueType
	}
	return font, nil
}

// tableDirectory slices the tables of an sfnt by tag; one lying outside the data is dropped.
func tableDirectory(data []byte) map[string][]byte {
	tables := map[string][]byte{}
	count := int(binary.BigEndian.Uint16(data[4:]))
	for i := 0; i < count && 12+16*i+16 <= len(data); i++ {
		entry := data[12+16*i:]
		offset, length := int(binary.BigEndian.Uint32(entry[8:])), int(binary.BigEndian.Uint32(entry[12:]))
		if offset >= 0 && length >= 0 && offset <= len(data) && length <= len(data)-offset {
			tables[string(entry[:4])] = data[offset : offset+length]
		}
	}
	return tables
}

// readCmapTable reads every subtable of a cmap table.
func readCmapTable(font *TrueType, cmap []byte) {
	count := int(binary.BigEndian.Uint16(cmap[2:]))
	for i := 0; i < count && 4+8*i+8 <= len(cmap); i++ {
		record := cmap[4+8*i:]
		key := [2]int{int(binary.BigEndian.Uint16(record)), int(binary.BigEndian.Uint16(record[2:]))}
		offset := int(binary.BigEndian.Uint32(record[4:]))
		if _, seen := font.Cmaps[key]; seen || offset < 0 || offset+4 > len(cmap) {
			continue
		}
		if codes := readSubtable(cmap[offset:]); codes != nil {
			font.Cmaps[key] = codes
		}
	}
}

// readSubtable reads a cmap subtable of format 0, 4, 6 or 12; nil for another format or a
// truncated one.
func readSubtable(table []byte) map[int]int {
	codes := map[int]int{}
	switch binary.BigEndian.Uint16(table) {
	case 0:
		if len(table) < 262 {
			return nil
		}
		for code, glyph := range table[6:262] {
			if glyph != 0 {
				codes[code] = int(glyph)
			}
		}
	case 4:
		return readFormat4(table, codes)
	case 6:
		if len(table) < 10 {
			return nil
		}
		first, count := int(binary.BigEndian.Uint16(table[6:])), int(binary.BigEndian.Uint16(table[8:]))
		if len(table) < 10+2*count {
			return nil
		}
		for i := 0; i < count; i++ {
			if glyph := int(binary.BigEndian.Uint16(table[10+2*i:])); glyph != 0 {
				codes[first+i] = glyph
			}
		}
	case 12:
		return readFormat12(table, codes)
	default:
		return nil
	}
	return codes
}

// readFormat4 reads a segment mapping to delta values.
func readFormat4(table []byte, codes map[int]int) map[int]int {
	if len(table) < 16 {
		return nil
	}
	segments := int(binary.BigEndian.Uint16(table[6:])) / 2
	ends, starts := 14, 16+2*segments
	deltas, ranges := starts+2*segments, starts+4*segments
	if len(table) < ranges+2*segments {
		return nil
	}
	u16 := func(at int) int { return int(binary.BigEndian.Uint16(table[at:])) }
	for s := 0; s < segments; s++ {
		start, end, delta, offset := u16(starts+2*s), u16(ends+2*s), u16(deltas+2*s), u16(ranges+2*s)
		for code := start; code <= end && code < 0xFFFF; code++ {
			glyph := (code + delta) & 0xFFFF
			if offset != 0 {
				at := ranges + 2*s + offset + 2*(code-start)
				if at+2 > len(table) {
					continue
				}
				if glyph = u16(at); glyph != 0 {
					glyph = (glyph + delta) & 0xFFFF
				}
			}
			if glyph != 0 {
				codes[code] = glyph
			}
		}
	}
	return codes
}

// readFormat12 reads segmented coverage (32-bit codes).
func readFormat12(table []byte, codes map[int]int) map[int]int {
	if len(table) < 16 {
		return nil
	}
	groups := int(binary.BigEndian.Uint32(table[12:]))
	for g := 0; g < groups && 16+12*g+12 <= len(table); g++ {
		group := table[16+12*g:]
		start, end, glyph := int(binary.BigEndian.Uint32(group)), int(binary.BigEndian.Uint32(group[4:])), int(binary.BigEndian.Uint32(group[8:]))
		for code := start; code <= end && code-start < 0x10000; code++ {
			if glyph+code-start != 0 {
				codes[code] = glyph + code - start
			}
		}
	}
	return codes
}

// readPostNames reads a post table of format 2: glyph ids below 258 name the standard Macintosh
// glyphs (macGlyphNames), the others Pascal strings that follow.
func readPostNames(post []byte) []string {
	count := int(binary.BigEndian.Uint16(post[32:]))
	if len(post) < 34+2*count {
		return nil
	}
	var strs []string
	for at := 34 + 2*count; at < len(post); {
		length := int(post[at])
		if at+1+length > len(post) {
			break
		}
		strs = append(strs, string(post[at+1:at+1+length]))
		at += 1 + length
	}
	names := make([]string, count)
	for gid := range names {
		index := int(binary.BigEndian.Uint16(post[34+2*gid:]))
		switch {
		case index < len(macGlyphNames):
			names[gid] = macGlyphNames[index]
		case index-len(macGlyphNames) < len(strs):
			names[gid] = strs[index-len(macGlyphNames)]
		}
	}
	return names
}

// UnicodeOf inverts the program's Unicode cmaps: glyph -> the smallest code outside the private
// use areas that reaches it, from (3,10) first, then (3,1), then the (0,x) subtables. A glyph
// several codes reach takes the smallest of the first table that has one, so the result never
// depends on map order.
func (t *TrueType) UnicodeOf() map[int]int {
	inverse := map[int]int{}
	for _, key := range [][2]int{{3, 10}, {3, 1}, {0, 4}, {0, 3}, {0, 2}, {0, 1}, {0, 0}} {
		codes := t.Cmaps[key]
		for _, code := range slices.Sorted(maps.Keys(codes)) {
			if _, seen := inverse[codes[code]]; !seen && !privateCode(code) {
				inverse[codes[code]] = code
			}
		}
	}
	return inverse
}

// privateCode reports a code in a private use area, which names no character.
func privateCode(code int) bool {
	return code >= 0xE000 && code <= 0xF8FF || code >= 0xF0000
}
