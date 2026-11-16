package paperdoc

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A paragraph may record a centre or right alignment; no other node, and no other value, may.
func TestAlignIsOnlyAParagraphFieldOfTwoValues(t *testing.T) {
	id := "n-" + strings.Repeat("a", 64)
	node := func(kind, align string) rec.Map {
		n := rec.Map{"id": id, "type": kind, "children": []any{}, "content": []any{},
			"provenance": rec.Map{"inspection_key": "b1", "source_hint": "unavailable"}}
		if align != "" {
			n["align"] = align
		}
		return n
	}
	failure := func(n rec.Map) (failed bool) {
		defer func() { failed = recover() != nil }()
		v := &validation{nodes: rec.Map{id: n}, seenSet: map[string]bool{}}
		v.validateDocumentNode(n)
		return false
	}
	for align, want := range map[string]bool{"": false, "center": false, "right": false} {
		if failure(node("paragraph", align)) != want {
			t.Errorf("paragraph %q fails %v, want %v", align, !want, want)
		}
	}
	if !failure(node("heading", "center")) {
		t.Error("a heading took an alignment")
	}
}
