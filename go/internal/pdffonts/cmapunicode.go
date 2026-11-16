package pdffonts

import (
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// A non-symbolic TrueType font must carry a Microsoft Unicode (3,1) or a Macintosh Roman (1,0) cmap
// subtable (PDF/UA-2 8.4.5.7 test 1). Subset programs a figure's exporter writes often keep only the
// Unicode platform's (0,x) subtable, which is the same format 4 table a (3,1) record names. The tagged
// PDF's copy of the program gets that record, pointing at the very same subtable; no glyph, width or
// outline changes (decision 179).

// errNoCmap says the program needs no record or cannot be given one.
var errNoCmap = errors.New("truetype cmap: nothing to add")

// cmapRecord is one encoding record of a cmap table.
type cmapRecord struct {
	platform, encoding uint16
	offset             uint32
}

// unicodeCmapSource is the encoding of the Unicode platform's format 4 subtable a (3,1) record can
// share, written "(0,3)".
func unicodeCmapSource(records []cmapRecord, table []byte) (cmapRecord, bool) {
	best, found := cmapRecord{}, false
	for _, record := range records {
		at := int(record.offset)
		if record.platform != 0 || at < 0 || at+2 > len(table) || binary.BigEndian.Uint16(table[at:]) != 4 {
			continue
		}
		if !found || record.encoding == 3 {
			best, found = record, true
		}
	}
	return best, found
}

// readCmapRecords reads a cmap table's encoding records, each checked against the table.
func readCmapRecords(table []byte) ([]cmapRecord, error) {
	if len(table) < 4 {
		return nil, errNoCmap
	}
	count := int(binary.BigEndian.Uint16(table[2:]))
	if count == 0 || 4+8*count > len(table) {
		return nil, errNoCmap
	}
	records := make([]cmapRecord, count)
	for index := range records {
		entry := table[4+8*index:]
		records[index] = cmapRecord{binary.BigEndian.Uint16(entry), binary.BigEndian.Uint16(entry[2:]), binary.BigEndian.Uint32(entry[4:])}
		if int(records[index].offset) >= len(table) {
			return nil, errNoCmap
		}
	}
	return records, nil
}

// withUnicodeCmap is the sfnt program with a (3,1) cmap record added, sharing the Unicode platform's
// format 4 subtable. from names the subtable's encoding, "(0,3)". An error (errNoCmap) when the
// program is no standalone sfnt, has a (3,1) or (1,0) subtable already, or has no format 4 subtable
// on the Unicode platform to share.
func withUnicodeCmap(program []byte) (changed []byte, from string, err error) {
	if len(program) < 12 || isCollection(program) {
		return nil, "", errNoCmap
	}
	version, tables := binary.BigEndian.Uint32(program), int(binary.BigEndian.Uint16(program[4:]))
	if tables == 0 || 12+16*tables > len(program) {
		return nil, "", errNoCmap
	}
	data := map[string][]byte{}
	for index := 0; index < tables; index++ {
		entry := program[12+16*index:]
		offset, length := int(binary.BigEndian.Uint32(entry[8:])), int(binary.BigEndian.Uint32(entry[12:]))
		if offset < 0 || length < 0 || offset > len(program) || length > len(program)-offset {
			return nil, "", errNoCmap
		}
		data[string(entry[:4])] = program[offset : offset+length]
	}
	table := data["cmap"]
	records, err := readCmapRecords(table)
	if err != nil {
		return nil, "", err
	}
	for _, record := range records {
		if (record.platform == 3 && record.encoding == 1) || (record.platform == 1 && record.encoding == 0) {
			return nil, "", errNoCmap
		}
	}
	source, ok := unicodeCmapSource(records, table)
	if !ok {
		return nil, "", errNoCmap
	}
	data["cmap"] = insertCmapRecord(table, records, cmapRecord{3, 1, source.offset})
	return assembleSfnt(version, data), fmt.Sprintf("(%d,%d)", source.platform, source.encoding), nil
}

// insertCmapRecord is the cmap table with record added in platform and encoding order, the subtables
// after the longer record list unchanged: only their offsets grow by the record's 8 bytes.
func insertCmapRecord(table []byte, records []cmapRecord, added cmapRecord) []byte {
	all := append(slices.Clone(records), added)
	slices.SortStableFunc(all, func(a, b cmapRecord) int {
		if a.platform != b.platform {
			return int(a.platform) - int(b.platform)
		}
		return int(a.encoding) - int(b.encoding)
	})
	out := binary.BigEndian.AppendUint16(nil, binary.BigEndian.Uint16(table))
	out = binary.BigEndian.AppendUint16(out, uint16(len(all)))
	for _, record := range all {
		out = binary.BigEndian.AppendUint16(out, record.platform)
		out = binary.BigEndian.AppendUint16(out, record.encoding)
		out = binary.BigEndian.AppendUint32(out, record.offset+8)
	}
	return append(out, table[4+8*len(records):]...)
}

// assembleSfnt writes the sfnt of the tables: the directory at the start, the tables in tag order
// each 4-byte aligned, every checksum recomputed, head's checkSumAdjustment the file's own.
func assembleSfnt(version uint32, data map[string][]byte) []byte {
	tags := slices.Sorted(maps.Keys(data))
	size := 12 + 16*len(tags)
	offsets := make([]int, len(tags))
	for index, tag := range tags {
		offsets[index] = size
		size += len(data[tag])
		size += (4 - size%4) % 4
	}
	out := make([]byte, size)
	binary.BigEndian.PutUint32(out, version)
	binary.BigEndian.PutUint16(out[4:], uint16(len(tags)))
	search, selector := 1, 0
	for search*2 <= len(tags) {
		search, selector = search*2, selector+1
	}
	binary.BigEndian.PutUint16(out[6:], uint16(16*search))
	binary.BigEndian.PutUint16(out[8:], uint16(selector))
	binary.BigEndian.PutUint16(out[10:], uint16(16*len(tags)-16*search))
	head := -1
	for index, tag := range tags {
		at := offsets[index]
		copy(out[at:], data[tag])
		entry := out[12+16*index:]
		copy(entry, tag)
		binary.BigEndian.PutUint32(entry[4:], sfntChecksum(tag, data[tag]))
		binary.BigEndian.PutUint32(entry[8:], uint32(at))
		binary.BigEndian.PutUint32(entry[12:], uint32(len(data[tag])))
		if tag == "head" {
			head = at
		}
	}
	if head >= 0 && head+12 <= len(out) {
		clear(out[head+8 : head+12])
		binary.BigEndian.PutUint32(out[head+8:], 0xB1B0AFBA-sfntSum(out))
	}
	return out
}

// CmapAdded is a figure font whose embedded program got the (3,1) record the Unicode subtable it
// already had names (pdfua.font_cmap_added).
type CmapAdded struct {
	Font string `json:"font"` // BaseFont, subset tag kept
	From string `json:"from"` // the shared subtable's encoding, "(0,3)"
}

// addUnicodeCmaps gives every embedded non-symbolic TrueType font of refs that lacks a (3,1) and a (1,0)
// cmap subtable the (3,1) record, in its program's own copy of the stream. A stream may serve several
// fonts; each is reported.
func (x *Fixer) addUnicodeCmaps(refs []string) {
	fonts := map[string][]string{}
	order := []string{}
	for _, ref := range refs {
		dict, _ := x.store.Resolve(ref).(map[string]any)
		if dict["/Subtype"] != "/TrueType" {
			continue
		}
		descriptor, _ := x.store.Resolve(dict["/FontDescriptor"]).(map[string]any)
		flags := int(Number(descriptor["/Flags"]))
		stream, ok := descriptor["/FontFile2"].(string)
		if !ok || flags&4 != 0 || flags&32 == 0 {
			continue
		}
		if _, seen := fonts[stream]; !seen {
			order = append(order, stream)
		}
		fonts[stream] = append(fonts[stream], fontBase(dict["/BaseFont"]))
	}
	for _, stream := range order {
		data, err := x.store.StreamData(stream)
		if err != nil {
			continue
		}
		changed, from, err := withUnicodeCmap(data)
		if err != nil {
			continue
		}
		dict, _ := x.store.Resolve(stream).(map[string]any)
		updated := maps.Clone(dict)
		if updated == nil {
			updated = map[string]any{}
		}
		updated["/Length1"] = len(changed)
		x.store.SetStream(stream, updated, changed)
		for _, name := range fonts[stream] {
			x.Report.CmapsAdded = append(x.Report.CmapsAdded, CmapAdded{Font: name, From: from})
		}
	}
}
