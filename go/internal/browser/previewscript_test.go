package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The page's own preview.js is the one script the inventory discounts; anything else is unsafe.
func TestPreviewScriptsDiscountOnlyThePagesOwnScript(t *testing.T) {
	if got := previewScripts(rec.Map{"scripts": []any{"preview.js"}}); len(got) != 1 || got[0] != "preview.js" {
		t.Errorf("own script: %v", got)
	}
	for _, scripts := range [][]any{{}, {"other.js"}, {"preview.js", "other.js"}, {"https://x/preview.js"}} {
		if got := previewScripts(rec.Map{"scripts": scripts}); got != nil {
			t.Errorf("%v discounted: %v", scripts, got)
		}
	}
	if got := previewScripts(rec.Map{}); got != nil {
		t.Errorf("no scripts field: %v", got)
	}
}
