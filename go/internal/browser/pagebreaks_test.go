package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The page's print-page markers (plan accessibility A8) are checked on their own: empty spans in
// the main text, out of the flow, a pixel at most, named "Page " and a label, with ids of their own.
func TestPageBreakMarkersAreChecked(t *testing.T) {
	good := func() rec.Map {
		return rec.Map{"id": "page-7", "label": "Page 7", "empty": true, "main": true, "flow": true, "size": 1.0}
	}
	if failed := failsWith(func() { verifyPageBreaks(rec.Map{"pageBreaks": []any{good()}}, []string{"main"}) }); failed {
		t.Fatalf("a good marker fails")
	}
	for name, change := range map[string]rec.Map{
		"text":      {"empty": false},
		"outside":   {"main": false},
		"in flow":   {"flow": false},
		"visible":   {"size": 12.0},
		"unnamed":   {"label": "Page "},
		"node id":   {"id": "main"},
		"other id":  {"id": "n-1"},
		"no prefix": {"label": "7"},
	} {
		if !failsWith(func() { verifyPageBreaks(rec.Map{"pageBreaks": []any{rec.Merge(good(), change)}}, []string{"main"}) }) {
			t.Errorf("%s: accepted", name)
		}
	}
	if !failsWith(func() { verifyPageBreaks(rec.Map{"pageBreaks": []any{good(), good()}}, nil) }) {
		t.Errorf("a duplicated marker is accepted")
	}
}

func failsWith(run func()) (failed bool) {
	defer func() {
		if recover() != nil {
			failed = true
		}
	}()
	run()
	return false
}
