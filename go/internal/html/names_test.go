package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// An external link's name is the text its formula shows, else its target as written; an e-mail
// address drops its scheme.
func TestLinkNameIsTheShownTextOrTheTarget(t *testing.T) {
	empty := `<span data-kind="math"><math id='m'></math></span>`
	for href, want := range map[string]string{"mailto:a@b.org": "a@b.org", "https://doi.org/10.1/x": "https://doi.org/10.1/x"} {
		if got := linkName(empty, href); got != want {
			t.Errorf("%s: %s, want %s", href, got, want)
		}
	}
	shown := `<span data-kind="math"><math id='m'><mi mathvariant='normal'>a</mi><mo>&lt;</mo>` + "\n" + `<mi>b</mi></math></span>`
	if got := linkName(shown, "https://x.org"); got != "a< b" {
		t.Errorf("shown formula named %q", got)
	}
}

// A link made of a formula or of spaces has no name of its own; text has one.
func TestNamelessSpans(t *testing.T) {
	for _, row := range []struct {
		span rec.Map
		want bool
	}{{rec.Map{"type": "math"}, true}, {rec.Map{"type": "text", "text": "  "}, true},
		{rec.Map{"type": "text", "text": "Smith"}, false}, {rec.Map{"type": "reference", "text": ""}, false}} {
		if got := nameless(row.span); got != row.want {
			t.Errorf("%v: %v", row.span, got)
		}
	}
}

func citationPaper(entryText string) *Paper {
	document := rec.Map{
		"citations": rec.Map{
			"calls":   []any{rec.Map{"id": int64(1), "keys": []any{"k"}, "targets": []any{int64(2)}}},
			"links":   []any{rec.Map{"id": int64(3), "entry": int64(2)}},
			"entries": []any{rec.Map{"id": int64(2), "key": "k", "blocks": []any{"b"}}},
		},
	}
	nodes := rec.Map{"b": rec.Map{"content": []any{rec.Map{"type": "text", "text": entryText}, rec.Map{"type": "math", "node": "m"}}}}
	return &Paper{document: document, nodes: nodes}
}

// A citation link with no text of its own is named by its entry, cut at a word within 80
// characters; a link with text keeps its own name.
func TestNamelessCitationLinksNameTheirEntry(t *testing.T) {
	entry := "Smith,  J. On the proof of everything that   matters, with a title far longer than eighty characters."
	paper := citationPaper(entry)
	got := paper.citationSpan(`<span data-kind="text"> </span>`, rec.Map{"type": "text", "text": " ", "citation": int64(1), "citation_link": int64(3)})
	want := `aria-label="Reference: Smith, J. On the proof of everything that matters, with a title far longer than…"`
	if !strings.Contains(got, want) || !strings.HasPrefix(got, "<a ") {
		t.Errorf("nameless citation link %s", got)
	}
	named := paper.citationSpan(`<span data-kind="text">Smith</span>`, rec.Map{"type": "text", "text": "Smith", "citation": int64(1), "citation_link": int64(3)})
	if strings.Contains(named, "aria-label") {
		t.Errorf("named citation link %s", named)
	}
	formula := paper.citationSpan(`<span data-kind="math"><math id='m'><msup><mi></mi><mn>12</mn></msup></math></span>`,
		rec.Map{"type": "math", "node": "m", "citation": int64(1), "citation_link": int64(3)})
	if !strings.Contains(formula, `aria-label="12"`) {
		t.Errorf("citation formula link %s", formula)
	}
	if got := citationPaper("").entryName(rec.Map{"key": "k", "blocks": []any{"b"}}); got != "Reference k" {
		t.Errorf("entry without text: %s", got)
	}
}

func textLinkPaper(span rec.Map, links ...rec.Map) *Paper {
	document := rec.Map{
		"nodes":       rec.Map{"b": rec.Map{"content": []any{span}}},
		"block_order": []any{"b"},
	}
	return &Paper{document: document, descriptors: links, described: true}
}

// A text link made of nothing but spaces has no accessible name (WCAG 2.4.4/4.1.2): the space
// past a formula is written as plain text; the neighbouring anchor with the same href is the
// named one. Text inside blank edges, and ordinary text, keep their anchor.
func TestBlankTextLinksAreNotAnchors(t *testing.T) {
	for _, row := range []struct {
		name     string
		text     string
		start    int64
		linkText string
		anchor   bool
	}{{"trailing space", "arXiv: 1101.0003 ", 16, " ", false},
		{"text", "here", 0, "here", true},
		{"blank edges", " x ", 0, " x ", true},
		{"no-break space", "\u00a0", 0, "\u00a0", false}} {
		span := rec.Map{"type": "text", "text": row.text}
		link := rec.Map{"id": "n-x", "href": "https://arxiv.org/abs/1101.0003", "text": row.linkText,
			"start": row.start, "block": "b", "span": int64(0)}
		got := textLinkPaper(span, link).textLinkSpan(span)
		if strings.Contains(got, "<a") != row.anchor {
			t.Errorf("%s: anchor %v: %s", row.name, row.anchor, got)
		}
		if !strings.Contains(got, htmlkit.Escape(row.text)) {
			t.Errorf("%s: text lost: %s", row.name, got)
		}
	}
}

// A formula's invisible operators stay in a link's name, so the name contains the page's text
// exactly (WCAG 2.5.3: axe compared "https" and "et al" as separate letters otherwise).
func TestLinkNameKeepsInvisibleOperators(t *testing.T) {
	formula := "<span data-kind=\"math\"><math id='m'><mi>e</mi><mo>\u2062</mo><mi>t</mi><mspace width='2pt'/><mi>a</mi></math></span>"
	want := "e\u2062ta"
	if got := linkName(formula, "https://x.org"); got != want {
		t.Errorf("link name %q, want %q", got, want)
	}
	if got := citationPaper("entry").citationName(formula, rec.Map{}); got != want {
		t.Errorf("citation name %q, want %q", got, want)
	}
	if got := visibleText(formula); got != "eta" {
		t.Errorf("visible text %q (headings and contents drop invisible operators)", got)
	}
}

// A citation link whose text is only its entry's label is named "Reference <label>: " and the
// entry without that label (decision 168); author-year text, a glyph and another language get none.
func TestBareLabelCitationLinksNameTheirEntry(t *testing.T) {
	paper := citationPaper("[12] Smith, J. On the proof of everything that matters, with a title far longer than eighty characters.")
	paper.document["citations"].(rec.Map)["links"] = []any{rec.Map{"id": int64(3), "entry": int64(2), "number": "12"}}
	cite := func(text string) rec.Map {
		return rec.Map{"type": "text", "text": text, "citation": int64(1), "citation_link": int64(3)}
	}
	got := paper.citationSpan(`<span data-kind="text">12</span>`, cite("12"))
	want := `aria-label="Reference 12: Smith, J. On the proof of everything that matters, with a title far longer than…"`
	if !strings.Contains(got, want) {
		t.Errorf("bare label %s", got)
	}
	glyph := cite("12")
	glyph["glyph_name"] = "Tumbler"
	for _, span := range []rec.Map{cite("Smith 2020"), cite("1"), glyph} {
		if got := paper.citationSpan(`<span data-kind="text">x</span>`, span); strings.Contains(got, "aria-label") {
			t.Errorf("%v named: %s", span, got)
		}
	}
	paper.document["language"] = rec.Map{"tag": "de"}
	if got := paper.citationSpan(`<span data-kind="text">12</span>`, cite("12")); strings.Contains(got, "aria-label") {
		t.Errorf("German page named %s", got)
	}
}

// The name says the label once: an entry opening with it drops it, an entry that is only the label
// leaves the name the label.
func TestWithoutLabel(t *testing.T) {
	for text, want := range map[string]string{
		"[12] Smith": "Smith", "(12) Smith": "Smith", "12. Smith": "Smith", "12 Smith": "Smith",
		"123 Smith": "123 Smith", "[12]": "", "Smith 12": "Smith 12",
	} {
		if got := withoutLabel(text, "12"); got != want {
			t.Errorf("%q: %q, want %q", text, got, want)
		}
	}
}
