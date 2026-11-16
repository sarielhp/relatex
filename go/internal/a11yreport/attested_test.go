package a11yreport

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A claim held with attested colour captions says so on the page: the captions are listed with their
// keys, as the author's word and not as relatex's finding (decision 188).
func TestPageListsAttestedColourCaptions(t *testing.T) {
	web := rec.Map{"status": "measured", "claim": "WCAG 2.2 AA", "open_items": []any{}, "structural": []any{
		rec.Map{"check": "use_of_color", "status": "attested", "count": int64(0), "items": []any{}, "attested": []any{
			rec.Map{"key": "fig:traces", "file": "main.tex", "line": int64(41), "text": "Figure 4: measured (solid blue) and reconstructed (dashed red)."}}}}}
	page := Page(Inputs{Document: reportDocument(), Web: web})
	for _, want := range []string{"Captions attested by the author", "<code>fig:traces</code>", "main.tex:41", "relatex did not verify it"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if plain := Page(Inputs{Document: reportDocument(), Web: openWeb()}); strings.Contains(plain, "attested by the author") {
		t.Error("a page without attestations names none")
	}
}
