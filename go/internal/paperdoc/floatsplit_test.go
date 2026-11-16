package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A figure in a beamer frame with allowframebreaks can be split between slides: its image ends
// one page and its caption opens the next, so no structure holds all its members.
func TestMemberParentOfASplitFloatIsNone(t *testing.T) {
	b := &builder{nodes: rec.Map{
		"page1": rec.Map{"id": "page1", "children": []any{"a", "image"}},
		"page2": rec.Map{"id": "page2", "children": []any{"caption", "b"}},
	}, order: []string{"page1", "page2"}}
	f := &floats{b: b}
	if parent, _ := f.memberParent([]any{"image", "caption"}); parent != nil {
		t.Errorf("a split float has parent %v", parent)
	}
	parent, first := f.memberParent([]any{"caption", "b"})
	if parent == nil || parent["id"] != "page2" || first != 0 {
		t.Errorf("members of one page: parent %v at %d", parent, first)
	}
}
