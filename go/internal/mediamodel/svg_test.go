package mediamodel

import "testing"

func TestExternalURL(t *testing.T) {
	cases := map[string]bool{"url(#a)": false, "url('#a')": true, "url( #a)": true, "fill:url(http://x)": true,
		"URL(x)": true, "none": false, "url(": true}
	for value, want := range cases {
		if got := externalURL(value); got != want {
			t.Errorf("externalURL(%q) = %v, want %v", value, got, want)
		}
	}
}
