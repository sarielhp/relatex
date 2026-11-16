package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// landmarkPaper is a document with a theorem, a proof and a references section whose entries
// sit in a list.
func landmarkPaper() *Paper {
	text := func(value string) []any { return []any{rec.Map{"type": "text", "text": value}} }
	nodes := rec.Map{
		"root":  rec.Map{"id": "root", "type": "document", "children": []any{"s1", "s2"}},
		"s1":    rec.Map{"id": "s1", "type": "section", "children": []any{"h1", "th", "pf"}},
		"h1":    rec.Map{"id": "h1", "type": "heading", "content": text("1 Intro"), "semantics": rec.Map{"counter": "section", "entity": int64(1)}},
		"th":    rec.Map{"id": "th", "type": "theorem", "children": []any{"thp"}, "semantics": rec.Map{"title": "Theorem"}},
		"thp":   rec.Map{"id": "thp", "type": "paragraph", "content": text("Theorem  3.2. Let x be")},
		"pf":    rec.Map{"id": "pf", "type": "proof", "children": []any{"pfp"}},
		"pfp":   rec.Map{"id": "pfp", "type": "paragraph", "content": text(" Proof. Clear.")},
		"s2":    rec.Map{"id": "s2", "type": "section", "children": []any{"h2", "list"}},
		"h2":    rec.Map{"id": "h2", "type": "heading", "content": text("References"), "semantics": rec.Map{"counter": "section", "entity": int64(2)}},
		"list":  rec.Map{"id": "list", "type": "unordered_list", "children": []any{"item1", "e2"}},
		"item1": rec.Map{"id": "item1", "type": "list_item", "children": []any{"e1"}},
		"e1":    rec.Map{"id": "e1", "type": "paragraph", "content": text("[1] A. Author")},
		"e2":    rec.Map{"id": "e2", "type": "paragraph", "content": text("[2] B. Author")},
	}
	document := rec.Map{"root": "root", "nodes": nodes, "main_block_order": []any{"h1", "thp", "pfp", "h2", "e1", "e2"},
		"citations": rec.Map{"entries": []any{rec.Map{"blocks": []any{"e1"}}, rec.Map{"blocks": []any{"e2"}}}}}
	return &Paper{document: document, nodes: nodes}
}

// The status notice and the contents link are the page's banner.
func TestHeaderHoldsTheNoticeAndTheContentsLink(t *testing.T) {
	header := landmarkPaper().pageHeader()
	if header != `<header>`+statusNotice+`<p class="page-links"><a href="#contents">Contents</a></p></header>` {
		t.Errorf("header %s", header)
	}
}

// Theorems and proofs are groups named by their title and number.
func TestTheoremsAndProofsAreNamedGroups(t *testing.T) {
	page := landmarkPaper().RenderNode("root")
	for _, want := range []string{
		`<div id="th" data-structure="theorem" class="theorem" role="group" aria-label="Theorem 3.2">`,
		`<div id="pf" data-structure="proof" class="proof" role="group" aria-label="Proof">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("lacks %s:\n%s", want, page)
		}
	}
	for text, want := range map[string]string{"Lemma A.1 (Key).": "Lemma A.1", "Lemma Let x": "Lemma", "Lem 2": "Lemma", "Lemma": "Lemma",
		"Lemma 4:": "Lemma 4", "Lemma (Zorn).": "Lemma"} {
		if got := GroupName("Lemma", text); got != want {
			t.Errorf("%q: %q, want %q", text, got, want)
		}
	}
	untitled := rec.Map{"type": "theorem", "children": []any{}}
	if attributes := landmarkPaper().groupAttributes(untitled, "theorem"); attributes != "" {
		t.Errorf("untitled theorem %q", attributes)
	}
}

// A theorem set by hand has no title; its first paragraph "Theorem 3.1 (…)" names it (decision 167
// amendment 4), and a paragraph that opens otherwise does not.
func TestTypedTheoremIsNamedByItsFirstParagraph(t *testing.T) {
	for text, want := range map[string]string{" Theorem 3.1 (Porschen, Speckenmeyer": "Theorem 3.1", "Theorem 1. (soundness) x": "Theorem 1",
		"Lemma 4: x": "Lemma 4", "Theorem A is": "", "The 3 cats": "", "Let 3 be": "", "theorem 3": "", "Theorem": ""} {
		if got := typedTitle(text); got != want {
			t.Errorf("%q: %q, want %q", text, got, want)
		}
	}
}

// The references' section is the doc-bibliography landmark, named by its heading; a section
// holding other blocks, or entries in two sections, is not.
func TestBibliographySectionIsALandmark(t *testing.T) {
	paper := landmarkPaper()
	if page := paper.RenderNode("root"); !strings.Contains(page,
		`<section id="s2" data-structure="section" class="section" role="doc-bibliography" aria-labelledby="h2">`) {
		t.Errorf("bibliography:\n%s", page)
	}
	if !strings.Contains(paper.RenderNode("s1"), `<section id="s1" data-structure="section" class="section">`) {
		t.Error("an ordinary section became a landmark")
	}
	// Rows: the section's children, then the list's; "fl" is a float, "ad" an address paragraph.
	for _, row := range []struct {
		children, list []any
		want           string
	}{
		{[]any{"h2", "list", "ad"}, []any{"item1", "e2"}, "s2"},       // end matter after the entries
		{[]any{"h2", "fl", "list", "ad"}, []any{"item1", "e2"}, "s2"}, // a float among them
		{[]any{"h2", "ad", "list"}, []any{"item1", "e2"}, ""},         // prose before the entries
		{[]any{"h2", "list", "ad", "e2"}, []any{"item1"}, ""},         // an entry after the end matter
		{[]any{"h2", "list", "sx"}, []any{"item1", "e2"}, ""},         // a section after the entries
		{[]any{"ad", "list"}, []any{"item1", "e2"}, ""},               // no heading
	} {
		paper := landmarkPaper()
		paper.nodes["fl"] = rec.Map{"id": "fl", "type": "figure_float", "children": []any{}}
		paper.nodes["ad"] = rec.Map{"id": "ad", "type": "paragraph", "content": []any{}}
		paper.nodes["sx"] = rec.Map{"id": "sx", "type": "section", "children": []any{}}
		rec.AsMap(paper.nodes["s2"])["children"] = row.children
		rec.AsMap(paper.nodes["list"])["children"] = row.list
		if got := htmlkit.BibliographySection(paper.document); got != row.want {
			t.Errorf("%v: %q, want %q", row.children, got, row.want)
		}
	}
	split := landmarkPaper()
	split.document["citations"] = rec.Map{"entries": []any{rec.Map{"blocks": []any{"e1", "thp"}}}}
	if htmlkit.BibliographySection(split.document) != "" {
		t.Error("entries in two sections make a bibliography")
	}
	if htmlkit.BibliographySection(rec.Map{"nodes": rec.Map{}}) != "" {
		t.Error("a document without citations has a bibliography")
	}
}

// The palettes reach 4.5:1 for text and 3:1 for rules, borders and the focus ring, in both themes.
func TestPalettesMeetTheContrastWCAGAsks(t *testing.T) {
	if gaps := htmlkit.ThemeContrastGaps(pageStyle); len(gaps) != 0 {
		t.Errorf("contrast gaps %v", gaps)
	}
	weak := strings.Replace(pageStyle, "--rule: #858585;", "--rule: #cfcfcf;", 1)
	gaps := htmlkit.ThemeContrastGaps(weak)
	if len(gaps) != 2 || rec.AsMap(gaps[0])["foreground"] != "--rule" || rec.AsMap(gaps[0])["theme"] != "light" {
		t.Errorf("weak rule gaps %v", gaps)
	}
	if gaps := htmlkit.ThemeContrastGaps(""); len(gaps) != 2 {
		t.Errorf("a page without palettes %v", gaps)
	}
}

// Focus, reduced motion and forced colours are in the stylesheet, CSS only.
func TestVisualRulesAreInTheStylesheet(t *testing.T) {
	for _, want := range []string{
		"a:focus-visible, button:focus-visible, [tabindex]:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }",
		"@media (prefers-reduced-motion: reduce) { :target { animation: none;",
		"@media (forced-colors: active) {",
		// decision 113 amendment: text the author hid is not painted, so no checker measures it
		`main [aria-hidden="true"][data-kind="text"] { visibility: hidden; }`,
		`[data-kind="rule"]:not([data-color]), [data-rule], [data-rich-rule] { forced-color-adjust: none; background: CanvasText !important; }`,
	} {
		if !strings.Contains(pageStyle, want) {
			t.Errorf("stylesheet lacks %s", want)
		}
	}
	if !strings.Contains(blockStyle, "var(--alert)") {
		t.Error("the alerted block's border is not themed")
	}
	for check := range htmlkit.VisualRules {
		if missing := htmlkit.MissingVisualRules(check, pageStyle); len(missing) != 0 {
			t.Errorf("%s: the stylesheet lacks %v", check, missing)
		}
		if missing := htmlkit.MissingVisualRules(check, ""); len(missing) != len(htmlkit.VisualRules[check]) {
			t.Errorf("%s: an empty stylesheet lacks %v", check, missing)
		}
	}
}

// A note is a DPUB-ARIA footnote, not a landmark, named by its number when the number is plain.
func TestNotesAreFootnotesNotLandmarks(t *testing.T) {
	for number, want := range map[string]string{"3": `role="doc-footnote" aria-label="Footnote 3"`, "†": `aria-label="Footnote †"`, "⋆": `aria-label="Footnote ⋆"`,
		`\TextOrMath {\textasteriskcentered }{*}`: `aria-label="Footnote"`} {
		if got := noteRole(number); !strings.Contains(got, want) {
			t.Errorf("%q: %s, want %s", number, got, want)
		}
	}
}
