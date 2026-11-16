package tablelayout

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// PanelReason names the first row that keeps a grid of figures a table, and nothing for a layout or a
// grid without pictures.
func TestPanelReasonNamesTheBreakingRow(t *testing.T) {
	cases := []struct {
		name string
		grid rec.Map
		want string
	}{
		{"text first", panelCells([]string{"t", "t"}, []string{"p", "p"}), "row 1 is text"},
		{"a picture beside text", panelCells([]string{"p", "p"}, []string{"p", "t"}), "row 2 mixes pictures with text"},
		{"a layout", panelCells([]string{"p", "p"}, []string{"t", "t"}), ""},
		{"no pictures", panelCells([]string{"t", "t"}, []string{"t", "t"}), ""},
	}
	for _, c := range cases {
		got := PanelReason(c.grid)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
