package inkdiff

import (
	"image"
	"image/color"
	"testing"
)

// blob is a 40 by 30 white image with a black rectangle at x, y of width w and height h.
func blob(x, y, w, h int) Ink {
	picture := image.NewGray(image.Rect(0, 0, 40, 30))
	for row := range 30 {
		for col := range 40 {
			value := uint8(255)
			if col >= x && col < x+w && row >= y && row < y+h {
				value = 0
			}
			picture.SetGray(col, row, color.Gray{Y: value})
		}
	}
	return FromImage(picture)
}

func TestChanged(t *testing.T) {
	base := blob(10, 10, 8, 6)
	cases := []struct {
		name    string
		other   Ink
		changed int
	}{
		{"identical", blob(10, 10, 8, 6), 0},
		{"moved within the shift", blob(13, 8, 8, 6), 0},
		{"one pixel wider (an edge)", blob(10, 10, 9, 6), 0},
		{"moved beyond the shift", blob(25, 10, 8, 6), 2},
		{"another shape", blob(10, 10, 3, 14), 1},
		{"missing", blob(0, 0, 0, 0), 1},
		{"another size", FromImage(image.NewGray(image.Rect(0, 0, 41, 30))), 1},
	}
	for _, test := range cases {
		if got := Changed(base, test.other); len(got) != test.changed {
			t.Errorf("%s: %d changed regions %v, want %d", test.name, len(got), got, test.changed)
		}
	}
}

func TestFromImageCompositesOnWhite(t *testing.T) {
	picture := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	picture.SetNRGBA(0, 0, color.NRGBA{A: 0})
	picture.SetNRGBA(1, 0, color.NRGBA{A: 255})
	ink := FromImage(picture)
	if ink.at(0, 0) || !ink.at(1, 0) || ink.Count() != 1 {
		t.Errorf("transparent pixel is ink or opaque black is not: %v", ink.dark)
	}
	if cropped := ink.Crop(1, 1); cropped.Count() != 0 {
		t.Errorf("crop %v", cropped.dark)
	}
}
