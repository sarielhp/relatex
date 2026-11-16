package semantic

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestSemanticPageTotalWritersPreserveTheFooterReference(t *testing.T) {
	const id = "n-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	node := rec.Map{"id": id, "type": "document_page_total", "count": int64(9), "content": []any{
		rec.Map{"type": "reference", "text": "9", "label": "TotPages", "target": id},
	}}
	markdownWriter := &markdown{}
	markdownWriter.export = &export{w: markdownWriter, nodes: rec.Map{id: node}}
	markdown := markdownWriter.pageTotal(node)
	if !strings.Contains(markdown, `[9](#`+id+`)`) {
		t.Fatalf("Markdown page total = %s", markdown)
	}
	if rendered := markdownWriter.export.renderNode(id, 0); !strings.Contains(rendered, `[9](#`+id+`)`) {
		t.Fatalf("Markdown document node = %s", rendered)
	}
	typstWriter := &typst{}
	typstWriter.export = &export{w: typstWriter, nodes: rec.Map{id: node}}
	typst := typstWriter.pageTotal(node)
	if !strings.Contains(typst, `#link(<`+id+`>)[#text("9")]`) {
		t.Fatalf("Typst page total = %s", typst)
	}
}
