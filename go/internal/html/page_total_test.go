package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestRenderDocumentPageTotalLinksItsFooterNumber(t *testing.T) {
	const id = "n-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	paper := &Paper{document: rec.Map{}, nodes: rec.Map{id: rec.Map{
		"id": id, "type": "document_page_total", "count": int64(9), "pdf_sha256": strings.Repeat("b", 64),
		"anchor": "page.9", "content": []any{rec.Map{"type": "reference", "text": "9", "label": "TotPages", "target": id}},
	}}}
	html := paper.RenderNode(id)
	if !strings.Contains(html, `data-document-page-total="9"`) || !strings.Contains(html, `data-label="TotPages" href="#`+id+`">9</a>`) {
		t.Fatalf("page-total HTML = %s", html)
	}
}
