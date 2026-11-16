package pdfpaint

import "testing"

func TestSupportedVersion(t *testing.T) {
	cases := map[string]bool{
		"mutool version 1.28.2":  true,
		"mutool version 1.28.5":  true,
		"mutool version 1.28.12": true,
		"mutool version 1.28.":   false,
		"mutool version 1.29.0":  false,
		"mutool version 1.27.2":  false,
		"mutool version 1.28.2a": false,
		"mutool version 1.280.1": false,
		"":                       false,
	}
	for line, want := range cases {
		if got := SupportedVersion(line); got != want {
			t.Errorf("SupportedVersion(%q) = %v, want %v", line, got, want)
		}
	}
}
