package paperdoc

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A frame title a theme draws in a picture is reported once for the deck, with the count of such
// frames, as a warning (decision 128, d); a deck without one reports nothing.
func TestDrawnFrameTitlesReportOneWarning(t *testing.T) {
	b := degradeBuilder(t)
	rows := []rec.Map{{"code": drawnTitleCode}, {"code": "caption_context"}, {"code": drawnTitleCode}}
	b.reportDrawnTitles(rows)
	lines := strings.Join(b.diagnostics.Lines("whatever"), "\n")
	if !strings.Contains(lines, "heading.drawn_in_picture") || !strings.Contains(lines, "2 frame titles") {
		t.Fatalf("diagnostics: %s", lines)
	}
	if !strings.Contains(lines, "warning") {
		t.Fatalf("drawn frame titles are a warning: %s", lines)
	}
	plain := degradeBuilder(t)
	plain.reportDrawnTitles([]rec.Map{{"code": "caption_context"}})
	if len(plain.diagnostics.Entries()) != 0 {
		t.Fatalf("no drawn title, no diagnostic: %v", plain.diagnostics.Entries())
	}
}
