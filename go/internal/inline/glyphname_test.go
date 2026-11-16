package inline

import "testing"

func TestGlyphLabelSpeaksTheNamesWords(t *testing.T) {
	cases := map[string]string{
		"Tumbler": "Tumbler", "WashCotton": "Wash cotton", "MVDivision": "MV division", "EUR": "EUR",
		"Wash30deg": "Wash 30 deg", "Mmapsto": "Double maps to bar", "dblbracketlefttp": "Double bracket left top",
		"Ydown": "Y down", "solidmultimap": "Solid multimap", "uni2318.alt": "uni 2318 alt", "NoChemicalCleaning": "No chemical cleaning",
	}
	for name, want := range cases {
		if got := GlyphLabel(name); got != want {
			t.Errorf("GlyphLabel(%q) = %q, want %q", name, got, want)
		}
	}
}
