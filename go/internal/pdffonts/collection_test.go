package pdffonts

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// collection builds a TrueType collection of the fonts given: both directories first, then the
// tables, with every table offset counted from the file start as in a real .ttc.
func collection(fonts ...[]byte) []byte {
	header := 12 + 4*len(fonts)
	directories, at := make([]int, len(fonts)), header
	for i, font := range fonts {
		directories[i] = at
		at += 12 + 16*int(binary.BigEndian.Uint16(font[4:]))
	}
	out := append([]byte("ttcf"), 0, 1, 0, 0)
	out = binary.BigEndian.AppendUint32(out, uint32(len(fonts)))
	for _, offset := range directories {
		out = binary.BigEndian.AppendUint32(out, uint32(offset))
	}
	var tables []byte
	for _, font := range fonts {
		size := 12 + 16*int(binary.BigEndian.Uint16(font[4:]))
		directory := bytes.Clone(font[:size])
		for i := 12; i < size; i += 16 {
			old := int(binary.BigEndian.Uint32(directory[i+8:]))
			binary.BigEndian.PutUint32(directory[i+8:], uint32(old-size+at+len(tables)))
		}
		out = append(out, directory...)
		tables = append(tables, font[size:]...)
	}
	return append(out, tables...)
}

func TestCollectionFaceReadsTheFontAsked(t *testing.T) {
	first := sfnt(map[string][]byte{"maxp": maxp(3), "post": post2("a")})
	second := sfnt(map[string][]byte{"maxp": maxp(7)})
	ttc := collection(first, second)
	if !isCollection(ttc) || isSfnt(ttc) {
		t.Fatal("a collection is not read as one font")
	}
	for face, want := range map[int][]byte{0: maxp(3), 1: maxp(7)} {
		font := collectionFace(ttc, face)
		if !isSfnt(font) {
			t.Fatalf("face %d is not an sfnt", face)
		}
		if got := tableDirectory(font)["maxp"]; !bytes.Equal(got, want) {
			t.Errorf("face %d maxp = %v, want %v", face, got, want)
		}
	}
	if collectionFace(ttc, 2) != nil || collectionFace(ttc, -1) != nil {
		t.Error("a face outside the collection is read")
	}
}
