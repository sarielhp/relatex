package pdffonts

import (
	"encoding/binary"
	"slices"
	"testing"
)

// sfnt builds a TrueType program of the given tables (tag -> data), as the table directory reads it.
func sfnt(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	out := binary.BigEndian.AppendUint32(nil, 0x00010000)
	out = binary.BigEndian.AppendUint16(out, uint16(len(tags)))
	out = append(out, 0, 0, 0, 0, 0, 0)
	offset := 12 + 16*len(tags)
	for _, tag := range tags {
		out = append(out, tag...)
		out = binary.BigEndian.AppendUint32(out, 0)
		out = binary.BigEndian.AppendUint32(out, uint32(offset))
		out = binary.BigEndian.AppendUint32(out, uint32(len(tables[tag])))
		offset += len(tables[tag])
	}
	for _, tag := range tags {
		out = append(out, tables[tag]...)
	}
	return out
}

// subtable is one cmap subtable record: platform, encoding and its data.
type subtable struct {
	platform, encoding uint16
	data               []byte
}

func cmapTable(subtables ...subtable) []byte {
	out := binary.BigEndian.AppendUint16(nil, 0)
	out = binary.BigEndian.AppendUint16(out, uint16(len(subtables)))
	offset := 4 + 8*len(subtables)
	for _, sub := range subtables {
		out = binary.BigEndian.AppendUint16(out, sub.platform)
		out = binary.BigEndian.AppendUint16(out, sub.encoding)
		out = binary.BigEndian.AppendUint32(out, uint32(offset))
		offset += len(sub.data)
	}
	for _, sub := range subtables {
		out = append(out, sub.data...)
	}
	return out
}

// format6 is a cmap subtable of format 6: glyphs for the codes first, first+1, ...
func format6(first uint16, glyphs ...uint16) []byte {
	out := binary.BigEndian.AppendUint16(nil, 6)
	out = binary.BigEndian.AppendUint16(out, uint16(10+2*len(glyphs)))
	out = binary.BigEndian.AppendUint16(out, 0)
	out = binary.BigEndian.AppendUint16(out, first)
	out = binary.BigEndian.AppendUint16(out, uint16(len(glyphs)))
	for _, glyph := range glyphs {
		out = binary.BigEndian.AppendUint16(out, glyph)
	}
	return out
}

// format4 is a cmap subtable of format 4 with one segment start..end whose glyphs are the codes
// plus delta, and the closing segment.
func format4(start, end, delta uint16) []byte {
	out := binary.BigEndian.AppendUint16(nil, 4)
	out = binary.BigEndian.AppendUint16(out, 32)
	out = binary.BigEndian.AppendUint16(out, 0)
	out = binary.BigEndian.AppendUint16(out, 4) // two segments
	out = append(out, 0, 0, 0, 0, 0, 0)
	for _, value := range []uint16{end, 0xFFFF, 0, start, 0xFFFF, delta, 1, 0, 0} {
		out = binary.BigEndian.AppendUint16(out, value)
	}
	return out[:32]
}

// post2 is a post table of format 2 naming glyph 0 .. len(names)-1 (standard Macintosh glyph
// indexes below 258 are given as numbers, other names as strings).
func post2(names ...string) []byte {
	out := binary.BigEndian.AppendUint32(nil, 0x00020000)
	out = append(out, make([]byte, 28)...)
	out = binary.BigEndian.AppendUint16(out, uint16(len(names)))
	var strs []byte
	extra := 0
	for _, name := range names {
		if index := slices.Index(macGlyphNames, name); index >= 0 {
			out = binary.BigEndian.AppendUint16(out, uint16(index))
			continue
		}
		out = binary.BigEndian.AppendUint16(out, uint16(258+extra))
		strs = append(append(strs, byte(len(name))), name...)
		extra++
	}
	return append(out, strs...)
}

func maxp(glyphs uint16) []byte {
	return binary.BigEndian.AppendUint16([]byte{0, 1, 0, 0}, glyphs)
}

func TestTrueTypeCmapsAndNamesAreRead(t *testing.T) {
	program, err := ParseTrueType(sfnt(map[string][]byte{"maxp": maxp(5),
		"cmap": cmapTable(subtable{3, 1, format4(65, 66, 0xFFFF-64+4)}, subtable{3, 0, format6(0xF041, 3, 2)}),
		"post": post2(".notdef", "A", "B", "alpha", "uni0394")}))
	if err != nil {
		t.Fatal(err)
	}
	if program.Glyph != 5 || program.Cmaps[[2]int{3, 1}][65] != 4 || program.Cmaps[[2]int{3, 1}][66] != 5 ||
		program.Cmaps[[2]int{3, 0}][0xF042] != 2 {
		t.Fatalf("cmaps %v", program.Cmaps)
	}
	if got := program.Names; !slices.Equal(got, []string{".notdef", "A", "B", "alpha", "uni0394"}) {
		t.Fatalf("names %v", got)
	}
	if _, err := ParseTrueType([]byte("%!PS-AdobeFont")); err == nil {
		t.Fatal("a Type 1 program was read as TrueType")
	}
}

// A glyph several Unicode codes reach takes the smallest outside the private use area, and the
// private use codes never name a character.
func TestUnicodeOfIsDeterministic(t *testing.T) {
	program := &TrueType{Cmaps: map[[2]int]map[int]int{
		{3, 1}: {0x20: 1, 0xA0: 1, 0xF041: 2, 0x41: 3}, {3, 10}: {0x1D400: 3}, {0, 3}: {0x42: 4}}}
	got := program.UnicodeOf()
	if got[1] != 0x20 || got[3] != 0x1D400 || got[4] != 0x42 || got[2] != 0 {
		t.Fatalf("inverse %v", got)
	}
}

// A composite font's CIDs reach their glyphs through the CIDToGIDMap, the glyphs their characters
// through the program's Unicode cmap, then its post names; others read U+FFFD.
func TestCompositeFontTextsComeFromTheProgram(t *testing.T) {
	store := newMemoryStore()
	program := sfnt(map[string][]byte{"maxp": maxp(5), "cmap": cmapTable(subtable{3, 1, format6(0x3B1, 1)}, subtable{1, 0, format6(0x61, 3)}),
		"post": post2(".notdef", "alpha", "mu", "g3", "g4")})
	store.objects["1 0 R"] = map[string]any{"/Subtype": "/Type0", "/Encoding": "/Identity-H", "/DescendantFonts": []any{"2 0 R"}}
	store.objects["2 0 R"] = map[string]any{"/Subtype": "/CIDFontType2", "/BaseFont": "/ABCDEF+Calibri",
		"/CIDToGIDMap": store.AddStream(map[string]any{}, []byte{0, 0, 0, 1, 0, 2, 0, 3, 0, 4, 0, 9}), "/FontDescriptor": "3 0 R"}
	store.objects["3 0 R"] = map[string]any{"/FontFile2": store.AddStream(map[string]any{}, program)}
	fixer := NewFixer(store, testTable())
	if !fixer.Fix("1 0 R", Use{Codes: map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 9: true}}) {
		t.Fatal("no map written")
	}
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, width := ReadCMap(string(data))
	want := map[int]string{1: "03B1", 2: "00B5", 3: replacement, 4: replacement, 5: replacement, 9: replacement}
	if width != 2 || len(mapped) != len(want) {
		t.Fatalf("mapped %v (%d bytes)", mapped, width)
	}
	for code, value := range want {
		if mapped[code] != value {
			t.Errorf("CID %d reads %s, want %s", code, mapped[code], value)
		}
	}
}

// A symbolic TrueType font with no Encoding reads a code through its (3,0) cmap (offset by U+F000)
// or (1,0) cmap, then the glyph's character as above; a code the cmap lacks reads U+FFFD.
func TestUnnamedTrueTypeTextsComeFromTheProgram(t *testing.T) {
	store := newMemoryStore()
	program := sfnt(map[string][]byte{"maxp": maxp(4), "cmap": cmapTable(subtable{3, 0, format6(0xF030, 1, 2)}),
		"post": post2(".notdef", "A", "uni0394", "g3")})
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/TrueType", "/BaseFont": "/ABCDEF+CIDFont+F3",
		"/FontDescriptor": "2 0 R"}
	store.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 4, "/FontFile2": store.AddStream(map[string]any{}, program)}
	fixer := NewFixer(store, testTable())
	if !fixer.Fix("1 0 R", Use{Codes: map[int]bool{0x30: true, 0x31: true, 0x32: true}}) {
		t.Fatal("no map written")
	}
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, _ := ReadCMap(string(data))
	if mapped[0x30] != "0041" || mapped[0x31] != "0394" || mapped[0x32] != replacement || len(mapped) != 3 {
		t.Fatalf("mapped %v", mapped)
	}
}
