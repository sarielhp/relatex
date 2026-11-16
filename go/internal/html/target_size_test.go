package html

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A rich cell holding a link is at least WCAG 2.2's minimum target tall (2.5.8): a column of
// references in 23 px TeX rows failed axe's target-size (books 0003, 0004; papers2 0309). A cell
// without a link keeps its native height.
func TestRichCellWithALinkIsATarget(t *testing.T) {
	paper := &Paper{document: rec.Map{}, nodes: rec.Map{}}
	link := paper.renderSpan(rec.Map{"type": "reference", "text": "Lemma 1", "label": "l", "target": "t"})
	if got := targetHeight(link); got != ";min-height:24px" {
		t.Errorf("targetHeight(%s) = %q", link, got)
	}
	text := paper.renderSpan(rec.Map{"type": "text", "text": "Lemma 1"})
	if got := targetHeight(text); got != "" {
		t.Errorf("targetHeight(%s) = %q, want none", text, got)
	}
}
