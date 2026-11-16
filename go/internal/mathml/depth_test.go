package mathml

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// nestedFormula is a formula whose innermost element is depth levels below the root.
func nestedFormula(depth int) string {
	return `<math xmlns="` + Namespace + `">` + strings.Repeat("<mrow>", depth-1) + "<mi>x</mi>" +
		strings.Repeat("</mrow>", depth-1) + "</math>"
}

func parseError(source string) (err error) {
	defer rec.Recover(&err)
	Parse(source)
	return nil
}

func TestDepthLimit(t *testing.T) {
	for _, depth := range []int{100, MaxDepth} {
		if err := parseError(nestedFormula(depth)); err != nil {
			t.Errorf("depth %d: %v", depth, err)
		}
	}
	err := parseError(nestedFormula(MaxDepth + 1))
	if err == nil || err.Error() != "MathML depth limit" {
		t.Errorf("depth %d: got %v, want MathML depth limit", MaxDepth+1, err)
	}
}
