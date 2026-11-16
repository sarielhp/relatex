package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// runPaper is a document whose references' section holds a paragraph, then the entries: no
// section landmark, but one run of entries.
func runPaper(language string) *Paper {
	text := func(value string) []any { return []any{rec.Map{"type": "text", "text": value}} }
	nodes := rec.Map{
		"root": rec.Map{"id": "root", "type": "document", "children": []any{"s1", "s2"}},
		"s1":   rec.Map{"id": "s1", "type": "section", "children": []any{"h1", "p1"}},
		"h1":   rec.Map{"id": "h1", "type": "heading", "content": text("1 Intro")},
		"p1":   rec.Map{"id": "p1", "type": "paragraph", "content": text("Intro.")},
		"s2":   rec.Map{"id": "s2", "type": "section", "children": []any{"h2", "ad", "e1", "e2"}},
		"h2":   rec.Map{"id": "h2", "type": "heading", "content": text("References")},
		"ad":   rec.Map{"id": "ad", "type": "paragraph", "content": text("References in print.")},
		"e1":   rec.Map{"id": "e1", "type": "paragraph", "content": text("[1] A. Author")},
		"e2":   rec.Map{"id": "e2", "type": "paragraph", "content": text("[2] B. Author")},
	}
	document := rec.Map{
		"root": "root", "nodes": nodes, "language": rec.Map{"tag": language},
		"main_block_order": []any{"h1", "p1", "h2", "ad", "e1", "e2"},
		"citations":        rec.Map{"entries": []any{rec.Map{"blocks": []any{"e1"}}, rec.Map{"blocks": []any{"e2"}}}},
	}
	return &Paper{document: document, nodes: nodes}
}

// A run of entries after prose is a doc-bibliography wrapper, named References.
func TestBibliographyRunIsAWrappedLandmark(t *testing.T) {
	page := runPaper("en").RenderNode("root")
	if strings.Contains(page, `aria-labelledby=`) {
		t.Errorf("the section became a landmark:\n%s", page)
	}
	if !strings.Contains(page, `<div id="bibliography" role="doc-bibliography" aria-label="References"><p id="e1"`) ||
		!strings.Contains(page, `</p></div></section>`) {
		t.Errorf("the run is not wrapped:\n%s", page)
	}
	if strings.Count(page, `role="doc-bibliography"`) != 1 {
		t.Errorf("landmarks:\n%s", page)
	}
}

// A page in another language gets no English wrapper; the entries stay as they were.
func TestBibliographyRunIsEnglishOnly(t *testing.T) {
	page := runPaper("de").RenderNode("root")
	if strings.Contains(page, `doc-bibliography`) {
		t.Errorf("a German page got the English wrapper:\n%s", page)
	}
	if !strings.Contains(page, `<section id="s2" data-structure="section" class="section"><h2 id="h2"`) ||
		!strings.Contains(page, `<p id="e1" data-block="paragraph" class="paragraph">`) {
		t.Errorf("the entries changed:\n%s", page)
	}
}

// The wrapper's id avoids an id the document already uses.
func TestBibliographyRunIdDoesNotCollide(t *testing.T) {
	paper := runPaper("en")
	paper.nodes["bibliography"] = rec.Map{"id": "bibliography", "type": "paragraph", "content": []any{}}
	if page := paper.RenderNode("root"); !strings.Contains(page, `<div id="bibliography-2" role="doc-bibliography" aria-label="References">`) {
		t.Errorf("the wrapper's id collides:\n%s", page)
	}
}
