package pdffonts

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// originalName is the name to look a figure font's original up by: the PDF's base font name, or,
// when that is a generated placeholder (Ghostscript's and Acrobat's CIDFont+F3), the PostScript
// name the embedded program keeps in its own name table (SymbolMT), subset tag removed.
func originalName(base string, name []byte) string {
	if !strings.HasPrefix(cMapless(base), "CIDFont+") {
		return base
	}
	own := programName(name)
	if at := strings.IndexByte(own, '+'); at == 6 {
		own = own[at+1:]
	}
	if own == "" || strings.HasPrefix(own, "CIDFont+") {
		return base
	}
	return own
}

// programName is the PostScript name (name ID 6) of an sfnt name table: the Windows (UTF-16)
// record first, else the Macintosh (Roman) one; "" without either.
func programName(table []byte) string {
	return sfntName(table, 6)
}

// sfntName is one name (name ID id) of an sfnt name table: the Windows (UTF-16) record first,
// else the Macintosh (Roman) one; "" without either.
func sfntName(table []byte, id uint16) string {
	if len(table) < 6 {
		return ""
	}
	count, strings0 := int(binary.BigEndian.Uint16(table[2:])), int(binary.BigEndian.Uint16(table[4:]))
	var mac string
	for i := 0; i < count && 6+12*i+12 <= len(table); i++ {
		record := table[6+12*i:]
		platform, name := binary.BigEndian.Uint16(record), binary.BigEndian.Uint16(record[6:])
		length, offset := int(binary.BigEndian.Uint16(record[8:])), strings0+int(binary.BigEndian.Uint16(record[10:]))
		if name != id || offset+length > len(table) {
			continue
		}
		value := table[offset : offset+length]
		switch platform {
		case 3:
			units := make([]uint16, len(value)/2)
			for j := range units {
				units[j] = binary.BigEndian.Uint16(value[2*j:])
			}
			return string(utf16.Decode(units))
		case 1:
			mac = string(value)
		}
	}
	return mac
}
