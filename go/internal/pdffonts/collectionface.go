package pdffonts

import (
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// errCollection is a font program that is not a readable collection, or a face of it that cannot
// be read.
var errCollection = errors.New("truetype collection: malformed program")

// sfntRecord is one table record of a collection's face directory; its offset counts from the
// file start.
type sfntRecord struct {
	tag      string
	checksum uint32
	offset   int
	length   int
}

// extractFace is one face of a TrueType collection as the standalone sfnt a PDF's /FontFile2 holds:
// the face's table directory at the start and its tables copied byte for byte, each 4-byte aligned.
// The head table differs only in its checkSumAdjustment (bytes 8..11), which is the file's own and
// must change with its layout; every other byte of every table is the face's. Glyph ids stay, so an
// /Identity /CIDToGIDMap remains valid. An error when the face or a table is not in ttc.
func extractFace(ttc []byte, face int) ([]byte, error) {
	version, records, err := faceRecords(ttc, face)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(records, func(a, b sfntRecord) int { return strings.Compare(a.tag, b.tag) })
	offsets, size := make([]int, len(records)), 12+16*len(records)
	for index, record := range records {
		offsets[index] = size
		size += record.length
		size += (4 - size%4) % 4
	}
	out := make([]byte, size)
	binary.BigEndian.PutUint32(out, version)
	binary.BigEndian.PutUint16(out[4:], uint16(len(records)))
	search, selector := 1, 0
	for search*2 <= len(records) {
		search, selector = search*2, selector+1
	}
	binary.BigEndian.PutUint16(out[6:], uint16(16*search))
	binary.BigEndian.PutUint16(out[8:], uint16(selector))
	binary.BigEndian.PutUint16(out[10:], uint16(16*len(records)-16*search))
	head := -1
	for index, record := range records {
		at := offsets[index]
		data := out[at : at+record.length]
		copy(data, ttc[record.offset:record.offset+record.length])
		sum := sfntChecksum(record.tag, data)
		if sum != record.checksum {
			return nil, fmt.Errorf("%w: table %s checksum %#08x, directory says %#08x", errCollection, record.tag, sum, record.checksum)
		}
		entry := out[12+16*index:]
		copy(entry, record.tag)
		binary.BigEndian.PutUint32(entry[4:], sum)
		binary.BigEndian.PutUint32(entry[8:], uint32(at))
		binary.BigEndian.PutUint32(entry[12:], uint32(record.length))
		if record.tag == "head" {
			head = at
		}
	}
	if head >= 0 && head+12 <= len(out) {
		clear(out[head+8 : head+12])
		binary.BigEndian.PutUint32(out[head+8:], 0xB1B0AFBA-sfntSum(out))
	}
	return out, nil
}

// faceRecords reads a collection face's sfnt version and table records, bounds-checking every
// record against ttc.
func faceRecords(ttc []byte, face int) (uint32, []sfntRecord, error) {
	if !isCollection(ttc) || len(ttc) < 16 {
		return 0, nil, errCollection
	}
	count := int(binary.BigEndian.Uint32(ttc[8:]))
	if face < 0 || face >= count || 16+4*face+4 > len(ttc) {
		return 0, nil, errCollection
	}
	at := int(binary.BigEndian.Uint32(ttc[12+4*face:]))
	if at < 0 || at+12 > len(ttc) {
		return 0, nil, errCollection
	}
	version, tables := binary.BigEndian.Uint32(ttc[at:]), int(binary.BigEndian.Uint16(ttc[at+4:]))
	if tables == 0 || at+12+16*tables > len(ttc) {
		return 0, nil, errCollection
	}
	records := make([]sfntRecord, tables)
	for index := range records {
		entry := ttc[at+12+16*index:]
		offset, length := int(binary.BigEndian.Uint32(entry[8:])), int(binary.BigEndian.Uint32(entry[12:]))
		if offset < 0 || length < 0 || offset > len(ttc) || length > len(ttc)-offset {
			return 0, nil, fmt.Errorf("%w: table %q lies outside the file", errCollection, string(entry[:4]))
		}
		records[index] = sfntRecord{string(entry[:4]), binary.BigEndian.Uint32(entry[4:]), offset, length}
	}
	return version, records, nil
}

// collectionFaceNamed is the face of a collection whose name table calls it base: its PostScript
// name (name ID 6) or its full name (name ID 4), folded, matched exactly. base is a font's name
// without its subset tag or a composite font's CMap suffix. Exactly one matching face, else false.
func collectionFaceNamed(ttc []byte, base string) (int, bool) {
	if !isCollection(ttc) {
		return 0, false
	}
	want := fold(cMapless(subsetless(base)))
	if want == "" {
		return 0, false
	}
	found := -1
	count := faceCount(ttc)
	if count <= 0 || count > (len(ttc)-16)/4 {
		return 0, false
	}
	for face := 0; face < count; face++ {
		_, records, err := faceRecords(ttc, face)
		if err != nil {
			continue
		}
		var table []byte
		for _, record := range records {
			if record.tag == "name" {
				table = ttc[record.offset : record.offset+record.length]
			}
		}
		if fold(sfntName(table, 6)) == want || fold(sfntName(table, 4)) == want {
			if found >= 0 {
				return 0, false
			}
			found = face
		}
	}
	return found, found >= 0
}

// faceName is a collection face's full name (name ID 4), else its PostScript name.
func faceName(ttc []byte, face int) string {
	_, records, err := faceRecords(ttc, face)
	if err != nil {
		return ""
	}
	var table []byte
	for _, record := range records {
		if record.tag == "name" {
			table = ttc[record.offset : record.offset+record.length]
		}
	}
	if name := sfntName(table, 4); name != "" {
		return name
	}
	return sfntName(table, 6)
}

// faceCount is the number of fonts a collection holds.
func faceCount(ttc []byte) int {
	if !isCollection(ttc) {
		return 0
	}
	return int(binary.BigEndian.Uint32(ttc[8:]))
}

// subsetless is a font name without its six-letter subset tag (ABCDEF+Name).
func subsetless(name string) string {
	if len(name) > 7 && name[6] == '+' {
		return name[7:]
	}
	return name
}

// fontBase is a font dictionary's BaseFont as the report names it: no leading slash, subset tag
// kept (DCWGQU+CambriaMath), as pdftag's figure fonts are named.
func fontBase(raw any) string {
	name, _ := raw.(string)
	return strings.TrimPrefix(name, "/")
}

// sfntChecksum is a table's checksum: the sum of its big-endian words, the head table's with its
// checkSumAdjustment zeroed, as an sfnt table directory records it.
func sfntChecksum(tag string, data []byte) uint32 {
	if tag != "head" || len(data) < 12 {
		return sfntSum(data)
	}
	copied := slices.Clone(data)
	clear(copied[8:12])
	return sfntSum(copied)
}

// sfntSum adds the big-endian words of data, the last one zero-padded.
func sfntSum(data []byte) (sum uint32) {
	for at := 0; at < len(data); at += 4 {
		var word [4]byte
		copy(word[:], data[at:])
		sum += binary.BigEndian.Uint32(word[:])
	}
	return sum
}

// Unwrapped is a font repaired by replacing a collection its figure embedded with the single face
// the font names (pdfua.font_unwrapped).
type Unwrapped struct {
	Font   string `json:"font"`   // BaseFont, subset tag kept
	Face   int    `json:"face"`   // the face's index in the collection
	Name   string `json:"name"`   // the face's name
	Faces  int    `json:"faces"`  // how many faces the collection held
	Before int    `json:"before"` // the collection's bytes
	After  int    `json:"after"`  // the extracted face's bytes
}

// unwrapCollections replaces every TrueType collection an embedded /FontFile2 holds with the one
// face its font names, before anything reads the program. The stream may serve several fonts:
// every one of them must name the same face, else it stays as it is. The subset font, its widths,
// ToUnicode and /CIDToGIDMap are untouched: the face's glyph ids and data are its own.
func (x *Fixer) unwrapCollections(refs []string) {
	order := []string{}
	// FontFile2 reference -> the fonts that use it, by the name the report shows and the name a
	// face's name table is looked up by.
	type embedded struct{ report, lookup string }
	fonts := map[string][]embedded{}
	for _, ref := range refs {
		dict, _ := x.store.Resolve(ref).(map[string]any)
		faces := []map[string]any{dict}
		if dict["/Subtype"] == "/Type0" {
			descendants, _ := x.store.Resolve(dict["/DescendantFonts"]).([]any)
			faces = nil
			for _, child := range descendants {
				if descendant, ok := x.store.Resolve(child).(map[string]any); ok {
					faces = append(faces, descendant)
				}
			}
		}
		for _, font := range faces {
			descriptor, _ := x.store.Resolve(font["/FontDescriptor"]).(map[string]any)
			stream, ok := descriptor["/FontFile2"].(string)
			if !ok {
				continue
			}
			if _, seen := fonts[stream]; !seen {
				order = append(order, stream)
			}
			fonts[stream] = append(fonts[stream], embedded{fontBase(font["/BaseFont"]), figureName(font["/BaseFont"])})
		}
	}
	seen := map[string]bool{}
	for _, stream := range order {
		if seen[stream] {
			continue
		}
		seen[stream] = true
		data, err := x.store.StreamData(stream)
		if err != nil || !isCollection(data) {
			continue
		}
		face, ok := collectionFaceNamed(data, fonts[stream][0].lookup)
		for _, font := range fonts[stream][1:] {
			other, matches := collectionFaceNamed(data, font.lookup)
			ok = ok && matches && other == face
		}
		if !ok {
			continue
		}
		faceData, err := extractFace(data, face)
		if err != nil {
			continue
		}
		dict, _ := x.store.Resolve(stream).(map[string]any)
		changed := maps.Clone(dict)
		if changed == nil {
			changed = map[string]any{}
		}
		changed["/Length1"] = len(faceData)
		x.store.SetStream(stream, changed, faceData)
		name := faceName(data, face)
		for _, font := range fonts[stream] {
			x.Report.Unwrapped = append(x.Report.Unwrapped, Unwrapped{Font: font.report, Face: face, Name: name,
				Faces: faceCount(data), Before: len(data), After: len(faceData)})
		}
	}
}
