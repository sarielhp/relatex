package html

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A heading of rules only is decoration hidden from assistive technology (titlesec's rule after
// a title, papers5 1187; a box before an appendix title, papers4 0919), a heading of formulas only
// is named by its formula (slides 0015), and a heading with text or an image keeps its own name:
// Ace reported the first two as empty-heading.
func TestHeadingsWithoutTextHaveANameOrAreHidden(t *testing.T) {
	rule := rec.Map{"type": "rule", "width_sp": int64(15953254), "height_sp": int64(32768), "depth_sp": int64(0)}
	paper := &Paper{document: rec.Map{}, nodes: rec.Map{}}
	ruleHTML := paper.renderSpan(rule)
	formula := `<span data-kind="math"><math alttext="n=4"><mi>n</mi><mo>=</mo><mn>4</mn></math></span>`
	for _, test := range []struct {
		content []any
		html    string
		want    string
	}{
		{[]any{rule}, ruleHTML, ` aria-hidden="true"`},
		{[]any{rec.Map{"type": "text", "text": " "}, rule}, " " + ruleHTML, ` aria-hidden="true"`},
		{[]any{rec.Map{"type": "math", "node": "m"}}, formula, ` aria-label="n=4"`},
		{[]any{rec.Map{"type": "text", "text": "Introduction"}, rule}, "Introduction" + ruleHTML, ""},
		{[]any{rec.Map{"type": "image", "id": "i"}}, `<img alt="" src="figs/a.png">`, ""},
	} {
		if got := headingName(rec.Map{"type": "heading", "content": test.content}, test.html); got != test.want {
			t.Errorf("headingName(%s) = %q, want %q", test.html, got, test.want)
		}
	}
}
