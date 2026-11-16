package epub

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A book is packaged as its split (decision 081): the front matter, then one document per chapter
// named as the chapter file is; --split none keeps the division by top-level sections.
func TestSplitBookByChapters(t *testing.T) {
	document := rec.Map{"main_block_order": []any{"h1", "h2"}, "nodes": rec.Map{
		"h1": rec.Map{"type": "heading", "semantics": rec.Map{"counter": "chapter"}},
		"h2": rec.Map{"type": "heading", "semantics": rec.Map{"counter": "chapter"}}}}
	p := preparePage(samplePage)
	documents, reason, chapters := splitBookPage(p, document, "chapters")
	names := []string{}
	for _, d := range documents {
		names = append(names, d.name)
	}
	if !chapters || reason != "" || strings.Join(names, " ") != "index.xhtml one.xhtml y.xhtml" {
		t.Fatalf("documents %v, reason %q, chapters %v", names, reason, chapters)
	}
	text := p.xhtml(documents[1])
	wellFormed(t, documents[1].name, text)
	for _, want := range []string{`<a href="y.xhtml#p2">two</a>`, `data-split-continued="root"`} {
		if !strings.Contains(text, want) {
			t.Errorf("chapter lacks %q:\n%s", want, text)
		}
	}
	last := p.xhtml(documents[2])
	if !strings.Contains(p.xhtml(documents[0]), `<aside id="note"`) || !strings.Contains(last, `<nav id="contents"`) {
		t.Errorf("the notes and the contents are lost:\n%s", last)
	}
	// Each chapter file holds its own pages' markers; the page list links across the files, and the
	// page after the text ends the last chapter (decision 130).
	pages := markPages(documents, []PageMark{{1, "i", "front"}, {2, "1", "p1"}, {3, "2", "p2"}, {4, "3", ""}})
	hrefs := []string{}
	for _, entry := range pages {
		hrefs = append(hrefs, entry.href)
	}
	if strings.Join(hrefs, " ") != "index.xhtml#page-1 one.xhtml#page-2 y.xhtml#page-3 y.xhtml#page-4" {
		t.Errorf("page list %v", hrefs)
	}
	for index, want := range []string{`aria-label="i"></span><p id="front">`, `aria-label="1"></span><p id="p1">`, `aria-label="3"></span>`} {
		if !strings.Contains(p.xhtml(documents[index]), want) {
			t.Errorf("%s lacks %q:\n%s", documents[index].name, want, p.xhtml(documents[index]))
		}
	}
	documents, _, chapters = splitBookPage(preparePage(samplePage), document, "none")
	if chapters || documents[1].name != "section-1.xhtml" {
		t.Errorf("--split none packaged chapters")
	}
}
