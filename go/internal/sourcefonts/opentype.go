// Package sourcefonts is the Go port of Relatex::SourceFonts and OpenTypeFont: the fonts a
// paper's text uses, as the HTML names and links them. TeX's own families are named with a
// CSS font stack; a font the paper loaded from a file is named by its own family and, when
// its license allows, its original file is linked.
package sourcefonts

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/bits"
	"os"
	"path/filepath"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

var signatures = map[string]string{"\x00\x01\x00\x00": "truetype", "true": "truetype", "OTTO": "opentype"}

const (
	// restricted: OS/2 fsType 2 alone is "restricted license embedding"; bitmapOnly forbids
	// outline use.
	restricted = 0x0002
	bitmapOnly = 0x0200
)

type tableEntry struct{ start, length int }

// Font is OpenTypeFont: the few facts the HTML export and its checks need from an OpenType or
// TrueType file. A collection is read at the face whose PostScript name was observed.
type Font struct {
	Path       string
	Format     string
	data       []byte
	collection bool
	tables     map[string]tableEntry
}

// OpenFont is OpenTypeFont.new(path, postscript_name:); postscript is "" for none.
func OpenFont(path, postscript string) *Font {
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	f := &Font{Path: path, data: data, collection: len(data) >= 4 && string(data[:4]) == "ttcf"}
	offsets := []int{0}
	if f.collection {
		offsets = f.collectionOffsets()
	}
	faces := make([]map[string]tableEntry, len(offsets))
	for index, offset := range offsets {
		faces[index] = f.readDirectory(offset)
	}
	f.tables = faces[0]
	if postscript != "" && len(faces) > 1 {
		f.tables = nil
		for _, tables := range faces {
			if name, ok := f.faceName(tables, 6); ok && name == postscript {
				f.tables = tables
				break
			}
		}
		if f.tables == nil {
			rec.Fail("Collection lacks face %s", postscript)
		}
	}
	face := f.tables[".face"].start
	f.Format = signatures[string(f.slice(face, 4))]
	if f.Format == "" {
		rec.Fail("Unsupported font file: %s", filepath.Base(path))
	}
	return f
}

// slice is byteslice: bytes past the end are cut off.
func (f *Font) slice(offset, length int) []byte {
	if offset < 0 || offset > len(f.data) {
		return nil
	}
	return f.data[offset:min(offset+length, len(f.data))]
}

func (f *Font) u16(offset int) int { return int(binary.BigEndian.Uint16(f.bytesAt(offset, 2))) }

// bytesAt is the size bytes at offset, failing on truncated data.
func (f *Font) bytesAt(offset, size int) []byte {
	if offset < 0 || offset+size > len(f.data) {
		rec.Fail("Truncated font data")
	}
	return f.data[offset : offset+size]
}

func (f *Font) s16(offset int) int { return int(int16(f.u16(offset))) }

func (f *Font) u32(offset int) int { return int(binary.BigEndian.Uint32(f.bytesAt(offset, 4))) }

// Collection is collection?.
func (f *Font) Collection() bool { return f.collection }

// SHA256 is the file's digest.
func (f *Font) SHA256() string {
	sum := sha256.Sum256(f.data)
	return hex.EncodeToString(sum[:])
}

// Size is bytesize.
func (f *Font) Size() int64 { return int64(len(f.data)) }

// Family is the typographic family (name 16) when present, else the legacy family (name 1).
func (f *Font) Family() string {
	if name, ok := f.Name(16); ok {
		return name
	}
	if name, ok := f.Name(1); ok {
		return name
	}
	rec.Fail("Font has no family name: %s", filepath.Base(f.Path))
	return ""
}

// PostScriptName is name 6, "" without one.
func (f *Font) PostScriptName() (string, bool) { return f.Name(6) }

// Shippable reports whether the font may travel with a document: its embedding license
// does not restrict it.
func (f *Font) Shippable() bool {
	table, ok := f.tables["OS/2"]
	if !ok || table.length < 10 {
		return true
	}
	flags := f.u16(table.start + 8)
	return flags&0x000F != restricted && flags&bitmapOnly == 0
}

// UnitsPerEm is the head table's units per em.
func (f *Font) UnitsPerEm() int {
	table, ok := f.tables["head"]
	if !ok {
		rec.Fail("Font has no head table")
	}
	return f.u16(table.start + 18)
}

// PositivePositioningBound is the font units by which positioning can widen a gap between
// two letters: per lookup, the largest positive x advance plus twice the largest x
// placement, summed over GPOS lookups and the legacy kern table.
func (f *Font) PositivePositioningBound() int { return f.gposBound() + f.kernBound() }

// Name is a name record: Windows Unicode first, then Macintosh Roman; ok is false without one.
func (f *Font) Name(identifier int) (string, bool) { return f.faceName(f.tables, identifier) }

func (f *Font) collectionOffsets() []int {
	count := f.u32(8)
	if count < 1 || count > 1024 {
		rec.Fail("Implausible font collection")
	}
	offsets := make([]int, count)
	for index := range offsets {
		offsets[index] = f.u32(12 + index*4)
	}
	return offsets
}

func (f *Font) readDirectory(offset int) map[string]tableEntry {
	count := f.u16(offset + 4)
	tables := map[string]tableEntry{}
	for index := 0; index < count; index++ {
		at := offset + 12 + index*16
		tag := string(f.slice(at, 4))
		start, length := f.u32(at+8), f.u32(at+12)
		if start+length > len(f.data) {
			rec.Fail("Truncated font table %s", tag)
		}
		tables[tag] = tableEntry{start, length}
	}
	tables[".face"] = tableEntry{offset, 0}
	return tables
}

func (f *Font) faceName(tables map[string]tableEntry, identifier int) (string, bool) {
	table, ok := tables["name"]
	if !ok {
		return "", false
	}
	offset := table.start
	count, strings := f.u16(offset+2), f.u16(offset+4)
	var windows, mac []int
	for index := 0; index < count; index++ {
		row := make([]int, 6)
		for field := range row {
			row[field] = f.u16(offset + 6 + index*12 + field*2)
		}
		if row[3] != identifier {
			continue
		}
		if windows == nil && row[0] == 3 && (row[1] == 0 || row[1] == 1 || row[1] == 10) {
			windows = row
		}
		if mac == nil && row[0] == 1 && row[1] == 0 {
			mac = row
		}
	}
	record := windows
	if record == nil {
		record = mac
	}
	if record == nil {
		return "", false
	}
	raw := f.slice(offset+strings+record[5], record[4])
	text, valid := decodeName(raw, record[0] == 3)
	text = rec.Strip(text)
	if !valid || text == "" {
		return "", false
	}
	return text, true
}

// decodeName is UTF-16BE (Windows) or Mac Roman to UTF-8; an invalid sequence is Ruby's
// EncodingError, no name.
func decodeName(raw []byte, windows bool) (string, bool) {
	if !windows {
		runes := make([]rune, len(raw))
		for index, character := range raw {
			if runes[index] = macRoman(character); runes[index] < 0 {
				return "", false
			}
		}
		return string(runes), true
	}
	if len(raw)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(raw)/2)
	for index := range units {
		units[index] = binary.BigEndian.Uint16(raw[index*2:])
	}
	for index := 0; index < len(units); index++ {
		unit := units[index]
		if unit >= 0xD800 && unit < 0xDC00 {
			if index+1 >= len(units) || units[index+1] < 0xDC00 || units[index+1] >= 0xE000 {
				return "", false
			}
			index++
		} else if unit >= 0xDC00 && unit < 0xE000 {
			return "", false
		}
	}
	text := string(utf16.Decode(units))
	return text, utf8.ValidString(text)
}

// macRomanHigh is Ruby's MacRoman for 0x80-0xFF; -1 where Ruby has no conversion.
var macRomanHigh = []rune{
	0x00C4, 0x00C5, 0x00C7, 0x00C9, 0x00D1, 0x00D6, 0x00DC, 0x00E1,
	0x00E0, 0x00E2, 0x00E4, 0x00E3, 0x00E5, 0x00E7, 0x00E9, 0x00E8,
	0x00EA, 0x00EB, 0x00ED, 0x00EC, 0x00EE, 0x00EF, 0x00F1, 0x00F3,
	0x00F2, 0x00F4, 0x00F6, 0x00F5, 0x00FA, 0x00F9, 0x00FB, 0x00FC,
	0x2020, 0x00B0, 0x00A2, 0x00A3, 0x00A7, 0x2022, 0x00B6, 0x00DF,
	0x00AE, 0x00A9, 0x2122, 0x00B4, 0x00A8, 0x2260, 0x00C6, 0x00D8,
	0x221E, 0x00B1, 0x2264, 0x2265, 0x00A5, 0x00B5, 0x2202, 0x2211,
	0x220F, 0x03C0, 0x222B, 0x00AA, 0x00BA, 0x2126, 0x00E6, 0x00F8,
	0x00BF, 0x00A1, 0x00AC, 0x221A, 0x0192, 0x2248, 0x2206, 0x00AB,
	0x00BB, 0x2026, 0x00A0, 0x00C0, 0x00C3, 0x00D5, 0x0152, 0x0153,
	0x2013, 0x2014, 0x201C, 0x201D, 0x2018, 0x2019, 0x00F7, 0x25CA,
	0x00FF, 0x0178, 0x2044, 0x00A4, 0x2039, 0x203A, 0xFB01, 0xFB02,
	0x2021, 0x00B7, 0x201A, 0x201E, 0x2030, 0x00C2, 0x00CA, 0x00C1,
	0x00CB, 0x00C8, 0x00CD, 0x00CE, 0x00CF, 0x00CC, 0x00D3, 0x00D4,
	-1, 0x00D2, 0x00DA, 0x00DB, 0x00D9, 0x0131, 0x02C6, 0x02DC,
	0x00AF, 0x02D8, 0x02D9, 0x02DA, 0x00B8, 0x02DD, 0x02DB, 0x02C7,
}

func macRoman(character byte) rune {
	if character < 0x80 {
		return rune(character)
	}
	return macRomanHigh[character-0x80]
}

func (f *Font) gposBound() int {
	table, ok := f.tables["GPOS"]
	if !ok {
		return 0
	}
	list := table.start + f.u16(table.start+8)
	count := f.u16(list)
	total := 0
	for index := 0; index < count; index++ {
		advance, placement := 0, 0
		for _, record := range f.lookupValues(list + f.u16(list+2+index*2)) {
			advance = max(advance, record.advance)
			placement = max(placement, 2*abs(record.placement))
		}
		total += advance + placement
	}
	return total
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

type valueRecord struct{ placement, advance int }

// lookupValues is every ValueRecord of a single (1) or pair (2) adjustment lookup, through
// extensions (9).
func (f *Font) lookupValues(lookup int) []valueRecord {
	kindOfLookup, count := f.u16(lookup), f.u16(lookup+4)
	records := []valueRecord{}
	for index := 0; index < count; index++ {
		subtable := lookup + f.u16(lookup+6+index*2)
		kind := kindOfLookup
		if kindOfLookup == 9 {
			kind = f.u16(subtable + 2)
			subtable += f.u32(subtable + 4)
		}
		switch kind {
		case 1:
			records = append(records, f.singleValues(subtable)...)
		case 2:
			records = append(records, f.pairValues(subtable)...)
		}
	}
	return records
}

func (f *Font) singleValues(subtable int) []valueRecord {
	format, valueFormat := f.u16(subtable), f.u16(subtable+4)
	if format == 1 {
		return []valueRecord{f.valueRecord(subtable+6, valueFormat)}
	}
	count := f.u16(subtable + 6)
	size := valueSize(valueFormat)
	records := make([]valueRecord, count)
	for index := range records {
		records[index] = f.valueRecord(subtable+8+index*size, valueFormat)
	}
	return records
}

func (f *Font) pairValues(subtable int) []valueRecord {
	format, first, second := f.u16(subtable), f.u16(subtable+4), f.u16(subtable+6)
	sizes := [2]int{valueSize(first), valueSize(second)}
	if format == 1 {
		return f.pairSetValues(subtable, first, second, sizes)
	}
	class1, class2 := f.u16(subtable+12), f.u16(subtable+14)
	start := subtable + 16
	records := []valueRecord{}
	for index := 0; index < class1*class2; index++ {
		at := start + index*(sizes[0]+sizes[1])
		records = append(records, f.valueRecord(at, first), f.valueRecord(at+sizes[0], second))
	}
	return records
}

func (f *Font) pairSetValues(subtable, first, second int, sizes [2]int) []valueRecord {
	count := f.u16(subtable + 8)
	records := []valueRecord{}
	for index := 0; index < count; index++ {
		set := subtable + f.u16(subtable+10+index*2)
		pairs := f.u16(set)
		for pair := 0; pair < pairs; pair++ {
			at := set + 2 + pair*(2+sizes[0]+sizes[1]) + 2
			records = append(records, f.valueRecord(at, first), f.valueRecord(at+sizes[0], second))
		}
	}
	return records
}

func valueSize(valueFormat int) int { return 2 * bits.OnesCount(uint(valueFormat)) }

// valueRecord reads the eight ValueRecord fields valueFormat declares and keeps the x
// placement (bit 0) and x advance (bit 2).
func (f *Font) valueRecord(offset, valueFormat int) valueRecord {
	record := valueRecord{}
	position := 0
	for bit := 0; bit < 8; bit++ {
		if valueFormat&(1<<bit) == 0 {
			continue
		}
		value := f.s16(offset + position*2)
		position++
		switch bit {
		case 0:
			record.placement = value
		case 2:
			record.advance = value
		}
	}
	return record
}

// kernBound is the largest positive pair value of format-0 subtables of the legacy kern table.
func (f *Font) kernBound() int {
	table, ok := f.tables["kern"]
	if !ok {
		return 0
	}
	if f.u16(table.start) != 0 {
		return 0
	}
	count := f.u16(table.start + 2)
	at := table.start + 4
	total := 0
	for index := 0; index < count; index++ {
		length, coverage := f.u16(at+2), f.u16(at+4)
		pairs := 0
		if coverage>>8 == 0 {
			pairs = f.u16(at + 6)
		}
		largest := 0
		for pair := 0; pair < pairs; pair++ {
			largest = max(largest, f.s16(at+14+pair*6+4))
		}
		at += length
		total += largest
	}
	return total
}
