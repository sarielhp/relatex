package glyphowner

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	directory := t.TempDir()
	owners := &Owners{SchemaVersion: Version, Leaves: []string{"n-a", "n-b"}, Pages: []Page{{Number: 1,
		X: []float64{10, 20, 30}, Y: []float64{5, 5, 5}, Text: []string{"a", "b", "c"}, Leaf: []int{1, 0, 2}, Unknown: []int{2}}}}
	if err := Write(filepath.Join(directory, File), owners); err != nil {
		t.Fatal(err)
	}
	read, err := Read(directory)
	if err != nil {
		t.Fatal(err)
	}
	page := read.Pages[0]
	for index, want := range []string{"n-a", "", "n-b"} {
		if got := read.Owner(page, index); got != want {
			t.Errorf("glyph %d: owner %q, want %q", index, got, want)
		}
	}
	if read.Owner(page, 7) != "" {
		t.Error("a glyph out of range has an owner")
	}
}

func TestPicturesRoundTripAndInvertedBox(t *testing.T) {
	directory := t.TempDir()
	owners := &Owners{SchemaVersion: Version, Pictures: []Picture{{Node: "n-fig", Page: 2, Box: [4]float64{10, 20, 30, 40}}}}
	if err := Write(filepath.Join(directory, File), owners); err != nil {
		t.Fatal(err)
	}
	read, err := Read(directory)
	if err != nil || len(read.Pictures) != 1 || read.Pictures[0] != owners.Pictures[0] {
		t.Fatalf("pictures %+v, %v", read, err)
	}
	owners.Pictures[0].Box = [4]float64{30, 20, 10, 40}
	if owners.Validate() == nil {
		t.Fatal("an inverted box was accepted")
	}
}

func TestReadMissingAndInvalid(t *testing.T) {
	if _, err := Read(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	directory := t.TempDir()
	broken := &Owners{SchemaVersion: Version, Pages: []Page{{Number: 1, X: []float64{1}, Y: []float64{1, 2}, Text: []string{"a"}, Leaf: []int{0}}}}
	if err := Write(filepath.Join(directory, File), broken); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(directory); err == nil {
		t.Fatal("columns of different lengths were accepted")
	}
}
