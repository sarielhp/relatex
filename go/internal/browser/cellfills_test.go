package browser

import (
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A cell with a colortbl fill is marked as a fill box and painted in it by the light theme, the
// PDF's paint; any other cell keeps the plain markup and no background.
func TestRichCellFillMarkupAndBackground(t *testing.T) {
	filled := rec.Map{"fill": "#1f3864"}
	attributes := richCellMarkup(filled)
	if !slices.Contains(attributes, any("data-fill-box")) || len(attributes) != len(richCellAttributes)+1 ||
		!slices.IsSortedFunc(attributes, func(a, b any) int { return strings.Compare(rec.ToS(a), rec.ToS(b)) }) {
		t.Errorf("filled cell attributes = %v", attributes)
	}
	if cellBackground(filled) != "rgb(31, 56, 100)" {
		t.Errorf("background = %s", cellBackground(filled))
	}
	plain := rec.Map{}
	if !slices.Equal(richCellMarkup(plain), richCellAttributes) || cellBackground(plain) != "rgba(0, 0, 0, 0)" {
		t.Errorf("plain cell = %v %s", richCellMarkup(plain), cellBackground(plain))
	}
}
