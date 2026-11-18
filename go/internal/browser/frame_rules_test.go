package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A rule frame's rules (svjour3's head box, html/ruleframe.go) stay in the page, hidden, so the
// paper's blocks still hold them: a page that drops them is a changed paper (regression: paper
// 0015 failed the export when the frame removed the rule spans).
func TestBlocksKeepRulesOfAFrame(t *testing.T) {
	rule := rec.Map{"type": "rule", "width_sp": int64(34105205), "height_sp": int64(26214), "depth_sp": int64(0)}
	blocks := []rec.Map{{"id": "n-0", "type": "paragraph", "content": []any{rule}}}
	page := func(content ...any) rec.Map {
		return rec.Map{"blocks": []any{rec.Map{"id": "n-0", "type": "paragraph", "content": content}},
			"structures": []any{}, "math": []any{}}
	}
	verifyBlocksAndMath(page(rule), nil, blocks, nil, rec.Map{}, nil, 0)
	if message := failure(func() { verifyBlocksAndMath(page(), nil, blocks, nil, rec.Map{}, nil, 0) }); message != "Browser changed paper blocks" {
		t.Errorf("a page without the frame's rule: %q", message)
	}
}
