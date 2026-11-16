package pdffonts

import (
	"bytes"
	"encoding/binary"
	"maps"
	"slices"
	"testing"
)

// namesTable is an sfnt name table with a Windows (3/1) UTF-16BE record per name ID.
func namesTable(names map[uint16]string) []byte {
	ids := slices.Sorted(maps.Keys(names))
	var records, values []byte
	offset := 0
	for _, id := range ids {
		var value []byte
		for _, r := range names[id] {
			value = binary.BigEndian.AppendUint16(value, uint16(r))
		}
		for _, field := range []int{3, 1, 0x409, int(id), len(value), offset} {
			records = binary.BigEndian.AppendUint16(records, uint16(field))
		}
		values = append(values, value...)
		offset += len(value)
	}
	out := binary.BigEndian.AppendUint16(nil, 0)
	out = binary.BigEndian.AppendUint16(out, uint16(len(ids)))
	out = binary.BigEndian.AppendUint16(out, uint16(6+12*len(ids)))
	return append(append(out, records...), values...)
}

// headTable is an sfnt head table of 54 bytes with a zero checkSumAdjustment.
func headTable(revision uint32) []byte {
	out := binary.BigEndian.AppendUint32(nil, 0x00010000)
	out = binary.BigEndian.AppendUint32(out, revision)
	out = binary.BigEndian.AppendUint32(out, 0)
	out = binary.BigEndian.AppendUint32(out, 0x5F0F3CF5)
	out = binary.BigEndian.AppendUint16(out, 0)
	out = binary.BigEndian.AppendUint16(out, 1000)
	return append(out, make([]byte, 54-len(out))...)
}

// ttcChecksum is a table's checksum as a collection's directory records it, computed apart from
// the code under test.
func ttcChecksum(tag string, data []byte) uint32 {
	if tag == "head" && len(data) >= 12 {
		copied := bytes.Clone(data)
		clear(copied[8:12])
		data = copied
	}
	var sum uint32
	for at := 0; at < len(data); at += 4 {
		var word [4]byte
		copy(word[:], data[at:])
		sum += binary.BigEndian.Uint32(word[:])
	}
	return sum
}

// testTTC builds a collection of the faces given (tag -> table bytes); the tags in shared are
// stored once and every face's record points at that copy, as Cambria's do.
func testTTC(faces []map[string][]byte, shared ...string) []byte {
	tags := make([][]string, len(faces))
	header := 12 + 4*len(faces)
	directories := header
	for index, face := range faces {
		tags[index] = slices.Sorted(maps.Keys(face))
		directories += 12 + 16*len(tags[index])
	}
	out := append([]byte("ttcf"), 0, 1, 0, 0)
	out = binary.BigEndian.AppendUint32(out, uint32(len(faces)))
	at := header
	for index := range faces {
		out = binary.BigEndian.AppendUint32(out, uint32(at))
		at += 12 + 16*len(tags[index])
	}
	placed, offsets, tables := map[string]int{}, make([][]int, len(faces)), []byte{}
	for index, face := range faces {
		offsets[index] = make([]int, len(tags[index]))
		for i, tag := range tags[index] {
			if offset, ok := placed[tag]; ok {
				offsets[index][i] = offset
				continue
			}
			offset := directories + len(tables)
			if slices.Contains(shared, tag) {
				placed[tag] = offset
			}
			offsets[index][i] = offset
			tables = append(tables, face[tag]...)
			tables = append(tables, make([]byte, (4-len(tables)%4)%4)...)
		}
	}
	for index, face := range faces {
		out = binary.BigEndian.AppendUint32(out, 0x00010000)
		out = binary.BigEndian.AppendUint16(out, uint16(len(tags[index])))
		out = append(out, 0, 0, 0, 0, 0, 0)
		for i, tag := range tags[index] {
			out = append(out, tag...)
			out = binary.BigEndian.AppendUint32(out, ttcChecksum(tag, face[tag]))
			out = binary.BigEndian.AppendUint32(out, uint32(offsets[index][i]))
			out = binary.BigEndian.AppendUint32(out, uint32(len(face[tag])))
		}
	}
	return append(out, tables...)
}

// fakeTTC is a two-face collection: "Fake" and "Fake Math", one table shared.
func fakeTTC() []byte {
	return testTTC([]map[string][]byte{
		{"head": headTable(0x00010000), "name": namesTable(map[uint16]string{4: "Fake", 6: "Fake"}),
			"glyf": []byte{1, 2, 3}, "loca": []byte{1, 2, 3, 4}, "cmap": sharedTable},
		{"head": headTable(0x00020000), "name": namesTable(map[uint16]string{4: "Fake Math", 6: "FakeMath"}),
			"glyf": []byte{4, 5, 6, 7}, "loca": []byte{5, 6, 7, 8}, "cmap": sharedTable},
	}, "cmap")
}

// sharedTable is a table both faces of fakeTTC point at; five bytes, so its checksum is not also
// its byte sequence.
var sharedTable = []byte{0x91, 0x92, 0x93, 0x94, 0x95}

func TestCollectionFaceNamedMatchesTheFaceExactly(t *testing.T) {
	ttc := fakeTTC()
	for base, want := range map[string]int{"ABCDEF+FakeMath": 1, "FakeMath": 1, "Fake Math": 1, "Fake": 0,
		"Fake-Identity-H": 0, "ABCDEF+Fake": 0} {
		if face, ok := collectionFaceNamed(ttc, base); !ok || face != want {
			t.Errorf("collectionFaceNamed(%q) = %d, %v; want face %d", base, face, ok, want)
		}
	}
	for _, base := range []string{"FakeMat", "Other", "", "Fake Math Extra"} {
		if face, ok := collectionFaceNamed(ttc, base); ok {
			t.Errorf("collectionFaceNamed(%q) = %d, want no face", base, face)
		}
	}
	if name := faceName(ttc, 1); name != "Fake Math" || faceCount(ttc) != 2 {
		t.Errorf("face 1 is %q of %d faces", name, faceCount(ttc))
	}
	if _, ok := collectionFaceNamed(fakeTTCWithNames("Fake"), "Fake"); ok {
		t.Error("two faces with the same name were read as one")
	}
	if _, err := extractFace([]byte("nope"), 0); err == nil {
		t.Error("a non-collection was extracted")
	}
}

// fakeTTCWithNames is a collection of two faces both named name.
func fakeTTCWithNames(name string) []byte {
	return testTTC([]map[string][]byte{
		{"head": headTable(1), "name": namesTable(map[uint16]string{4: name, 6: name})},
		{"head": headTable(2), "name": namesTable(map[uint16]string{4: name, 6: name})},
	})
}

func TestExtractFaceIsAStandaloneSfnt(t *testing.T) {
	ttc := fakeTTC()
	face, err := extractFace(ttc, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isSfnt(face) || isCollection(face) || binary.BigEndian.Uint32(face) != 0x00010000 {
		t.Fatalf("the face is not a standalone sfnt: % x", face[:4])
	}
	tables := tableDirectory(face)
	if len(tables) != 5 {
		t.Fatalf("the face has %d tables, want 5", len(tables))
	}
	want := map[string][]byte{"head": headTable(0x00020000), "name": namesTable(map[uint16]string{4: "Fake Math", 6: "FakeMath"}),
		"glyf": {4, 5, 6, 7}, "loca": {5, 6, 7, 8}, "cmap": sharedTable}
	for tag, data := range want {
		got := tables[tag]
		if tag == "head" {
			if len(got) != len(data) || !bytes.Equal(got[:8], data[:8]) || !bytes.Equal(got[12:], data[12:]) {
				t.Errorf("head differs outside bytes 8..11: % x, want % x", got, data)
			}
			continue
		}
		if !bytes.Equal(got, data) {
			t.Errorf("table %s = % x, want % x", tag, got, data)
		}
	}
	count, tags := int(binary.BigEndian.Uint16(face[4:])), []string{}
	for index := 0; index < count; index++ {
		entry := face[12+16*index:]
		tags = append(tags, string(entry[:4]))
		if offset := int(binary.BigEndian.Uint32(entry[8:])); offset%4 != 0 {
			t.Errorf("table %s starts at %d, not 4-byte aligned", entry[:4], offset)
		}
	}
	if !slices.IsSorted(tags) {
		t.Errorf("table records are not sorted: %v", tags)
	}
	if bytes.Count(face, sharedTable) != 1 {
		t.Error("the shared table is not copied exactly once")
	}
	var sum uint32
	for at := 0; at < len(face); at += 4 {
		var word [4]byte
		copy(word[:], face[at:])
		sum += binary.BigEndian.Uint32(word[:])
	}
	if sum != 0xB1B0AFBA {
		t.Errorf("whole-file checksum %#08x, want 0xB1B0AFBA", sum)
	}
}

func TestExtractFaceRejectsBrokenInput(t *testing.T) {
	ttc := fakeTTC()
	if _, err := extractFace(ttc, 2); err == nil {
		t.Error("a face past the end was extracted")
	}
	if _, err := extractFace(ttc, -1); err == nil {
		t.Error("a negative face was extracted")
	}
	if _, err := extractFace(ttc[:len(ttc)-2], 1); err == nil {
		t.Error("a truncated collection was extracted")
	}
	broken := bytes.Clone(ttc)
	at := int(binary.BigEndian.Uint32(broken[16:]))        // face 1's directory
	binary.BigEndian.PutUint32(broken[at+12+8:], 0xFFFFFF) // its first table's offset, past the end
	if _, err := extractFace(broken, 1); err == nil {
		t.Error("a table outside the file was extracted")
	}
}

// collectionFixture is a Type0/CIDFontType2 font whose /FontFile2 is ttc, with another font's
// BaseFont to name.
func collectionFixture(store *memoryStore, base string, ttc []byte) {
	store.objects["10 0 R"] = map[string]any{"/Length1": float64(len(ttc)), "/Filter": "/FlateDecode"}
	store.data["10 0 R"] = ttc
	store.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 4.0, "/FontFile2": "10 0 R"}
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type0", "/Encoding": "/Identity-H",
		"/BaseFont": "/" + base, "/DescendantFonts": []any{"3 0 R"}, "/ToUnicode": "20 0 R"}
	store.objects["3 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/CIDFontType2", "/BaseFont": "/" + base,
		"/FontDescriptor": "2 0 R", "/CIDToGIDMap": "/Identity"}
	store.data["20 0 R"] = []byte("1 begincodespacerange <0000> <FFFF> endcodespacerange")
}

func TestPrepareUnwrapsACollectionAsTheFaceItsFontNames(t *testing.T) {
	ttc := fakeTTC()
	want, err := extractFace(ttc, 1)
	if err != nil {
		t.Fatal(err)
	}
	store := newMemoryStore()
	collectionFixture(store, "ABCDEF+FakeMath", ttc)
	fixer := NewFixer(store, testTable())
	fixer.Prepare([]string{"1 0 R"})
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{}})
	data, err := store.StreamData("10 0 R")
	if err != nil || !bytes.Equal(data, want) {
		t.Fatalf("the stream is %d bytes (%v), want the Fake Math face (%d bytes)", len(data), err, len(want))
	}
	dict := store.objects["10 0 R"].(map[string]any)
	if dict["/Length1"] != len(want) || dict["/Filter"] != nil {
		t.Errorf("the stream's dictionary is %v, want /Length1 %d and no filter", dict, len(want))
	}
	if store.objects["1 0 R"].(map[string]any)["/ToUnicode"] != "20 0 R" {
		t.Error("the font's /ToUnicode changed")
	}
	if len(fixer.Report.Unwrapped) != 1 {
		t.Fatalf("report %+v", fixer.Report.Unwrapped)
	}
	found := fixer.Report.Unwrapped[0]
	if found.Font != "ABCDEF+FakeMath" || found.Face != 1 || found.Name != "Fake Math" || found.Faces != 2 ||
		found.Before != len(ttc) || found.After != len(want) {
		t.Errorf("unwrapped %+v", found)
	}
}

func TestPrepareKeepsACollectionNoFaceNames(t *testing.T) {
	ttc := fakeTTC()
	store := newMemoryStore()
	collectionFixture(store, "ABCDEF+Unknown", ttc)
	fixer := NewFixer(store, testTable())
	fixer.Prepare([]string{"1 0 R"})
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{}})
	if data, _ := store.StreamData("10 0 R"); !bytes.Equal(data, ttc) {
		t.Error("the collection was replaced for a font that names no face")
	}
	if len(fixer.Report.Unwrapped) != 0 {
		t.Errorf("report %+v", fixer.Report.Unwrapped)
	}
}

// A FontFile2 shared by two fonts naming different faces stays whole: neither font's face wins.
func TestPrepareKeepsACollectionTwoFontsNameDifferently(t *testing.T) {
	ttc := fakeTTC()
	store := newMemoryStore()
	collectionFixture(store, "ABCDEF+FakeMath", ttc)
	store.objects["30 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/TrueType", "/BaseFont": "/ABCDEF+Fake",
		"/FontDescriptor": "2 0 R"}
	fixer := NewFixer(store, testTable())
	fixer.Prepare([]string{"1 0 R", "30 0 R"})
	if data, _ := store.StreamData("10 0 R"); !bytes.Equal(data, ttc) {
		t.Error("the shared collection was replaced although its fonts name different faces")
	}
	if len(fixer.Report.Unwrapped) != 0 {
		t.Errorf("report %+v", fixer.Report.Unwrapped)
	}
}
