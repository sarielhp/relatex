package links

import "testing"

func TestOrcidName(t *testing.T) {
	cases := map[string]string{
		"https://orcid.org/0000-0001-9416-1742":     "ORCID iD 0000-0001-9416-1742",
		"http://www.orcid.org/0009-0003-3240-830X/": "ORCID iD 0009-0003-3240-830X",
		"https://orcid.org/my-profile":              "ORCID",
	}
	for href, want := range cases {
		if got := OrcidName(href); got != want {
			t.Errorf("OrcidName(%q) = %q, want %q", href, got, want)
		}
	}
}

func TestOrcidLogo(t *testing.T) {
	const pt = 65_536
	if !OrcidLogo("https://orcid.org/0000-0001-9416-1742", 7*pt, 6*pt, pt) {
		t.Error("a 7 pt picture in an ORCID link is the logo")
	}
	if OrcidLogo("https://orcid.org/0000-0001-9416-1742", 100*pt, 80*pt, 0) {
		t.Error("a figure-sized picture is no logo")
	}
	if OrcidLogo("https://example.org/orcid.org/", 7*pt, 7*pt, 0) {
		t.Error("only a link to orcid.org holds the logo")
	}
	if OrcidLogo(nil, 7*pt, 7*pt, 0) {
		t.Error("no link, no logo")
	}
}
