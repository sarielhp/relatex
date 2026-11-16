package typstsource

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A heading set in several lines (decision 126) is written with Typst's line break after the
// line-break span's space: "Appendix A \ Proof".
func TestHeadingLineBreakIsATypstLineBreak(t *testing.T) {
	spans := []rec.Map{
		{"type": "text", "text": "Appendix A"},
		{"type": "text", "text": " ", "line_break": true},
		{"type": "text", "text": "Proof"},
	}
	e := &exporter{}
	if got, want := e.inline(spans, rec.Map{"id": "h"}, 0, false), `Appendix A \ Proof`; got != want {
		t.Errorf("inline = %q, want %q", got, want)
	}
}
