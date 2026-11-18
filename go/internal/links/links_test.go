package links

import "testing"

// \href{mailto:}{name@host} (third cohort 718) names no address: the link is malformed, and the
// conversion goes on with its text read plain.
func TestAnEmptyMailtoIsMalformed(t *testing.T) {
	reason, err := SafeHref("mailto:")
	if err != nil || reason != "Malformed external link: missing opaque part for mailto URL" {
		t.Fatalf("SafeHref(mailto:) = %q, %v", reason, err)
	}
	if reason, err := SafeHref("mailto:a@b.org"); err != nil || reason != "" {
		t.Fatalf("SafeHref(mailto:a@b.org) = %q, %v", reason, err)
	}
}

// A TeX escape in a link target is the character it escapes (25/rect_net's DOI, 2001-2\_9): the
// href is followed, not rejected as a malformed address.
func TestTargetHrefResolvesTeXEscapes(t *testing.T) {
	cases := map[string]string{
		`http://dx.doi.org/10.1007/978-1-4684-2001-2\_9`: "http://dx.doi.org/10.1007/978-1-4684-2001-2_9",
		`https://x.org/a\#b\%20c\&d\~e`:                  "https://x.org/a#b%20c&d~e",
		"https://x.org/plain":                            "https://x.org/plain",
	}
	for given, want := range cases {
		got := TargetHref(given)
		if got != want {
			t.Errorf("TargetHref(%q) = %q, want %q", given, got, want)
		}
		if reason, err := SafeHref(EncodedHref(got)); err != nil || reason != "" {
			t.Errorf("SafeHref(%q) = %q, %v", got, reason, err)
		}
	}
}
