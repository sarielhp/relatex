package pdffonts

import "encoding/binary"

// ligatures reads the ligature substitutions of a GSUB table (lookup type 4, and type 7
// extensions that hold one): ligature glyph -> the glyph sequences that form it. Only what an
// installed font's ligature glyph needs to read back as its characters ("ti" for the ti glyph).
func ligatures(gsub []byte) map[int][][]int {
	found := map[int][][]int{}
	if len(gsub) < 10 {
		return found
	}
	list := int(binary.BigEndian.Uint16(gsub[8:]))
	for _, lookup := range offsets(gsub, list) {
		readLookup(gsub, list+lookup, found)
	}
	return found
}

// offsets reads the count and 16-bit offsets that start a table at at (nil when truncated).
func offsets(data []byte, at int) []int {
	if at < 0 || at+2 > len(data) {
		return nil
	}
	count := int(binary.BigEndian.Uint16(data[at:]))
	if at+2+2*count > len(data) {
		return nil
	}
	list := make([]int, count)
	for i := range list {
		list[i] = int(binary.BigEndian.Uint16(data[at+2+2*i:]))
	}
	return list
}

// readLookup collects the ligature subtables of the lookup at at.
func readLookup(gsub []byte, at int, found map[int][][]int) {
	if at < 0 || at+6 > len(gsub) {
		return
	}
	kind := int(binary.BigEndian.Uint16(gsub[at:]))
	for _, sub := range offsets(gsub, at+4) {
		table, subKind := at+sub, kind
		if kind == 7 && table+8 <= len(gsub) {
			subKind = int(binary.BigEndian.Uint16(gsub[table+2:]))
			table += int(binary.BigEndian.Uint32(gsub[table+4:]))
		}
		if subKind == 4 {
			readLigatureSubtable(gsub, table, found)
		}
	}
}

// readLigatureSubtable reads a LigatureSubstFormat1 table.
func readLigatureSubtable(gsub []byte, at int, found map[int][][]int) {
	if at < 0 || at+6 > len(gsub) || binary.BigEndian.Uint16(gsub[at:]) != 1 {
		return
	}
	firsts := coverage(gsub, at+int(binary.BigEndian.Uint16(gsub[at+2:])))
	for i, set := range offsets(gsub, at+4) {
		if i >= len(firsts) {
			return
		}
		for _, entry := range offsets(gsub, at+set) {
			ligature, components, ok := readLigature(gsub, at+set+entry)
			if ok {
				found[ligature] = append(found[ligature], append([]int{firsts[i]}, components...))
			}
		}
	}
}

// readLigature reads one ligature: its glyph and the components after the first.
func readLigature(gsub []byte, at int) (int, []int, bool) {
	if at < 0 || at+4 > len(gsub) {
		return 0, nil, false
	}
	count := int(binary.BigEndian.Uint16(gsub[at+2:]))
	if count < 1 || at+4+2*(count-1) > len(gsub) {
		return 0, nil, false
	}
	components := make([]int, count-1)
	for i := range components {
		components[i] = int(binary.BigEndian.Uint16(gsub[at+4+2*i:]))
	}
	return int(binary.BigEndian.Uint16(gsub[at:])), components, true
}

// coverage lists the glyphs of a coverage table in coverage index order.
func coverage(data []byte, at int) []int {
	if at < 0 || at+4 > len(data) {
		return nil
	}
	count := int(binary.BigEndian.Uint16(data[at+2:]))
	var glyphs []int
	switch binary.BigEndian.Uint16(data[at:]) {
	case 1:
		for i := 0; i < count && at+4+2*i+2 <= len(data); i++ {
			glyphs = append(glyphs, int(binary.BigEndian.Uint16(data[at+4+2*i:])))
		}
	case 2:
		for i := 0; i < count && at+4+6*i+6 <= len(data); i++ {
			from, to := int(binary.BigEndian.Uint16(data[at+4+6*i:])), int(binary.BigEndian.Uint16(data[at+6+6*i:]))
			for gid := from; gid <= to && len(glyphs) < 70000; gid++ {
				glyphs = append(glyphs, gid)
			}
		}
	}
	return glyphs
}
