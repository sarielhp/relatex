package typstsource

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A listing is a raw block of its lines' printed text (decision 120).
func TestCodeIsARawBlock(t *testing.T) {
	line := func(id, text string) rec.Map {
		return rec.Map{"id": id, "type": "paragraph", "children": []any{}, "content": []any{rec.Map{"type": "text", "text": text}}}
	}
	e := &exporter{nodes: rec.Map{
		"c":  rec.Map{"id": "c", "type": "code", "children": []any{"l1", "l2"}},
		"l1": line("l1", "struct customer {"),
		"l2": line("l2", "};"),
	}, labelsByTarget: map[string][]string{}, placed: map[string]string{}, consumed: map[string]bool{}}
	if got, want := e.renderNodeOnly("c"), `#raw(block: true, "struct customer {\n};")`; got != want {
		t.Errorf("code = %q, want %q", got, want)
	}
}
