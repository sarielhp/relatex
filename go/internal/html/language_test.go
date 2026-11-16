package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// A span set in another language carries its tag; a span in the document's language none.
func TestRenderSpanWritesTheSpansLanguage(t *testing.T) {
	paper := &Paper{document: rec.Map{}, nodes: rec.Map{}}
	foreign := paper.renderSpan(rec.Map{"type": "text", "text": "Guten Tag", "lang": "de"})
	if foreign != `<span lang="de" data-kind="text">Guten Tag</span>` {
		t.Errorf("foreign span = %s", foreign)
	}
	if plain := paper.renderSpan(rec.Map{"type": "text", "text": "hello"}); strings.Contains(plain, "lang=") {
		t.Errorf("a span in the document's language has a lang: %s", plain)
	}
	link := paper.renderSpan(rec.Map{"type": "reference", "text": "1", "label": "l", "target": "t", "lang": "fr", "color": "#0000ff"})
	if !strings.HasPrefix(link, `<a lang="fr" style="color:#0000ff" data-kind="reference"`) {
		t.Errorf("foreign colored link = %s", link)
	}
}

// The page's lang is the document's tag, English for a document without the field.
func TestDocumentLanguage(t *testing.T) {
	if got := htmlkit.DocumentLanguage(rec.Map{"language": rec.Map{"tag": "de-AT", "source": "babel", "name": "naustrian"}}); got != "de-AT" {
		t.Errorf("declared language = %q", got)
	}
	if got := htmlkit.DocumentLanguage(rec.Map{}); got != "en" {
		t.Errorf("default language = %q", got)
	}
}
