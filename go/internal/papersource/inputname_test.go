package papersource

import "testing"

// TeX runs in the main file's directory: the files its source names are relative to it.
func TestInputNamesAreRelativeToTheMainFile(t *testing.T) {
	deck := &Source{main: "slides/deck.tex"}
	cases := map[string]string{"../preamble": "preamble", "macros.tex": "slides/macros.tex", "./parts/a": "slides/parts/a",
		"../../outside": "", "../..": ""}
	for name, want := range cases {
		if got := deck.inputName(name); got != want {
			t.Errorf("inputName(%q) = %q, want %q", name, got, want)
		}
	}
	paper := &Source{main: "paper.tex"}
	if got := paper.inputName("./sections/abstract"); got != "./sections/abstract" {
		t.Errorf("a main file at the root names files as written: %q", got)
	}
}
