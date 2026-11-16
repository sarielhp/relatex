package wcag

import (
	"slices"
	"testing"
)

func TestColourNamesInProse(t *testing.T) {
	cases := map[string][]string{
		"The red line is the bound; blue dots are samples.":  {"red", "blue"},
		`Points shown in \textcolor{red}{bold} are optimal.`: {},
		"Grey and gray regions, Red again.":                  {"gray", "red"},
		"A red-black tree and Green's function.":             {},
		"Gray code order; a reddish area.":                   {"red"},
		"Black box and white noise.":                         {},
	}
	for text, want := range cases {
		if got := ColourNames(text); !slices.Equal(got, want) {
			t.Errorf("ColourNames(%q) = %v, want %v", text, got, want)
		}
	}
}

// A colour word of an idiom that names a thing is no colour to look for: the red carpet of a photo.
func TestColourNamesSkipIdioms(t *testing.T) {
	cases := map[string][]string{
		"Follow the red carpet to the door.":     {},
		"A red herring; red tape; the red line.": {"red"},
		"The red carpet is red.":                 {"red"},
	}
	for text, want := range cases {
		if got := ColourNames(text); !slices.Equal(got, want) {
			t.Errorf("ColourNames(%q) = %v, want %v", text, got, want)
		}
	}
}
