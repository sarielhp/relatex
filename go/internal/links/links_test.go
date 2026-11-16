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
