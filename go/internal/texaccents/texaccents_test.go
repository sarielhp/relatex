package texaccents

import (
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestCombiningComposes(t *testing.T) {
	cases := map[string]string{"'e": "é", `"o`: "ö", "ve": "ě", "cc": "ç", "Ho": "ő", "ka": "ą"}
	for in, want := range cases {
		got := norm.NFC.String(in[len(in)-1:] + Combining[in[:len(in)-1]])
		if got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	if len(Combining) != 15 {
		t.Fatalf("Combining has %d accents, want 15", len(Combining))
	}
}
