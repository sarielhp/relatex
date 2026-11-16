package pdffonts

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// subtablesCmap is a cmap table with the encodings given, platform/encoding/format each, one subtable per record.
func subtablesCmap(encodings ...[3]uint16) []byte {
	out := binary.BigEndian.AppendUint16(nil, 0)
	out = binary.BigEndian.AppendUint16(out, uint16(len(encodings)))
	var subtables []byte
	offset := 4 + 8*len(encodings)
	for _, encoding := range encodings {
		out = binary.BigEndian.AppendUint16(out, encoding[0])
		out = binary.BigEndian.AppendUint16(out, encoding[1])
		out = binary.BigEndian.AppendUint32(out, uint32(offset+len(subtables)))
		subtable := binary.BigEndian.AppendUint16(nil, encoding[2])
		subtable = binary.BigEndian.AppendUint16(subtable, 8)
		subtable = append(subtable, 1, 2, 3, 4)
		subtables = append(subtables, subtable...)
	}
	return append(out, subtables...)
}

func recordsOf(t *testing.T, program []byte) []cmapRecord {
	t.Helper()
	records, err := readCmapRecords(tableDirectory(program)["cmap"])
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestUnicodePlatformSubtableGetsAMicrosoftRecord(t *testing.T) {
	glyf := []byte{9, 8, 7, 6, 5}
	program := sfnt(map[string][]byte{"head": headTable(0x00010000), "glyf": glyf, "cmap": subtablesCmap([3]uint16{0, 3, 4})})
	changed, from, err := withUnicodeCmap(program)
	if err != nil || from != "(0,3)" {
		t.Fatalf("withUnicodeCmap = %v, %q", err, from)
	}
	records := recordsOf(t, changed)
	if len(records) != 2 || records[0].platform != 0 || records[1].platform != 3 || records[1].encoding != 1 ||
		records[0].offset != records[1].offset {
		t.Fatalf("records %v: want (0,3) and a (3,1) sharing its subtable", records)
	}
	table := tableDirectory(changed)["cmap"]
	if binary.BigEndian.Uint16(table[records[1].offset:]) != 4 {
		t.Error("the shared subtable is not the format 4 one")
	}
	if !bytes.Equal(tableDirectory(changed)["glyf"], glyf) {
		t.Error("a table other than cmap changed")
	}
	if sum := sfntSum(changed); sum != 0xB1B0AFBA {
		t.Errorf("whole-file checksum %#x, want 0xB1B0AFBA", sum)
	}
	if _, _, again := withUnicodeCmap(changed); again == nil {
		t.Error("a program with a (3,1) record was changed again")
	}
}

func TestCmapIsLeftAloneWithoutAUnicodeSubtableToShare(t *testing.T) {
	for name, encodings := range map[string][][3]uint16{
		"has (3,1)":           {{0, 3, 4}, {3, 1, 4}},
		"has (1,0)":           {{1, 0, 0}},
		"symbol only":         {{3, 0, 4}},
		"unicode, format 12":  {{0, 4, 12}},
		"unicode, format 6":   {{0, 3, 6}},
		"no unicode platform": {{1, 0, 0}, {3, 10, 12}},
	} {
		program := sfnt(map[string][]byte{"head": headTable(0x00010000), "cmap": subtablesCmap(encodings...)})
		if _, _, err := withUnicodeCmap(program); err == nil {
			t.Errorf("%s: the program was changed", name)
		}
	}
	if _, _, err := withUnicodeCmap(fakeTTC()); err == nil {
		t.Error("a collection was changed")
	}
}
