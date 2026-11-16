package pipeline

import "testing"

// TestAuthorMathDefault: author math runs by default when Markdown, HTML or EPUB is written
// (decision 187); the switches override it in newConversion.
func TestAuthorMathDefault(t *testing.T) {
	cases := []struct {
		formats []string
		want    bool
	}{
		{[]string{"html"}, true},
		{[]string{"epub"}, true},
		{[]string{"markdown"}, true},
		{[]string{"html", "typst"}, true},
		{[]string{"typst"}, false},
		{[]string{"json"}, false},
		{nil, false},
	}
	for _, test := range cases {
		if got := authorMathDefault(test.formats); got != test.want {
			t.Errorf("formats %v: %v, want %v", test.formats, got, test.want)
		}
	}
}
