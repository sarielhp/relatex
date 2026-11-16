package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A list set in a table float's body (float_member) and its items are read in the float's partition:
// papers2/0309's cell label paragraph then resumes in a new block after the list, in order.
func TestMemberListReadsInItsFloatPartition(t *testing.T) {
	s := newScopes([]rec.Map{
		{"kind": "context", "entity": int64(1), "parent": int64(0), "role": "paragraph", "seq": int64(6)},
		{"kind": "diagnostic", "seq": int64(7), "code": "float_context", "source": "latex-float-environment-hooks",
			"entity": int64(1), "parent": int64(0), "category": "table"},
		{"kind": "context", "entity": int64(2), "parent": int64(0), "role": "unordered_list", "seq": int64(20)},
		{"kind": "diagnostic", "seq": int64(21), "code": "float_member", "source": "latex-float-environment-hooks",
			"entity": int64(2), "parent": int64(0), "role": "unordered_list", "float": int64(1)},
		{"kind": "context", "entity": int64(3), "parent": int64(2), "role": "list_item", "seq": int64(24)},
		{"kind": "context", "entity": int64(9), "parent": int64(0), "role": "unordered_list", "seq": int64(90)},
		{"kind": "context", "entity": int64(10), "parent": int64(9), "role": "list_item", "seq": int64(91)},
	})
	item, ok := s.partition(rec.Map{"entity": int64(3), "seq": int64(30)})
	if !ok || item != (partition{"float", int64(1)}) {
		t.Fatalf("list item in a float body: got %v %v, want the float partition", item, ok)
	}
	// A list outside any float stays in the main flow.
	other, ok := s.partition(rec.Map{"entity": int64(10), "seq": int64(95)})
	if ok || other != (partition{}) {
		t.Fatalf("list item outside floats: got %v %v, want the main flow", other, ok)
	}
}
