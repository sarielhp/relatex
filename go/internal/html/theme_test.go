package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The page follows the reader's system theme unless the toggle chose one: the light palette and
// the light inks apply under prefers-color-scheme: light except when data-theme="dark".
func TestThemeFollowsTheSystemUnlessChosen(t *testing.T) {
	system := `@media (prefers-color-scheme: light) { :root:not([data-theme="dark"]) { `
	if !strings.Contains(pageStyle, system+lightTheme+" } }") || !strings.Contains(pageStyle, `:root[data-theme="light"] { `+lightTheme) {
		t.Errorf("the page style does not follow the system theme")
	}
	css := inksCSS(rec.Map{"text_paint": rec.Map{"inks": []any{"#000080"}}})
	if !strings.Contains(css, system+"--ink-000080: #000080; } }") {
		t.Errorf("the light inks do not follow the system theme: %s", css)
	}
}
