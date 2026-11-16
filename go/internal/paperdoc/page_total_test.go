package paperdoc

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func pageTotalBuilder(existingReference bool) (*builder, string) {
	identity := rec.Map{"input_sha256": strings.Repeat("a", 64)}
	rootID := (&builder{identity: identity}).uid("main", int64(0))
	b := &builder{identity: identity, nodes: rec.Map{rootID: rec.Map{"children": []any{}}}, order: []string{},
		reading: rec.Map{"targets": []any{rec.Map{"kind": "document_page_total", "label": "TotPages", "number": "9",
			"count": int64(9), "anchor": "page.9", "pdf_sha256": strings.Repeat("b", 64)}}, "blocks": []any{}}}
	if existingReference {
		b.nodes["existing"] = rec.Map{"content": []any{rec.Map{"type": "reference", "text": "9", "label": "TotPages", "target": "existing"}}}
		b.order = append(b.order, "existing")
	}
	b.buildDocumentPageTotals()
	return b, rootID
}

func TestDocumentPageTotalRecordsFooterReferenceWhenNoUseWasRetained(t *testing.T) {
	b, rootID := pageTotalBuilder(false)
	id := b.uid("document_page_total", "TotPages")
	node := rec.AsMap(b.nodes[id])
	content := rec.FetchList(node, "content")
	if len(content) != 1 || !validPageTotalReference(rec.AsMap(content[0]), node, 9) {
		t.Fatalf("page-total content = %v", content)
	}
	if !rec.Equal(rec.FetchList(rec.AsMap(b.nodes[rootID]), "children"), []any{id}) {
		t.Fatalf("root children = %v", rec.FetchList(rec.AsMap(b.nodes[rootID]), "children"))
	}
	v := &validation{nodes: b.nodes, data: rec.Map{"root": rootID,
		"labels":   rec.Map{"TotPages": rec.Map{"target": id, "number": "9"}},
		"metadata": rec.Map{"status": "partial", "source_page_total": id}}}
	v.validatePageTotal(node)
}

func TestDocumentPageTotalDoesNotDuplicateAReferenceAlreadyInTheReading(t *testing.T) {
	b, rootID := pageTotalBuilder(true)
	id := b.uid("document_page_total", "TotPages")
	node := rec.AsMap(b.nodes[id])
	if _, exists := node["content"]; exists {
		t.Fatalf("duplicate page-total reference content = %v", node["content"])
	}
	v := &validation{nodes: b.nodes, data: rec.Map{"root": rootID,
		"labels":   rec.Map{"TotPages": rec.Map{"target": id, "number": "9"}},
		"metadata": rec.Map{"status": "partial", "source_page_total": id}}}
	v.validatePageTotal(node)
}
