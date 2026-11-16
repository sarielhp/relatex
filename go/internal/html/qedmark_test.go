package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/rec"
)

// An image QED mark is named by its image's alt, "End of proof"; a text mark by its aria-label.
func TestQEDMarkNames(t *testing.T) {
	pair := figure("figs/duck.pdf", "10")
	pair.span["id"], pair.span["alt"], pair.span["alt_provenance"] = "duck", "End of proof", "inferred"
	document := figurePaper(pair)
	displayed, _ := figures.Plan(document)
	paper := &Paper{document: document, nodes: rec.Map{}, figures: displayed}
	text := paper.symbolSpan(rec.Map{"type": "symbol", "name": "qed", "text": "∎"})
	if !strings.Contains(text, `data-mark="text" role="img" aria-label="End of proof"`) {
		t.Errorf("text mark = %s", text)
	}
	got := paper.symbolSpan(rec.Map{"type": "symbol", "name": "qed", "image": pair.span})
	if !strings.Contains(got, `data-mark="image"><img id="duck"`) || !strings.Contains(got, `alt="End of proof"`) || strings.Contains(got, "aria-label") ||
		strings.Contains(got, "presentation") || strings.Contains(got, "data-undescribed") {
		t.Errorf("image mark = %s", got)
	}
}
