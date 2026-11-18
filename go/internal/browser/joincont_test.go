package browser

import (
	"strings"
	"testing"
)

// A text piece cut after a formula's punctuation (data-join-cont) is part of the run before it:
// no style inventory counts it, so the styled-run count stays the document's.
func TestStyleInventoriesSkipJoinContinuations(t *testing.T) {
	for name, script := range map[string]string{"styles": textStylesScript, "platform fonts": platformFontsScript} {
		if !strings.Contains(script, "[data-text-style]:not([data-join-cont])") {
			t.Errorf("%s script counts a continuation piece: %s", name, script)
		}
	}
}
