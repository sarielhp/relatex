package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// An equation that is only an xy-pic picture prints its number in a text block of its own beside
// the rendered picture: the label names the block printing "(number)", and the picture only when
// no block prints it.
func TestPictureOnlyEquationLabelNamesItsTagBlock(t *testing.T) {
	tag := textBlock("p1", "paragraph", int64(9), "(3.1)")
	picture := rec.Map{"id": "p2", "type": "paragraph", "entity": int64(9), "pages": []any{int64(1)},
		"content": []any{rec.Map{"type": "rendered_graphic"}}}
	e := &Evidence{records: []rec.Map{{"code": "equation_context", "source": "latex-equation-hooks",
		"entity": int64(9), "seq": int64(1)}}}
	label := rec.Map{"name": "eq", "counter": "equation", "number": "3.1", "target_entity": int64(9), "seq": int64(5)}

	if got := blockIDs(e.equationBlocks(label, []rec.Map{tag, picture})); len(got) != 1 || got[0] != "p1" {
		t.Fatalf("tagged picture equation targets %v, want the tag block p1", got)
	}
	if got := blockIDs(e.equationBlocks(label, []rec.Map{picture})); len(got) != 1 || got[0] != "p2" {
		t.Fatalf("untagged picture equation targets %v, want the picture p2", got)
	}
}
