package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// TestFormulaLinkNamesItsLabelsTarget: a formula inside a \hyperref text carries the link's
// label and that label's target; another target, a missing half or a link on text is invalid.
func TestFormulaLinkNamesItsLabelsTarget(t *testing.T) {
	v := &validation{data: rec.Map{"labels": rec.Map{"dfn:a": rec.Map{"target": "n-1"}}}}
	v.validateMathLink(rec.Map{"type": "math", "node": "m", "link_label": "dfn:a", "link_target": "n-1"}, "math")
	v.validateMathLink(rec.Map{"type": "math", "node": "m"}, "math")
	for _, span := range []rec.Map{
		{"type": "math", "link_label": "dfn:a", "link_target": "n-2"},
		{"type": "math", "link_label": "dfn:a"},
		{"type": "math", "link_label": "dfn:b", "link_target": "n-1"},
		{"type": "text", "link_label": "dfn:a", "link_target": "n-1"},
	} {
		func() {
			defer func() {
				if _, ok := recover().(rec.Failure); !ok {
					t.Errorf("accepted %v", span)
				}
			}()
			v.validateMathLink(span, span["type"])
		}()
	}
}
