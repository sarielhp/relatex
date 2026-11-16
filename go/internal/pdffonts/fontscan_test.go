package pdffonts

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Windows has no fc-list (decision 129): the font folders are scanned and each face listed as
// fc-list lists it, a collection's faces with their index.
func TestScanFontsListsEveryFaceAsFcListDoes(t *testing.T) {
	directory := t.TempDir()
	ttc := testTTC([]map[string][]byte{
		{"head": headTable(1), "name": namesTable(map[uint16]string{1: "Fake", 2: "Regular", 6: "Fake-Regular", 16: "Fake Pro"})},
		{"head": headTable(2), "name": namesTable(map[uint16]string{1: "Fake", 2: "Bold", 6: "Fake-Bold"})},
	})
	single, err := extractFace(ttc, 1)
	if err != nil {
		t.Fatal(err)
	}
	collection, nested := filepath.Join(directory, "fonts.ttc"), filepath.Join(directory, "nested", "single.TTF")
	for path, data := range map[string][]byte{collection: ttc, nested: single, filepath.Join(directory, "notes.txt"): []byte("x"),
		filepath.Join(directory, "broken.otf"): []byte("not a font")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := scanFonts([]string{directory, filepath.Join(directory, "missing")})
	want := []fcEntry{
		{file: collection, face: 0, families: []string{"Fake Pro", "Fake"}, styles: []string{"Regular"}, postscript: "Fake-Regular"},
		{file: collection, face: 1, families: []string{"Fake"}, styles: []string{"Bold"}, postscript: "Fake-Bold"},
		{file: nested, face: 0, families: []string{"Fake"}, styles: []string{"Bold"}, postscript: "Fake-Bold"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scanned %+v, want %+v", got, want)
	}
}
