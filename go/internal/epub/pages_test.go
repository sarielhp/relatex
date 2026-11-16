package epub

import (
	"strings"
	"testing"
)

const pagedPage = `<html lang="en"><head><title>Paged</title></head><body><main><div id="root" data-structure="document">` +
	`<p id="front">Title</p>` +
	`<section id="s1"><h2 id="h1">One</h2><p id="p1">First.</p><ol><li><p id="item">Item</p></li></ol></section>` +
	`<section id="s2"><h2 id="h2">Two</h2><div id="d1" class="display_math"><math><mi>x</mi></math></div>` +
	`<table><tbody><tr><td><p id="cell">c</p></td></tr></tbody></table>` +
	`<figure id="f"><figcaption>Caption</figcaption><p id="infig">drawn</p></figure></section>` +
	`</div></main></body></html>`

func TestPageMarkersAndPageList(t *testing.T) {
	p := preparePage(pagedPage)
	documents, reason := splitPage(p)
	if reason != "" {
		t.Fatalf("page stays whole: %s", reason)
	}
	marks := []PageMark{{1, "i", "front"}, {2, "1", "h1"}, {3, "2", "item"}, {4, "3", "d1"}, {5, "4", "cell"}, {6, "5", "infig"},
		{7, "6", "missing"}, {8, "", "infig"}, {9, "7", ""}}
	pages := markPages(documents, marks)
	// Every mark is listed (decision 130): the absent block's goes with the next mark, the page
	// after the text at its end.
	if len(pages) != 9 {
		t.Fatalf("listed %d pages, want 9: %v", len(pages), pages)
	}
	if pages[0] != (pageEntry{"i", "index.xhtml#page-1"}) || pages[1] != (pageEntry{"1", "section-1.xhtml#page-2"}) ||
		pages[6] != (pageEntry{"6", "section-2.xhtml#page-7"}) || pages[7] != (pageEntry{"8", "section-2.xhtml#page-8"}) ||
		pages[8] != (pageEntry{"7", "section-2.xhtml#page-9"}) {
		t.Errorf("entries %v", pages)
	}
	var text []string
	for _, document := range documents {
		text = append(text, p.xhtml(document))
	}
	marker := func(page, label string) string {
		return `<span id="page-` + page + `" epub:type="pagebreak" role="doc-pagebreak" aria-label="` + label + `"></span>`
	}
	for _, want := range []string{
		marker("1", "i") + `<p id="front">`,         // a paragraph of the document element
		marker("2", "1") + `<h2 id="h1">`,           // before a heading, never inside it
		`<li>` + marker("3", "2") + `<p id="item">`, // inside a list item, not between list items
		marker("4", "3") + `<div id="d1"`,           // before a display
		`<td>` + marker("5", "4") + `<p id="cell">`,
		marker("6", "5") + marker("7", "6") + marker("8", "8") + `<figure id="f">`, // before the figure, whose caption stays first
		`</figure>` + marker("9", "7") + `</section>`,                              // after the text
	} {
		if !strings.Contains(strings.Join(text, "\n"), want) {
			t.Errorf("missing %q in\n%s", want, strings.Join(text, "\n"))
		}
	}
	for index, document := range text {
		wellFormed(t, documents[index].name, document)
	}
	nav := navigation(p, documents, pages, p.title())
	wellFormed(t, "nav", nav)
	if !strings.Contains(nav, `<nav epub:type="page-list" role="doc-pagelist" id="page-list" hidden="hidden"><h2>Pages</h2><ol>`+
		`<li><a href="index.xhtml#page-1">i</a></li><li><a href="section-1.xhtml#page-2">1</a></li>`) {
		t.Errorf("page list:\n%s", nav)
	}
	if strings.Contains(navigation(p, documents, nil, p.title()), "page-list") {
		t.Error("a page list without marks")
	}
}

func TestMarkerIDsAvoidTheDocumentsIDs(t *testing.T) {
	p := preparePage(`<html><head><title>t</title></head><body><p id="page-1">a</p><p id="b">b</p></body></html>`)
	documents := []contentDocument{{name: FirstDocument, body: p.body.Children}}
	pages := markPages(documents, []PageMark{{1, "1", "b"}})
	if len(pages) != 1 || pages[0].href != "index.xhtml#page-1-2" {
		t.Errorf("entries %v", pages)
	}
}
