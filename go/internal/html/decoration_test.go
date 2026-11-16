package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A decorated text span (text-mode \underline and soul's \ul underline, soul's \st strikes out;
// decision 134) carries its decoration twice: as CSS the reader sees and as data-decoration the
// browser inventory reads back, and checks against the painted line. A plain span carries neither.
func TestRenderSpanDecoratesUnderlinedAndStruckText(t *testing.T) {
	paper := &Paper{document: rec.Map{}, nodes: rec.Map{}}
	for decoration, css := range map[string]string{"underline": "text-decoration:underline", "strike": "text-decoration:line-through"} {
		got := paper.renderSpan(rec.Map{"type": "text", "text": "word", "decoration": decoration})
		if !strings.Contains(got, `data-decoration="`+decoration+`"`) || !strings.Contains(got, css) {
			t.Errorf("%s span = %s", decoration, got)
		}
	}
	if plain := paper.renderSpan(rec.Map{"type": "text", "text": "word"}); strings.Contains(plain, "decoration") {
		t.Errorf("plain span = %s", plain)
	}
}
