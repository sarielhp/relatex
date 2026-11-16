package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// aipproc sets a figure's picture and its caption in one paragraph: the caption line opens after the
// picture, and the label names that paragraph (papers3/0852).
func TestFigureLabelNamesACaptionOpeningAfterItsPicture(t *testing.T) {
	figure := rec.Map{"id": "figure", "type": "paragraph", "entity": int64(6), "content": []any{
		rec.Map{"type": "image", "index": int64(1)}, rec.Map{"type": "text", "text": " FIGURE 1. Luminosity function"}}}
	e := &Evidence{blocks: []rec.Map{figure}}
	e.owners = e.inventory("flow_candidate", "owner")
	label := rec.Map{"name": "FigLF", "counter": "figure", "number": "1", "anchor": "figure.1",
		"entity": int64(6), "target_entity": int64(0), "seq": int64(10)}

	if got := blockIDs(e.targetBlocks(label)); len(got) != 1 || got[0] != "figure" {
		t.Fatalf("figure label targets %v, want the picture's caption paragraph", got)
	}
}
