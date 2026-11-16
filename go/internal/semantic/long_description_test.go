package semantic

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// TestLongDescriptionsFollowTheFigureBlock: a figure's long description (decision 110) is text of
// its own after the block that holds the figure, in Markdown and in Typst; an image without one
// adds nothing.
func TestLongDescriptionsFollowTheFigureBlock(t *testing.T) {
	node := rec.Map{"id": "n-1", "type": "paragraph", "content": []any{
		rec.Map{"type": "image", "alt": "A plot.", "long_description": "The error falls *fast* [then] levels off."},
		rec.Map{"type": "image", "alt": "Another."},
	}}
	markdownWriter := &markdown{}
	markdownWriter.export = &export{w: markdownWriter, nodes: rec.Map{"n-1": node}}
	want := "\n\n*Description of the figure:* The error falls \\*fast\\* \\[then\\] levels off."
	if got := markdownWriter.export.longDescriptions(node); got != want {
		t.Errorf("Markdown long descriptions = %q, want %q", got, want)
	}
	typstWriter := &typst{}
	typstWriter.export = &export{w: typstWriter, nodes: rec.Map{"n-1": node}}
	got := typstWriter.export.longDescriptions(node)
	if !strings.HasPrefix(got, "\n\n#block[#emph[Description of the figure:] #text(\"The error falls *fast* [then] levels off.\")]") ||
		strings.Count(got, "#block") != 1 {
		t.Errorf("Typst long descriptions = %q", got)
	}
	if alt := typstAlt(rec.AsMap(rec.FetchList(node, "content")[1])); alt != `, alt: "Another."` {
		t.Errorf("Typst alt = %q", alt)
	}
}
