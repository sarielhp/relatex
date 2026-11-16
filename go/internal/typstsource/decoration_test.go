package typstsource

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A decorated run keeps its line (decision 134): text-mode \underline and soul's \ul are
// #underline[], soul's \st is #strike[].
func TestStyledDecoratesUnderlinedAndStruckRuns(t *testing.T) {
	e := &exporter{}
	cases := map[string]string{"underline": "#underline[word]", "strike": "#strike[word]", "": "word"}
	for decoration, want := range cases {
		span := rec.Map{"type": "text", "text": "word"}
		if decoration != "" {
			span["decoration"] = decoration
		}
		if got := e.styled("word", styleOf(span, false), nil); got != want {
			t.Errorf("%q: styled = %s, want %s", decoration, got, want)
		}
	}
}
