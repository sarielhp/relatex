package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/pagetree"
	"github.com/sarielhp/relatex/internal/rec"
)

const samplePage = `<!DOCTYPE html>
<html lang="de"><head><meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<meta http-equiv="Content-Security-Policy" content="default-src 'none'"/>
<title>A &amp; B</title><style>p > span { color: red; }</style><script src="preview.js" defer="defer"></script></head>
<body><p role="status">Notice</p><p class="page-links"><a href="#contents">Contents</a></p><main>` +
	`<div id="root" data-structure="document" class="document"><p id="front">Front&nbsp;matter <span data-cell-text>x</span><br></p>` +
	`<section id="s1"><h2 id="h1">One</h2><p id="p1">See <a href="#p2">two</a> and <math><mi>x</mi></math>.</p></section>` +
	`<section id="s2"><h2 id="h2"><math><mi>y</mi></math></h2><p id="p2">Back to <a href="#p1">one</a>.` +
	`<img src="figs/plot.svg" alt="Figure 1" data-undescribed="true"/></p><p id="b1"><span data-bibliography-entry="1">Ref</span></p></section></div>` +
	`<aside id="note" data-note="1" aria-label="Footnote"><p>Note</p></aside></main>` +
	`<nav id="contents" aria-label="Contents"><h2>Contents</h2><ol><li><a href="#h1">One</a></li><li><a href="#h2"></a></li></ol></nav>` +
	`<script>alert(1)</script></body></html>
`

func wellFormed(t *testing.T, name, text string) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(text))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("%s is not well-formed XML: %v\n%s", name, err, text)
		}
	}
}

func TestMarkupBecomesXHTML(t *testing.T) {
	p := preparePage(samplePage)
	document := p.xhtml(contentDocument{name: FirstDocument, body: p.body.Children})
	wellFormed(t, "page", document)
	for _, want := range []string{`<html lang="de" data-theme="light" xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="de">`,
		`<span data-cell-text="">x</span><br/>`, "Front\u00a0matter", `<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi></math>`,
		`<title>A &amp; B</title>`, `<link rel="stylesheet" href="style.css"/>`} {
		if !strings.Contains(document, want) {
			t.Errorf("missing %q in\n%s", want, document)
		}
	}
	for _, unwanted := range []string{"<script", "Content-Security-Policy", "<style"} {
		if strings.Contains(document, unwanted) {
			t.Errorf("unexpected %q in\n%s", unwanted, document)
		}
	}
	if p.style != "p > span { color: red; }" {
		t.Errorf("stylesheet %q", p.style)
	}
	if p.title() != "A & B" {
		t.Errorf("title %q", p.title())
	}
}

func TestForeignContentKeepsCaseAndNamespaces(t *testing.T) {
	p := preparePage(`<html><head><title>t</title></head><body><svg viewBox="0 0 1 1"><use xlink:href="#g"/><g id="g"><path d="M0 0"/></g></svg>` +
		`<math><mspace width="1em"/><mi>a</mi></math></body></html>`)
	document := p.xhtml(contentDocument{name: FirstDocument, body: p.body.Children})
	wellFormed(t, "page", document)
	for _, want := range []string{`<svg viewBox="0 0 1 1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">`,
		`<use xlink:href="#g"></use>`, `<mspace width="1em"></mspace><mi>a</mi>`} {
		if !strings.Contains(document, want) {
			t.Errorf("missing %q in\n%s", want, document)
		}
	}
	if got := properties(contentDocument{body: p.body.Children}); strings.Join(got, " ") != "mathml svg" {
		t.Errorf("properties %v", got)
	}
}

func TestSplitBySectionsRewritesLinks(t *testing.T) {
	p := preparePage(samplePage)
	documents, reason := splitPage(p)
	if reason != "" || len(documents) != 3 {
		t.Fatalf("split into %d (%q)", len(documents), reason)
	}
	names := []string{documents[0].name, documents[1].name, documents[2].name}
	if strings.Join(names, " ") != "index.xhtml section-1.xhtml section-2.xhtml" {
		t.Fatalf("names %v", names)
	}
	texts := make([]string, 3)
	for index, document := range documents {
		texts[index] = p.xhtml(document)
		wellFormed(t, document.name, texts[index])
	}
	checks := []struct {
		document int
		want     string
	}{
		{0, `<p role="status">Notice</p>`}, {0, `<a href="section-2.xhtml#contents">Contents</a>`},
		{0, `<div id="root" data-structure="document" class="document"><p id="front">`},
		{1, `<a href="section-2.xhtml#p2">two</a>`}, {1, `<div data-structure="document" class="document"><section id="s1">`},
		{2, `<a href="section-1.xhtml#p1">one</a>`}, {2, `<aside id="note"`}, {2, `<nav id="contents"`},
	}
	for _, check := range checks {
		if !strings.Contains(texts[check.document], check.want) {
			t.Errorf("document %d lacks %q:\n%s", check.document, check.want, texts[check.document])
		}
	}
	if strings.Contains(texts[1], "Notice") || strings.Contains(texts[1], "<aside") {
		t.Errorf("a middle document holds the notice or the notes:\n%s", texts[1])
	}
}

func TestSplitKeepsOneDocumentForCrossSectionIDReferences(t *testing.T) {
	page := strings.Replace(samplePage, `<p id="p2">`, `<p id="p2" aria-describedby="p1">`, 1)
	documents, reason := splitPage(preparePage(page))
	if len(documents) != 1 || !strings.Contains(reason, "#p1") {
		t.Fatalf("documents %d, reason %q", len(documents), reason)
	}
	single := `<html><head><title>t</title></head><body><main><div data-structure="document"><section id="a"></section></div></main></body></html>`
	if documents, reason := splitPage(preparePage(single)); len(documents) != 1 || reason == "" {
		t.Fatalf("one section split into %d (%q)", len(documents), reason)
	}
}

func TestNavigationContentsAndLandmarks(t *testing.T) {
	p := preparePage(samplePage)
	documents, _ := splitPage(p)
	nav := navigation(p, documents, nil, p.title())
	wellFormed(t, "nav", nav)
	for _, want := range []string{`<nav epub:type="toc" role="doc-toc" id="toc">`, `<a href="section-1.xhtml#h1">One</a>`,
		`<a href="section-2.xhtml#h2">y</a>`, `<a epub:type="bodymatter" href="index.xhtml">`,
		`<a epub:type="bibliography" href="section-2.xhtml#s2">Bibliography</a>`, `lang="de" xml:lang="de"`} {
		if !strings.Contains(nav, want) {
			t.Errorf("missing %q in\n%s", want, nav)
		}
	}
	bare := preparePage(`<html><head><title>Plain</title></head><body><p>text</p></body></html>`)
	documents, _ = splitPage(bare)
	nav = navigation(bare, documents, nil, bare.title())
	if !strings.Contains(nav, `<ol><li><a href="index.xhtml">Plain</a></li></ol>`) || strings.Contains(nav, "bibliography") {
		t.Errorf("navigation without headings:\n%s", nav)
	}
}

func TestDescribedKeysOnThePlaceholderFlag(t *testing.T) {
	image := func(attrs ...pagetree.Attribute) *pagetree.Node {
		return &pagetree.Node{Name: "img", Attrs: attrs}
	}
	if described(image(pagetree.Attribute{Name: "alt", Value: "Figure 3"},
		pagetree.Attribute{Name: htmlkit.UndescribedAttribute, Value: "true"}), "Figure 3") {
		t.Errorf("a placeholder caption label is no text alternative")
	}
	if !described(image(pagetree.Attribute{Name: "alt", Value: "A plot."}), "A plot.") {
		t.Errorf("a description is a text alternative")
	}
	if !described(image(pagetree.Attribute{Name: "alt", Value: ""},
		pagetree.Attribute{Name: "role", Value: "presentation"}), "") {
		t.Errorf("a decorative image needs no text alternative")
	}
}

func TestContainer(t *testing.T) {
	text := container()
	wellFormed(t, "container", text)
	if !strings.Contains(text, `<rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/>`) {
		t.Errorf("container:\n%s", text)
	}
}

func TestPackageDocumentMetadataAndManifest(t *testing.T) {
	metadata := Metadata{Identifier: "urn:uuid:x", Title: "T <1>", Creators: []string{"Ada", "Bob"}, Language: "en",
		Modified: "2026-10-18T21:08:12Z", Access: Accessibility{Images: 2, Described: 1, Formulas: 3, Headings: 4}}
	items := []item{{"i-nav", "nav.xhtml", "application/xhtml+xml", []string{"nav"}},
		{"i-index", "index.xhtml", "application/xhtml+xml", []string{"mathml"}}, {"i-fig", "figs/a.pdf", "application/pdf", nil}}
	opf := packageDocument(metadata, items, []string{"index.xhtml"})
	wellFormed(t, "opf", opf)
	for _, want := range []string{`unique-identifier="pub-id" xml:lang="en"`, `<dc:identifier id="pub-id">urn:uuid:x</dc:identifier>`,
		`<dc:title>T &lt;1&gt;</dc:title>`, "<dc:creator>Ada</dc:creator>\n<dc:creator>Bob</dc:creator>", `<dc:language>en</dc:language>`,
		`<meta property="dcterms:modified">2026-10-18T21:08:12Z</meta>`, `<meta property="schema:accessMode">visual</meta>`,
		`<meta property="schema:accessModeSufficient">textual,visual</meta>`, `<meta property="schema:accessibilityFeature">MathML</meta>`,
		`<meta property="schema:accessibilityFeature">structuralNavigation</meta>`, `<meta property="schema:accessibilityHazard">none</meta>`,
		"1 figure of 2 has no text alternative yet", "is not claimed",
		`<item id="i-index" href="index.xhtml" media-type="application/xhtml+xml" properties="mathml"/>`,
		`<spine>` + "\n" + `<itemref idref="i-index"/>`} {
		if !strings.Contains(opf, want) {
			t.Errorf("missing %q in\n%s", want, opf)
		}
	}
	for _, unwanted := range []string{"conformsTo", "alternativeText", `accessModeSufficient">textual<`} {
		if strings.Contains(opf, unwanted) {
			t.Errorf("unexpected %q in\n%s", unwanted, opf)
		}
	}
}

func TestAccessibilityMetadataFollowsTheEvidence(t *testing.T) {
	text := func(access Accessibility) string {
		var builder strings.Builder
		for _, pair := range accessibilityMetadata(access) {
			builder.WriteString(pair[0] + "=" + pair[1] + "\n")
		}
		return builder.String()
	}
	plain := text(Accessibility{})
	if strings.Contains(plain, "visual") || strings.Contains(plain, "MathML") || strings.Contains(plain, "structuralNavigation") ||
		!strings.Contains(plain, "schema:accessModeSufficient=textual\n") {
		t.Errorf("text-only document:\n%s", plain)
	}
	described := text(Accessibility{Images: 2, Described: 2})
	if !strings.Contains(described, "accessibilityFeature=alternativeText") || !strings.Contains(described, "accessModeSufficient=textual\n") {
		t.Errorf("every figure described:\n%s", described)
	}
	if strings.Contains(described, "longDescription") {
		t.Errorf("long descriptions claimed without one:\n%s", described)
	}
	// A long description (decision 110) is a feature of its own.
	long := text(Accessibility{Images: 2, Described: 2, Long: 1})
	if !strings.Contains(long, "accessibilityFeature=longDescription") || !strings.Contains(long, "1 figure has a long description") {
		t.Errorf("a figure with a long description:\n%s", long)
	}
	var builder strings.Builder
	writeMetadata(&builder, Metadata{Modified: fallbackModified, Source: "book.pdf, SHA-256 ab", PageBreakSource: pageBreakSource("book.pdf, SHA-256 ab", "B"),
		Access: Accessibility{Claim: htmlkit.WCAGClaim, Pages: 3}})
	if got := pageBreakSource("", "A & B"); got != "relatex: PDF page breaks of A & B" {
		t.Errorf("page-break source without the PDF %q", got)
	}
	for _, want := range []string{`<meta property="pageBreakSource">book.pdf, SHA-256 ab</meta>`, `<meta property="dcterms:conformsTo">EPUB Accessibility 1.1 - WCAG 2.2 Level AA</meta>`,
		`<meta property="a11y:certifiedBy">relatex accessibility checks`, "<dc:language>und</dc:language>",
		`<dc:source id="print-source">book.pdf, SHA-256 ab</dc:source>` + "\n" + `<meta refines="#print-source" property="source-of">pagination</meta>`, "accessibilityFeature\">pageNavigation<", "accessibilityFeature\">printPageNumbers<",
		"listed in a page list", "as established by the export's accessibility checks"} {
		if !strings.Contains(builder.String(), want) {
			t.Errorf("claimed metadata lacks %q:\n%s", want, builder.String())
		}
	}
	// No claim without the page list, nor while a description is an unreviewed draft.
	for _, access := range []Accessibility{{Claim: htmlkit.WCAGClaim}, {Claim: htmlkit.WCAGClaim, Pages: 3, Images: 1, Described: 1, Drafts: 1}} {
		listed := text(access)
		if strings.Contains(listed, "conformsTo") || strings.Contains(listed, "certifiedBy") || !strings.Contains(listed, "not claimed") {
			t.Errorf("unearned claim for %+v:\n%s", access, listed)
		}
	}
	drafted := text(Accessibility{Images: 3, Described: 2, Drafts: 2})
	if !strings.Contains(drafted, "1 figure of 3 has no text alternative yet. 2 figure descriptions were drafted by a model and not yet reviewed by the author.") {
		t.Errorf("summary with drafts:\n%s", drafted)
	}
}

// TestRelatexAltIsNoDraft: a description the source gives with \relatexalt (decision 125,
// amendment 2) is the author's, so it never holds back the EPUB's claim as a model draft does.
func TestRelatexAltIsNoDraft(t *testing.T) {
	images := []any{rec.Map{"type": "image", "alt_provenance": "source_macro"}, rec.Map{"type": "image", "alt_provenance": "model"},
		rec.Map{"type": "image", "alt_provenance": "source"}}
	if got := draftedImages(rec.Map{"nodes": rec.Map{"n": rec.Map{"content": images}}}, 0); got != 1 {
		t.Errorf("drafts %d, want 1", got)
	}
}

func TestArchiveIsReproducibleWithMimetypeFirst(t *testing.T) {
	directory := t.TempDir()
	entries := []entry{{containerPath, []byte(container())}, {"EPUB/a.txt", []byte(strings.Repeat("text ", 100))}}
	first, second := filepath.Join(directory, "a.epub"), filepath.Join(directory, "b.epub")
	writeArchive(first, entries)
	writeArchive(second, entries)
	one, _ := os.ReadFile(first)
	two, _ := os.ReadFile(second)
	if !bytes.Equal(one, two) {
		t.Fatal("the same entries gave different archives")
	}
	if !bytes.HasPrefix(one[30:], []byte("mimetypeapplication/epub+zip")) {
		t.Fatalf("mimetype is not the first, stored entry without extra field: %q", one[:70])
	}
	reader, err := zip.NewReader(bytes.NewReader(one), int64(len(one)))
	if err != nil {
		t.Fatal(err)
	}
	if reader.File[0].Method != zip.Store || reader.File[2].Method != zip.Deflate || reader.File[2].Name != "EPUB/a.txt" {
		t.Errorf("entries %v", reader.File)
	}
	for _, file := range reader.File {
		if file.Modified.Year() != 1980 || len(file.Extra) != 0 {
			t.Errorf("%s: timestamp %v, extra %v", file.Name, file.Modified, file.Extra)
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(opened)
		if err != nil {
			t.Fatalf("%s: %v", file.Name, err)
		}
		if file.Name == "EPUB/a.txt" && string(data) != strings.Repeat("text ", 100) {
			t.Errorf("content %q", data)
		}
	}
}

func TestModifiedDate(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	if got := ModifiedDate(""); got != "2023-11-14T22:13:20Z" {
		t.Errorf("SOURCE_DATE_EPOCH gave %s", got)
	}
	t.Setenv("SOURCE_DATE_EPOCH", "")
	pdf := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF\n<< /CreationDate (D:20261018230812+02'00') >>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ModifiedDate(pdf); got != "2026-10-18T21:08:12Z" {
		t.Errorf("PDF date gave %s", got)
	}
	if got := ModifiedDate(filepath.Join(t.TempDir(), "missing.pdf")); got != fallbackModified {
		t.Errorf("missing PDF gave %s", got)
	}
}

func TestIdentifierIsANameBasedUUID(t *testing.T) {
	files := map[string][]byte{"index.xhtml": []byte("a")}
	id := identifier("abc", files)
	if id != identifier("abc", files) || id == identifier("abd", files) || len(id) != len("urn:uuid:")+36 || id[len("urn:uuid:")+14] != '5' {
		t.Errorf("identifier %s", id)
	}
	corrected := map[string][]byte{"index.xhtml": []byte("b")}
	if identifier("abc", corrected) != id {
		t.Error("a corrected package of the same sources is another publication")
	}
	if identifier("", files) == identifier("", corrected) {
		t.Error("packages without sources are not told apart by their files")
	}
	if itemID("figs/a b.png", map[string]bool{"i-figs_a_b.png": true}) != "i-figs_a_b.png-2" {
		t.Error("item ids are not unique")
	}
}

func TestReadClaim(t *testing.T) {
	directory := t.TempDir()
	if ReadClaim(filepath.Join(directory, "accessibility.json")) != "" {
		t.Error("an absent report claims")
	}
	path := filepath.Join(directory, "accessibility.json")
	for text, want := range map[string]string{`{"claim": null}`: "", `{"claim": "WCAG 2.2 Level AA"}`: htmlkit.WCAGClaim} {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := ReadClaim(path); got != want {
			t.Errorf("%s gave %q", text, got)
		}
	}
}

func TestConformToEPUBContentModels(t *testing.T) {
	p := preparePage(`<html><head><title>t</title></head><body>` +
		`<math><mtable><mtr><mtd intent=":equation-label"><mtext>(1)</mtext></mtd></mtr></mtable>` +
		`<mrow intent="norm($v)"><mo>‖</mo><mi arg="v">v</mi><mo>‖</mo></mrow>` +
		`<mspace mathbackground="currentColor" style="width:1px"/><mo fence="true" symmetric="true">⌊</mo><mspace mathbackground="#000000"/></math>` +
		`<figure><p>a</p><figcaption id="c">Caption</figcaption><p>b</p></figure>` +
		`<figure><p>c</p><figcaption>Kept</figcaption></figure></body></html>`)
	document := p.xhtml(contentDocument{name: FirstDocument, body: p.body.Children})
	for _, want := range []string{`<mtd data-intent=":equation-label"><mtext>(1)</mtext></mtd>`, `<mrow data-intent="norm($v)"><mo>‖</mo><mi data-arg="v">v</mi>`,
		`<mspace style="width:1px;background-color:currentColor"></mspace>`,
		`<mspace mathbackground="#000000"></mspace>`, `<mo data-fence="true" data-symmetric="true">⌊</mo>`, `<figure aria-labelledby="c"><p>a</p><div id="c" class="figcaption">Caption</div>`, `<figcaption>Kept</figcaption>`} {
		if !strings.Contains(document, want) {
			t.Errorf("missing %q in\n%s", want, document)
		}
	}
	if p.adjustments != (Adjustments{IntentsAsData: 2, ColorsAsStyle: 1, FencesAsData: 2, CaptionsAsDivisions: 1}) {
		t.Errorf("adjustments %+v", p.adjustments)
	}
}

// Every display formula is a named group the keyboard reaches, so a wide one can be scrolled without
// the page's script (decision 095).
func TestDisplaysAreFocusable(t *testing.T) {
	p := preparePage(`<html><head><title>t</title></head><body><div id="d1" data-block="display_math" class="display_math">` +
		`<span data-kind="math"><math><mi>x</mi></math></span><span data-kind="text">(3)</span></div>` +
		`<div id="d2" data-block="display_math" class="display_math"><span data-kind="math"><math><mi>y</mi></math></span></div></body></html>`)
	document := p.xhtml(contentDocument{name: FirstDocument, body: p.body.Children})
	for _, want := range []string{`tabindex="0" role="group" aria-label="Equation (3), scrollable"`,
		`tabindex="0" role="group" aria-label="Display formula, scrollable"`} {
		if !strings.Contains(document, want) {
			t.Errorf("missing %q in\n%s", want, document)
		}
	}
}
