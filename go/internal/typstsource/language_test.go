package typstsource

import "testing"

func TestTypstLanguageSplitsTheRegion(t *testing.T) {
	cases := map[string]string{"de": `lang: "de"`, "de-AT": `lang: "de", region: "AT"`, "grc": `lang: "grc"`}
	for tag, want := range cases {
		if got := typstLanguage(tag); got != want {
			t.Errorf("typstLanguage(%q) = %s; want %s", tag, got, want)
		}
	}
}

// A foreign run is set in its language.
func TestStyledSetsTheRunsLanguage(t *testing.T) {
	e := &exporter{}
	if got := e.styled("Bonjour", textStyle{lang: "fr"}, nil); got != `#text(lang: "fr")[Bonjour]` {
		t.Errorf("styled = %s", got)
	}
}
