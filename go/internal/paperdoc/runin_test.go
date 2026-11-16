package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A run-in heading's paragraph carries its title's span count in its own copy of the semantics;
// another block, or a count beyond the content, carries none.
func TestRuninTitleSpans(t *testing.T) {
	semantics := rec.Map{"kind": "heading", "runin": true, "level": int64(4)}
	fields := withTitle(rec.Map{"semantics": semantics}, rec.Map{"type": "paragraph", "title_spans": int64(2)}, 3)
	if got := rec.AsMap(fields["semantics"])["title_spans"]; got != int64(2) || semantics["title_spans"] != nil {
		t.Errorf("title spans %v, shared semantics %v", got, semantics)
	}
	for _, block := range []rec.Map{{"type": "paragraph", "title_spans": int64(4)}, {"type": "paragraph"}} {
		if _, has := rec.AsMap(withTitle(rec.Map{"semantics": semantics}, block, 3)["semantics"])["title_spans"]; has {
			t.Errorf("no title for %v", block)
		}
	}
	displayed := rec.Map{"semantics": rec.Map{"kind": "heading", "runin": false}}
	if _, has := rec.AsMap(withTitle(displayed, rec.Map{"type": "paragraph", "title_spans": int64(1)}, 2)["semantics"])["title_spans"]; has {
		t.Error("a displayed heading has no run-in title")
	}
}
