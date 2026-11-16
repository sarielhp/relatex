package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func failsColor(span rec.Map) (failed bool) {
	defer func() { failed = recover() != nil }()
	validateSpanColor(span, span["type"])
	return false
}

func TestSpanColorIsAnOptionalNonBlackHexOnTextAndReferenceSpans(t *testing.T) {
	for _, span := range []rec.Map{
		{"type": "text", "text": "a"},
		{"type": "text", "text": "a", "color": "#ff0000"},
		{"type": "reference", "text": "1", "color": "#0000ff"},
		{"type": "rule", "color": "#123456"},
	} {
		if failsColor(span) {
			t.Errorf("%v was rejected", span)
		}
	}
	for _, span := range []rec.Map{
		{"type": "text", "color": "#000000"},
		{"type": "text", "color": "#FF0000"},
		{"type": "text", "color": "red"},
		{"type": "text", "color": 7},
		{"type": "math", "color": "#ff0000"},
		{"type": "image", "color": "#ff0000"},
	} {
		if !failsColor(span) {
			t.Errorf("%v was accepted", span)
		}
	}
}
