// Package html is the Go port of relatex's HTML reading copy: HTML and PaperHTML, with
// PaperTableHTML, PaperImages and the presentation side of RichTableLayout, TableSections
// and ClassicTableProjection. A reading aid, with its incomplete qualification visible in
// the artifact.
package html

import (
	"slices"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/mathintent"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
	"github.com/sarielhp/relatex/internal/stylemodel"
)

// pageStyle is the reading page's stylesheet: the theme of the reader's system (prefers-color-scheme,
// dark when it states none) unless data-theme="light" or "dark" chooses one (the toggle in
// preview.js, or a reader's own choice), both from one set of custom properties.
var pageStyle = `:root { ` + darkTheme + ` font-family: Georgia, serif; color: var(--ink); background: var(--bg); }
:root[data-theme="light"] { ` + lightTheme + ` }
@media (prefers-color-scheme: light) { :root:not([data-theme="dark"]) { ` + lightTheme + ` } }
body { max-width: 110ch; margin: 1rem auto; padding: 0 1rem 2rem; line-height: 1.65; }
.page-links { font-family: system-ui, sans-serif; font-size: .9rem; color: var(--muted); margin: 0 0 1rem; }
nav { font-family: system-ui, sans-serif; border-top: 1px solid var(--rule); margin-top: 2.5rem; padding-top: 1rem; }
nav h2 { font-size: 1.1rem; margin: 0 0 .5rem; }
nav ol { list-style: none; margin: 0; padding: 0; }
nav ol ol { padding-left: 1.5rem; }
nav li { margin: .25rem 0; }
a { color: var(--link); text-underline-offset: .2em; }
h2, h3, h4, h5, h6 { font-size: 1.5em; line-height: 1.3; margin: 2rem 0 .83em; }
h1.title { font-size: 1em; font-weight: normal; margin: 1rem 0; white-space: pre-wrap; }
th { font-weight: inherit; }
.skip-link { position: absolute; left: .5rem; top: -10rem; z-index: 4; font: .9rem system-ui, sans-serif; color: var(--link); background: var(--card); border: 1px solid var(--rule); border-radius: .3rem; padding: .4rem .8rem; }
.skip-link:focus { top: .5rem; }
p { margin: 1rem 0; white-space: pre-wrap; }
p[data-toc-entry] { margin: .3rem 0; }
.theorem, .proof { margin: 1.25rem 0; padding: .6rem 1rem; border-left: .4rem solid var(--theorem); background: var(--theorem-bg); border-radius: 0 .3rem .3rem 0; }
.proof { border-left-color: var(--proof); background: var(--proof-bg); }
.theorem > :first-child, .proof > :first-child { margin-top: 0; }
.theorem > :last-child, .proof > :last-child { margin-bottom: 0; }
.proof [data-name="qed_open_square"], .proof [data-name="qed"] { float: right; margin-left: 1rem; font-size: 1.15em; }
[data-align="center"] { text-align: center; }
[data-align="right"] { text-align: right; }
[data-align="center"] > table { margin-left: auto; margin-right: auto; }
[data-align="right"] > table { margin-left: auto; }
[data-frame-hidden] { visibility: hidden; }
[data-frame] { margin-block: 0; padding: 0 3pt; border: 0 solid currentColor; border-width: 0 .4pt; }
[data-frame="first"] { border-top-width: .4pt; }
[data-frame="last"] { border-bottom-width: .4pt; }
[data-frame="box"], [data-frame="shadow"] { position: relative; overflow: hidden; margin-block: .6em; border-width: .4pt; width: fit-content; max-width: 100%; box-sizing: border-box; }
[data-frame="shadow"] { box-shadow: 4pt 4pt 0 currentColor; margin-right: 4pt; }
[data-frame="box"] [data-frame-hidden], [data-frame="shadow"] [data-frame-hidden] { position: absolute !important; top: 0; left: 0; }
.display_math { margin: 1.5rem 0; overflow-x: auto; padding: .25rem; }
.display_math:has(> [data-kind="text"]) > [data-kind="math"] { display: inline-block; width: calc(100% - 5rem); text-align: center; }
.display_math > [data-kind="text"] { display: inline-block; width: 5rem; text-align: right; }
.display_math:has(> [data-kind="math"] > math:not([display="block"])) { display: flex; align-items: center; }
.display_math:has(> [data-kind="math"] > math:not([display="block"])) > [data-kind] { width: auto; flex: none; }
.display_math:has(> [data-kind="math"] > math:not([display="block"])) > [data-kind="math"]:has(> math[display="block"]) { flex: 1; }
math { font-family: ` + mathexport.MathFontFamily + `; }
mpadded { white-space: nowrap; }
` + joinCSS + `img, svg[data-kind="image"] { background: var(--figure-bg); }
svg[data-kind="image"] { font-style: normal; font-weight: normal; font-variant: normal; font-stretch: normal; letter-spacing: normal; word-spacing: normal; text-transform: none; text-decoration: none; }
@keyframes target-fade { 0%, 60% { background: var(--target); } 100% { background: transparent; } }
:target { animation: target-fade 12s ease-out forwards; scroll-margin-top: 2rem; }
.preview-card { position: absolute; z-index: 2; max-width: min(60ch, 100vw - 1rem); max-height: 45vh; overflow: auto; padding: .5rem .75rem; background: var(--card); color: var(--ink); border: 1px solid var(--rule); border-radius: .4rem; box-shadow: 0 .3rem 1rem rgba(0, 0, 0, .35); }
a[data-kind="note_reference"] { vertical-align: .4rem; line-height: 0; }
[data-note-backref] { display: inline-block; min-width: 24px; min-height: 24px; text-align: center; }
.long-description { margin: .25rem 0 .75rem; font-size: .9em; }
.long-description summary { min-height: 24px; color: var(--muted); cursor: pointer; }
.long-description p, .figure-details { margin: .25rem 0 0; }
.preview-card[hidden] { display: none; }
.preview-card > :first-child { margin-top: 0; }
.preview-card > :last-child { margin-bottom: 0; }
.theme-toggle { position: fixed; top: .5rem; right: .5rem; z-index: 3; font: .8rem system-ui, sans-serif; color: var(--muted); background: var(--card); border: 1px solid var(--rule); border-radius: 1rem; padding: .2rem .7rem; cursor: pointer; }
@media (max-width: 600px) { body { margin-top: .5rem; padding: 0 .75rem 2rem; } }
` + codeCSS + visualStyle

// textPaintColor is PDFTextPaint::COLOR.
const textPaintColor = "rgb(0, 0, 0)"

var structures = []string{"document", "section", "theorem", "proof"}

var floatTypes = []string{"figure_float", "table_float", "float_caption"}

// textDecorationLines is the CSS text-decoration line of each text decoration (decision 134).
var textDecorationLines = map[string]string{"underline": "underline", "strike": "line-through"}

// Paper renders one paper document (PaperHTML).
type Paper struct {
	document    rec.Map
	nodes       rec.Map
	descriptors []rec.Map
	described   bool
	figures     map[string]string
	base        int64 // the body text size in scaled points; text runs are sized relative to it
	absolute    bool  // inside the pixel-exact table projection, where runs keep their TeX pixel size
	// inlineCaptions are the captions written as div because they sit between their float's blocks.
	inlineCaptions map[string]bool
	// tableNames are the captions that name the tables their float holds, by table_grid id
	// (a11y-804).
	tableNames map[string]string
	// caption is the first caption of the float whose children are being written: an undescribed
	// image's alt is its label, which the image names with aria-describedby.
	caption rec.Map
	// declaredInks are the inks the page declares as custom properties (inkColor), computed once.
	declaredInks map[string]bool
	// hiddenInks are the inks the author hid (contrast.Hidden), computed once; fill is the
	// author's fill behind the span being written (contrast.Behind), "" on the page.
	hiddenInks map[string]bool
	fill       string
	// cellFill is the colortbl fill of the table cell being written (contrast.CellFill), or of the
	// cell around its table; "" outside filled cells.
	cellFill string
	// backdrops are the fills the PDF paints behind faint text (contrast.Backdrops, decision 113),
	// read once.
	backdrops *contrast.Backdrops
	// indexes is the linked index (decision 084), planned once.
	indexes *indexPlan
	// longDescriptions are the disclosures of the described images written since the block being
	// rendered began; the block's markup is followed by them (decision 110).
	longDescriptions []string
	// headingDepths are the sections around each heading, and title the title's paragraph
	// (decision 088), both read once.
	headingDepths map[string]int
	title         *string
	// tocEntries are the paragraphs of a printed contents list (tocentries.go), read once.
	tocEntries map[string]bool
	// frameCuts are the rule spans of a rule frame's paragraphs that its border replaces (ruleframe.go).
	frameCuts map[string]map[int]bool
	// printedContents is whether the paper prints its own table of contents (contents.go), read once.
	printedContents *bool
	// runin is the run-in title the next content list opens with (runin.go).
	runin runinTitle
	// bibliography is the bibliography's section (decision 095), found once.
	bibliography *string
	// bibRun is the bibliography's run of entries when no section holds it alone, found once
	// (decision 111); bibID is the wrapper's id.
	bibRun *bibliographyRun
	bibID  string
	// parents is the document's child -> parent map (reference names, decision 111), read once.
	parents map[string]string
	// previous is the text span just before the span being written in its content list; a
	// reference's name is omitted when the sentence already says its kind.
	previous string
	// macros are the paper's kept math macros, read once for the formulas' intent (decision 109).
	macros mathintent.Macros
}

// New is PaperHTML.new(document): the document is validated.
func New(document rec.Map) *Paper {
	paperdoc.Validate(document)
	figures, _ := figures.Plan(document)
	return &Paper{document: document, nodes: rec.FetchMap(document, "nodes"), figures: figures, base: stylemodel.BaseSize(document),
		macros: mathintent.MacrosOf(document)}
}

// Render is PaperHTML.new(document).render.
func Render(document rec.Map) string { return New(document).Render() }

func (p *Paper) node(id any) rec.Map { return rec.Node(p.nodes, id) }

// previewScriptTag loads the reference previews from preview.js beside the page (WritePreviewScript):
// without the script the links stay plain links, as in the EPUB package.
const previewScriptTag = `<script src="preview.js" defer="defer"></script>`

// Render is PaperHTML#render.
func (p *Paper) Render() string {
	html := "<!DOCTYPE html>\n<html lang=\"" + htmlkit.Escape(htmlkit.DocumentLanguage(p.document)) + "\"><head><meta charset=\"utf-8\"/>\n" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"/>\n" +
		"<meta http-equiv=\"Content-Security-Policy\" content=\"" + p.contentSecurityPolicy() + "\"/>\n" +
		"<title>Reading copy</title><style>" + p.stylesheet() + "</style>" +
		previewScriptTag + "</head>\n" +
		"<body>" + SkipLink + p.pageHeader() + p.contentsAtTop() + "<main id=\"" + htmlkit.MainID + "\">" + p.RenderNode(rec.FetchString(p.document, "root")) +
		p.afterMainContent() + "</main>" + p.contentsAtEnd() + "</body></html>\n"
	front, ok := p.document["frontmatter"].(rec.Map)
	if !ok {
		return html
	}
	authors, err := canonjson.Marshal(rec.Fetch(front, "authors"))
	if err != nil {
		rec.Fail("%v", err)
	}
	metadata := "<meta name=\"author\" content=\"" + htmlkit.Escape(string(authors)) + "\"/>"
	if abstract, has := front["abstract"].(string); has {
		metadata += "<meta name=\"description\" content=\"" + htmlkit.Escape(abstract) + "\"/>"
	}
	return strings.Replace(html, "<title>Reading copy</title>", "<title>"+htmlkit.Escape(rec.FetchString(front, "title"))+"</title>"+metadata, 1)
}

const statusNotice = `<p role="status">Diagnostic reading copy. Full conversion verification is incomplete.</p>`

func (p *Paper) contentSecurityPolicy() string {
	// Images are files beside the page (figs/): its origin when served, file: when opened locally.
	policy := "default-src 'none'; img-src data: 'self' file:; style-src 'unsafe-inline'; script-src 'self' file:; base-uri 'none'; form-action 'none'"
	// Linked fonts sit next to the page: its origin when served, file: when opened locally.
	for _, font := range htmlkit.FontItems(p.document) {
		if truthy(font["file"]) {
			return policy + "; font-src data: 'self' file:;"
		}
	}
	if mathexport.AccentFontUsed(p.document) {
		return policy + "; font-src 'self' file:;"
	}
	return policy
}

func truthy(value any) bool { return value != nil && value != false }

func (p *Paper) stylesheet() string {
	return pageStyle + p.blockCSS() + inksCSS(p.document) + htmlkit.SourceFontsCSS(p.document) + mathexport.AccentFontCSS(p.document) + textPaintCSS(p.document) + itemMarkerCSS(p.document) + htmlkit.LinkCSS
}

func textPaintCSS(document rec.Map) string {
	if !truthy(document["text_paint"]) {
		return ""
	}
	paintmodel.ValidateText(document)
	return "main{color:var(--ink)}main a{color:inherit}"
}

func (p *Paper) afterMainContent() string {
	var builder strings.Builder
	for _, id := range rec.FetchList(p.document, "note_order") {
		builder.WriteString(p.RenderNode(rec.AsString(id)))
	}
	return builder.String()
}

func (p *Paper) children(node rec.Map) string {
	identifier, _ := node["id"].(string)
	parent, first, last, run := p.bibliographyRun()
	var builder strings.Builder
	children := rec.FetchList(node, "children")
	frames := p.ruleFrames(children)
	sides := frameSides(frames)
	for index, child := range children {
		if run && parent == identifier && index == first {
			builder.WriteString(p.bibliographyOpen())
		}
		builder.WriteString(markFrameSide(p.RenderNode(rec.AsString(child)), sides[index]))
		if run && parent == identifier && index == last {
			builder.WriteString("</div>")
		}
	}
	return builder.String()
}

var listTags = map[string]string{"unordered_list": "ul", "ordered_list": "ol", "list_item": "li"}

// RenderNode is PaperHTML#render_node: the node's markup, followed by the long descriptions of
// the images it holds that no block inside it placed (decision 110).
func (p *Paper) RenderNode(identifier string) string {
	return p.withLongDescriptions(func() string { return p.renderNode(identifier) })
}

func (p *Paper) renderNode(identifier string) string {
	node := p.node(identifier)
	kind := rec.FetchString(node, "type")
	if slices.Contains(floatTypes, kind) {
		return p.floatNode(node)
	}
	if tag, ok := listTags[kind]; ok {
		return "<" + tag + " id=\"" + identifier + "\" data-structure=\"" + kind + "\" class=\"" + kind +
			"\"" + listStyle(node) + ">" + p.children(node) + "</" + tag + ">"
	}
	if kind == "document_page_total" {
		value := toS(rec.Fetch(node, "count"))
		if content, ok := node["content"].([]any); ok && len(content) > 0 {
			value = p.spans(content)
		}
		return "<aside id=\"" + identifier + "\" data-document-page-total=\"" + toS(rec.Fetch(node, "count")) +
			"\" data-source-label=\"TotPages\" data-pdf-sha256=\"" + toS(rec.Fetch(node, "pdf_sha256")) +
			"\" data-source-anchor=\"" + htmlkit.Escape(rec.FetchString(node, "anchor")) + "\">Source PDF: " + value + " pages.</aside>"
	}
	content, _ := node["content"].([]any)
	if slices.ContainsFunc(content, func(span any) bool { return rec.AsMap(span)["type"] == "table_grid" }) {
		return "<div id=\"" + identifier + "\" data-block=\"" + kind + "\" class=\"paragraph\"" + alignAttribute(node) + ">" + p.spans(content) + "</div>"
	}
	if kind == "note" {
		return p.noteNode(identifier, node)
	}
	if kind == "code" {
		return p.codeNode(identifier, node)
	}
	return p.plainNode(identifier, node, kind)
}

var blockTags = map[string]string{"heading": "h2", "paragraph": "p", "display_math": "div"}

// plainNode is HTML#render_node.
func (p *Paper) plainNode(identifier string, node rec.Map, kind string) string {
	if family := BlockFamily(node); family != "" {
		return p.blockNode(identifier, node, family)
	}
	if slices.Contains(structures, kind) {
		tag, attributes := "div", ""
		switch kind {
		case "section":
			tag, attributes = "section", p.sectionAttributes(identifier, node)
		case "theorem", "proof":
			attributes = p.groupAttributes(node, kind)
		}
		return "<" + tag + " id=\"" + identifier + "\" data-structure=\"" + kind + "\" class=\"" + kind + "\"" + attributes + ">" +
			p.children(node) + "</" + tag + ">"
	}
	tag, ok := blockTags[kind]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rubyInspect(kind), Lookup: true})
	}
	class, name := kind, ""
	if kind == "paragraph" {
		p.runin = p.runinTitleOf(identifier, node)
	}
	content := p.spans(p.frameRules(identifier, rec.FetchList(node, "content")))
	switch {
	case kind == "heading":
		tag, name = p.headingTag(identifier), headingName(node, content)
	case kind == "paragraph" && identifier == p.titleBlock():
		tag, class = "h1", kind+" title"
	}
	if kind == "paragraph" && p.contentsEntries()[identifier] {
		name += " data-toc-entry"
	}
	name += alignAttribute(node)
	return "<" + tag + " id=\"" + identifier + "\" data-block=\"" + kind + "\" class=\"" + class + "\"" + name + ">" +
		content + "</" + tag + ">"
}

func (p *Paper) noteNode(identifier string, note rec.Map) string {
	var labels strings.Builder
	for _, value := range rec.FetchList(note, "labels") {
		label := rec.AsMap(value)
		content := ""
		if _, styled := label["style"]; styled {
			content = p.renderSpan(rec.Merge(rec.Pick(label, "text", "style"), rec.Map{"type": "text"}))
		} else {
			content = htmlkit.Escape(rec.FetchString(label, "text"))
		}
		labels.WriteString("<span data-note-label data-block-target=\"" + toS(rec.Fetch(label, "block")) + "\">" + content + "</span>")
	}
	var links strings.Builder
	for _, value := range rec.FetchList(note, "callsites") {
		links.WriteString("<a data-note-backref href=\"#" + toS(rec.Fetch(rec.AsMap(value), "id")) +
			"\" aria-label=\"Return to note call\">↩</a>")
	}
	return "<aside id=\"" + identifier + "\" data-note=\"" + htmlkit.Escape(rec.FetchString(note, "number")) + "\"" + noteRole(rec.FetchString(note, "number")) + ">" +
		"<div data-note-labels>" + labels.String() + "</div>" + p.children(note) + "<div data-note-backlinks>" + links.String() + "</div></aside>"
}

func (p *Paper) floatNode(node rec.Map) string {
	kind := rec.FetchString(node, "type")
	id := toS(rec.Fetch(node, "id"))
	if kind == "float_caption" {
		content := p.children(node)
		if p.inlineCaptions[id] {
			return "<div id=\"" + id + "\" data-structure=\"float_caption\" class=\"figcaption\">" + content + "</div>"
		}
		return "<figcaption id=\"" + id + "\" data-structure=\"float_caption\">" + content + "</figcaption>"
	}
	for caption, tag := range htmlkit.CaptionTags(node, p.nodes) {
		if tag == "div" {
			if p.inlineCaptions == nil {
				p.inlineCaptions = map[string]bool{}
			}
			p.inlineCaptions[caption] = true
		}
	}
	outerCaption := p.caption
	p.caption = nil
	caption := ""
	for _, child := range rec.FetchList(node, "children") {
		if p.node(child)["type"] == "float_caption" {
			p.caption = p.node(child)
			caption = toS(child)
			break
		}
	}
	if kind == "table_float" && caption != "" {
		p.nameTables(node)
	}
	content := p.children(node)
	p.caption = outerCaption
	label := ""
	if htmlkit.FigureGrouped(node, p.nodes) {
		label = " role=\"group\""
	}
	return "<figure id=\"" + id + "\" data-structure=\"" + kind + "\" class=\"" + kind + "\" style=\"margin:0\"" + label + ">" + content + "</figure>"
}

// renderSpan is PaperHTML#render_span.
func (p *Paper) renderSpan(span rec.Map) string {
	switch rec.Fetch(span, "type") {
	case "table_grid":
		return p.TableGridSpan(span)
	case "image":
		return p.linkedImage(p.imageSpan(span), span)
	case "rule":
		return p.ruleSpan(span)
	}
	if span["line_break"] == true {
		// The break between two lines of one heading (decision 126): the space every reader reads,
		// then the line break the PDF shows, in one span the browser inventory reads back.
		return `<span data-line-break data-kind="text">` + htmlkit.Escape(rec.FetchString(span, "text")) + "<br></span>"
	}
	html := ""
	if span["type"] == "note_reference" {
		html = p.noteReferenceSpan(span)
	} else {
		html = p.baseSpan(span)
	}
	html = p.externalLinkSpan(html, span)
	if linked, ok := p.indexLinkSpan(span); ok {
		html = linked
	}
	html = p.citationSpan(html, span)
	if link, ok := span["external_link"]; ok {
		html = strings.Replace(html, "data-kind=", "data-external-link-id=\""+htmlkit.Escape(rec.AsString(link))+"\" data-kind=", 1)
	}
	if lang, tagged := span["lang"].(string); tagged {
		html = strings.Replace(html, "data-kind=", "lang=\""+htmlkit.Escape(lang)+"\" data-kind=", 1)
	}
	color, colored := span["color"].(string)
	html = p.textMarks(html, color)
	// Text-mode \underline and soul's \ul are underlined text (decoration), not a formula; soul's \st
	// is struck-out text.
	decoration := ""
	if line, decorated := textDecorationLines[toS(span["decoration"])]; decorated {
		name := toS(span["decoration"])
		decoration = ";text-decoration:" + line
		html = strings.Replace(html, "data-kind=", "data-decoration=\""+name+"\" data-kind=", 1)
	}
	if span["math"] == true {
		html = strings.Replace(html, "data-kind=", "data-math=\"true\" data-kind=", 1)
	}
	styleValue, styled := span["style"]
	if !styled {
		if colored || decoration != "" {
			css := strings.TrimPrefix(decoration, ";")
			if colored {
				css = "color:" + p.inkColor(color) + decoration
			}
			return strings.Replace(html, "data-kind=", "style=\""+htmlkit.Escape(css)+"\" data-kind=", 1)
		}
		return html
	}
	style := rec.AsMap(styleValue)
	font := fontmodel.ForStyle(p.document, style)
	css := stylemodel.CSS(style, p.sizeBase())
	if font != nil {
		css += ";font-family:" + htmlkit.FontFamily(p.document, style) + ";font-kerning:normal"
		// A linked file is one exact face; named fonts may need the browser's bold or italic.
		if truthy(font["file"]) {
			css += ";font-synthesis:none"
		}
	}
	if colored {
		css += ";color:" + p.inkColor(color)
	}
	css += decoration
	encoded, err := canonjson.Marshal(style)
	if err != nil {
		rec.Fail("%v", err)
	}
	return strings.Replace(html, "data-kind=", "data-text-style=\""+htmlkit.Escape(string(encoded))+"\" style=\""+htmlkit.Escape(css)+"\" data-kind=", 1)
}

// symbolSpan: a QED mark announces the end of the proof and shows what the author drew (decision 176): the glyphs
// in a browser font, the image named by its alt text (the span then has no aria-label: axe forbids one on a span, and
// the image names the mark), a drawn box as the open square.
func (p *Paper) symbolSpan(span rec.Map) string {
	if span["name"] != "qed" {
		return `<span data-kind="symbol" data-name="qed_open_square" role="img" aria-label="End of proof">□</span>`
	}
	if image, ok := span["image"].(rec.Map); ok {
		return `<span data-kind="symbol" data-name="qed" data-mark="image">` + p.imageSpan(image) + "</span>"
	}
	mark, shown := "drawn", "□"
	if text, ok := span["text"].(string); ok {
		mark, shown = "text", htmlkit.Escape(text)
	}
	return `<span data-kind="symbol" data-name="qed" data-mark="` + mark + `" role="img" aria-label="End of proof">` + shown + "</span>"
}

// baseSpan is HTML#render_span.
func (p *Paper) baseSpan(span rec.Map) string {
	switch rec.Fetch(span, "type") {
	case "text":
		return "<span data-kind=\"text\">" + htmlkit.Escape(rec.FetchString(span, "text")) + "</span>"
	case "symbol":
		return p.symbolSpan(span)
	case "reference":
		name := ""
		if named := p.referenceName(span, p.previous); named != "" {
			name = " aria-label=\"" + htmlkit.Escape(named) + "\""
		}
		return "<a data-kind=\"reference\" data-label=\"" + htmlkit.Escape(rec.FetchString(span, "label")) + "\"" + name + " href=\"#" +
			toS(rec.Fetch(span, "target")) + "\">" + htmlkit.Escape(rec.FetchString(span, "text")) + "</a>"
	case "math":
		// A formula wholly inside a \hyperref text is part of its link: the link holds the MathML.
		// Its name is the formula's characters, as an external link's around a formula.
		if target, linked := span["link_target"].(string); linked {
			formula := p.renderMath(rec.FetchString(span, "node"))
			return "<a data-reference-label=\"" + htmlkit.Escape(rec.FetchString(span, "link_label")) + "\" href=\"#" + htmlkit.Escape(target) +
				"\" aria-label=\"" + htmlkit.Escape(linkName(formula, "#"+target)) + "\" data-kind=\"math\">" + formula + "</a>"
		}
		return "<span data-kind=\"math\">" + p.renderMath(rec.FetchString(span, "node")) + "</span>"
	}
	rec.Fail("Unknown HTML span")
	return ""
}

// renderMath is HTML#render_math: the formula with its id and its rows' ids, projected for
// browsers.
func (p *Paper) renderMath(identifier string) string {
	node := p.node(identifier)
	math := mathexport.ParsePaper(rec.FetchString(node, "mathml"))
	math.SetAttribute("id", identifier)
	if alt := mathexport.AltText(math); alt != "" {
		math.SetAttribute("alttext", alt)
	}
	for _, child := range rec.FetchList(node, "children") {
		row := p.node(child)
		element := math.Path(rec.FetchString(row, "mathml_path"))
		if element == nil {
			crash("undefined method 'add_attribute' for nil")
		}
		element.SetAttribute("id", rec.AsString(child))
	}
	mathexport.FitToPage(math, p.base)
	return mathexport.ProjectPage(math, node, p.macros)
}

func (p *Paper) externalLinkSpan(html string, span rec.Map) string {
	if !p.described {
		p.descriptors, p.described = extlinks.Descriptors(p.document, false), true
	}
	if span["type"] == "text" {
		return p.textLinkSpan(span)
	}
	if span["type"] != "math" {
		return html
	}
	for _, link := range p.descriptors {
		if rec.Equal(link["math_node"], span["node"]) {
			href := rec.FetchString(link, "href")
			opening := "<a data-external-link=\"" + toS(rec.Fetch(link, "id")) + "\" href=\"" + htmlkit.Escape(href) +
				"\" rel=\"noopener noreferrer\" aria-label=\"" + htmlkit.Escape(linkName(html, href)) + "\" "
			html = strings.Replace(html, "<span ", opening, 1)
			if strings.HasSuffix(html, "</span>") {
				html = strings.TrimSuffix(html, "</span>") + "</a>"
			}
			return html
		}
	}
	return html
}

func (p *Paper) textLinkSpan(span rec.Map) string {
	selected := []rec.Map{}
	for _, link := range p.descriptors {
		if _, ok := link["block"]; ok && rec.Identity(extlinks.Resolve(p.document, link)) == rec.Identity(span) {
			selected = append(selected, link)
		}
	}
	sort.SliceStable(selected, func(a, b int) bool {
		return rubynum.Less(rec.Fetch(selected[a], "start"), rec.Fetch(selected[b], "start"))
	})
	text := []rune(rec.FetchString(span, "text"))
	cursor := 0
	var builder strings.Builder
	for _, link := range selected {
		start := int(rec.FetchInt(link, "start"))
		low := min(cursor, len(text))
		builder.WriteString(htmlkit.Escape(string(text[low:max(min(start, len(text)), low)])))
		linkText := rec.FetchString(link, "text")
		cursor = start + len([]rune(linkText))
		// A link needs a name (WCAG 2.4.4/4.1.2): text that is only spaces names nothing, and
		// the neighbouring anchor with the same href is the named one, so write the space as
		// plain text instead of a nameless anchor (decision 135; the browser check expects it).
		if extlinks.BlankText(linkText) {
			builder.WriteString(htmlkit.Escape(linkText))
			continue
		}
		builder.WriteString("<a data-external-link=\"" + toS(rec.Fetch(link, "id")) + "\" href=\"" + htmlkit.Escape(rec.FetchString(link, "href")) +
			"\" rel=\"noopener noreferrer\">" + htmlkit.Escape(linkText) + "</a>")
	}
	rest := ""
	if cursor <= len(text) {
		rest = string(text[cursor:])
	}
	opening := "<span data-kind=\"text\">"
	// A glyph with no character but a name (marvosym's Tumbler) shows U+FFFD and is read by name.
	if name, named := span["glyph_name"].(string); named {
		opening = "<span role=\"img\" aria-label=\"" + htmlkit.Escape(inline.GlyphLabel(name)) + "\" data-glyph-name=\"" +
			htmlkit.Escape(name) + "\" data-kind=\"text\">"
	}
	return opening + builder.String() + htmlkit.Escape(rest) + "</span>"
}

var citationFields = []string{"citation", "citation_link", "bibliography_entry"}

func (p *Paper) citationSpan(html string, span rec.Map) string {
	attributes := []string{}
	for _, key := range citationFields {
		if value, ok := span[key]; ok {
			attributes = append(attributes, "data-"+strings.ReplaceAll(key, "_", "-")+"=\""+toS(value)+"\"")
		}
	}
	if len(attributes) == 0 {
		return html
	}
	graph := rec.FetchMap(p.document, "citations")
	if citation, ok := span["citation"]; ok {
		call := findID(rec.FetchList(graph, "calls"), citation)
		for _, field := range []string{"keys", "targets"} {
			encoded, err := canonjson.Marshal(rec.Fetch(call, field))
			if err != nil {
				rec.Fail("%v", err)
			}
			attributes = append(attributes, "data-citation-"+field+"=\""+htmlkit.Escape(string(encoded))+"\"")
		}
	}
	if linkID, ok := span["citation_link"]; ok {
		link := findID(rec.FetchList(graph, "links"), linkID)
		entry := findID(rec.FetchList(graph, "entries"), rec.Fetch(link, "entry"))
		attributes = append(attributes, "href=\"#"+toS(rec.FetchList(entry, "blocks")[0])+"\"")
		if nameless(span) {
			attributes = append(attributes, "aria-label=\""+htmlkit.Escape(p.citationName(html, entry))+"\"")
		} else if named := p.labelName(span, link, entry); named != "" {
			attributes = append(attributes, "aria-label=\""+htmlkit.Escape(named)+"\"")
		}
		html = strings.Replace(html, "<span ", "<a ", 1)
		if strings.HasSuffix(html, "</span>") {
			html = strings.TrimSuffix(html, "</span>") + "</a>"
		}
	}
	return strings.Replace(html, "data-kind=", strings.Join(attributes, " ")+" data-kind=", 1)
}

// findID is rows.find { |row| row.fetch('id') == id }; a miss is Ruby's NoMethodError on nil.
func findID(rows []any, id any) rec.Map {
	for _, row := range rows {
		if item := rec.AsMap(row); rec.Equal(rec.Fetch(item, "id"), id) {
			return item
		}
	}
	crash("undefined method 'fetch' for nil")
	return nil
}

func (p *Paper) noteReferenceSpan(span rec.Map) string {
	text := htmlkit.Escape(rec.FetchString(span, "text"))
	return "<a id=\"" + toS(rec.Fetch(span, "callsite")) + "\" data-kind=\"note_reference\" href=\"#" + toS(rec.Fetch(span, "target")) +
		"\" aria-label=\"Footnote " + text + "\">" + text + "</a>"
}

// pixels is sp * 96.0 / (65_536 * 72.27) as format('%.6f').
func pixels(sp any) string {
	return fixed(rubynum.Mul(sp, 96.0).(float64) / (65_536 * texPoints()))
}

func texPoints() float64 { return 72.27 }

func (p *Paper) ruleSpan(span rec.Map) string {
	color := ""
	if value, ok := span["color"]; ok {
		color = "data-color=\"" + toS(value) + "\" "
	}
	if _, ok := span["margin_left_sp"]; ok {
		color += "data-margin-left-sp=\"" + toS(rec.Fetch(span, "margin_left_sp")) + "\" data-margin-right-sp=\"" +
			toS(rec.Fetch(span, "margin_right_sp")) + "\" "
	}
	hidden := ""
	if span["frame_hidden"] == true {
		hidden = "data-frame-hidden "
	}
	return "<span data-kind=\"rule\" " + hidden + "data-width-sp=\"" + toS(rec.Fetch(span, "width_sp")) + "\" data-height-sp=\"" +
		toS(rec.Fetch(span, "height_sp")) + "\" data-depth-sp=\"" + toS(rec.Fetch(span, "depth_sp")) + "\" " + color +
		"style=\"" + htmlkit.RuleCSS(span) + "\"></span>"
}

func (p *Paper) imageSpan(span rec.Map) string {
	uri := htmlkit.FigurePath(p.figures, p.document, span)
	width, height := pixels(rec.Fetch(span, "width_sp")), pixels(rec.Fetch(span, "height_sp"))
	depthSP, hasDepth := span["depth_sp"]
	if !hasDepth {
		depthSP = int64(0)
	}
	depth := pixels(rubynum.Mul(depthSP, int64(-1)))
	baseline, placement := "", ""
	if value, ok := span["placement"]; ok {
		baseline = ";vertical-align:" + depth + "px"
		encoded, err := canonjson.Marshal(value)
		if err != nil {
			rec.Fail("%v", err)
		}
		placement = "data-depth-sp=\"" + toS(rec.Fetch(span, "depth_sp")) + "\" data-placement=\"" + htmlkit.Escape(string(encoded)) + "\" "
	}
	picture, inline := htmlkit.InlinePicture(p.document, span)
	name := "alt=\""
	if inline {
		// A figure shown as inline SVG (decision 146) is an image named by its alternative.
		name = "role=\"img\" aria-label=\""
	}
	alternative := name + htmlkit.Escape(htmlkit.ImageAlt(span, htmlkit.CaptionLabel(p.caption, p.nodes))) + "\""
	long, described := htmlkit.LongDescription(span)
	if htmlkit.Undescribed(span) {
		alternative += " " + htmlkit.UndescribedAttribute + "=\"true\""
	}
	switch {
	case span["decorative"] == true:
		alternative = "alt=\"\" role=\"presentation\""
	case span["external_link"] != nil:
		// A linked image (the ORCID logo) is named by its link.
		alternative = "alt=\"\""
	case described:
		// A long description (decisions 096, 110) is a disclosure after the image's block: the image
		// names the disclosure with aria-details and its text with aria-describedby.
		alternative += " aria-describedby=\"" + htmlkit.DescriptionID(span) + "\" aria-details=\"" + htmlkit.DetailsID(span) + "\""
		p.longDescriptions = append(p.longDescriptions, htmlkit.LongDescriptionHTML(p.document, span, long))
	}
	identity := "id=\"" + toS(rec.Fetch(span, "id")) + "\" data-kind=\"image\" data-asset=\"" + toS(rec.Fetch(span, "asset")) +
		"\" data-width-sp=\"" + toS(rec.Fetch(span, "width_sp")) + "\" data-height-sp=\"" + toS(rec.Fetch(span, "height_sp")) + "\" " + placement
	style := "style=\"width:" + width + "px;height:" + height + "px" + baseline + "\" "
	if inline {
		// The file it shows stays in figs/ (decision 058: originals beside derived forms), named by data-src.
		return "<svg" + picture.PictureAttributes() + " " + identity + "data-src=\"" + uri + "\" " + style + alternative + ">" +
			picture.Content + "</svg>"
	}
	return "<img " + identity + "src=\"" + uri + "\" " + style + alternative + "/>"
}

// withLongDescriptions is a block's markup followed by the disclosures of the described images
// it holds (decision 110), outside the block: a paragraph's phrasing content cannot hold one.
func (p *Paper) withLongDescriptions(render func() string) string {
	outer := p.longDescriptions
	p.longDescriptions = nil
	markup := render() + strings.Join(p.longDescriptions, "")
	p.longDescriptions = outer
	return markup
}

// sizeBase is the size text runs are expressed against: the document's body size as a ratio
// for the reader's own text size, or 0 for TeX pixels inside the pixel-exact table projection.
func (p *Paper) sizeBase() int64 {
	if p.absolute {
		return 0
	}
	return p.base
}

// alignAttribute is the attribute of a paragraph's recorded alignment (decision 116), or nothing.
func alignAttribute(node rec.Map) string {
	if align, ok := node["align"].(string); ok {
		return " data-align=\"" + align + "\""
	}
	return ""
}
