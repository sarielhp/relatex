package accessibility

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// runLandmarkDocument is a document whose bibliography entries follow a paragraph in their
// section: no section landmark, one run of entries (the revtex shape).
func runLandmarkDocument(language string) rec.Map {
	text := []any{rec.Map{"type": "text", "text": "x"}}
	nodes := rec.Map{
		"root": rec.Map{"id": "root", "type": "document", "children": []any{"s1"}},
		"s1":   rec.Map{"id": "s1", "type": "section", "children": []any{"h1", "p1", "e1", "e2"}},
		"h1":   rec.Map{"id": "h1", "type": "heading", "content": text},
		"p1":   rec.Map{"id": "p1", "type": "paragraph", "content": text},
		"e1":   rec.Map{"id": "e1", "type": "paragraph", "content": text},
		"e2":   rec.Map{"id": "e2", "type": "paragraph", "content": text},
	}
	return rec.Map{
		"root": "root", "nodes": nodes, "language": rec.Map{"tag": language},
		"main_block_order": []any{"h1", "p1", "e1", "e2"},
		"citations":        rec.Map{"entries": []any{rec.Map{"blocks": []any{"e1"}}, rec.Map{"blocks": []any{"e2"}}}},
	}
}

func hasLandmark(items []any, name string) bool {
	for _, value := range items {
		if rec.AsMap(value)["landmark"] == name {
			return true
		}
	}
	return false
}

// A page with the run's entries but no wrapper misses the doc-bibliography landmark; the
// wrapper earns it, and a page in another language asks for none.
func TestBibliographyRunLandmarkItem(t *testing.T) {
	document := runLandmarkDocument("en")
	bare := []byte(`<html lang="en"><body><main id="main"><section id="s1"><p id="e1"></p></section></main></body></html>`)
	if !hasLandmark(landmarkItems(document, bare), "doc-bibliography") {
		t.Error("a run without its wrapper passed")
	}
	wrapped := []byte(`<html lang="en"><body><main id="main"><section id="s1">` +
		`<div id="bibliography" role="doc-bibliography" aria-label="References"><p id="e1"></p></div></section></main></body></html>`)
	if hasLandmark(landmarkItems(document, wrapped), "doc-bibliography") {
		t.Error("the wrapped run is reported")
	}
	if hasLandmark(landmarkItems(runLandmarkDocument("de"), bare), "doc-bibliography") {
		t.Error("a German page is asked for the English wrapper")
	}
}
