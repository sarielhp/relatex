package fontmodel

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Page decorations name fonts (Spans) but the reading page shows none, so the browser's
// presentation spans leave them out (decision 016, amendment 2026-11-17).
func TestPresentationSpansLeaveOutPageDecorations(t *testing.T) {
	run := rec.Map{"text": "1", "style": rec.Map{"family": "lmr"}}
	document := rec.Map{"block_order": []any{}, "nodes": rec.Map{}, "note_order": []any{},
		"page_decorations": []any{rec.Map{"id": "n-1", "runs": []any{run}}}}
	if got := len(Spans(document)); got != 1 {
		t.Errorf("Spans = %d, want 1", got)
	}
	if got := len(PresentationSpans(document)); got != 0 {
		t.Errorf("PresentationSpans = %d, want 0", got)
	}
}
