// Package semantic is the Go port of SemanticExport with its two writers, MarkdownExport
// (with MarkdownAuthorMath) and TypstExport: the paper document as Markdown or Typst, each
// formula as the writer's math (or native MathML in Markdown), images materialized beside
// the text.
package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
)

// writer is what a format writes its own way.
type writer interface {
	header() string
	renderGroup(node rec.Map, children []string) string
	code(node rec.Map, text string) string
	block(node rec.Map, body string) string
	styledText(span rec.Map) string
	reference(span rec.Map) string
	internalLink(content string, id any) string
	externalLink(content string, href any) string
	noteReference(span rec.Map) string
	convert(xml string) string
	mathFormat() string
	nativeMath(node rec.Map, id string, failure string) (string, bool)
	math(value string, display bool, id string) string
	image(span rec.Map) string
	longDescription(text string) string
	fallbackImage(path, id string) string
	table(grid, span rec.Map) string
	symbol(span rec.Map) string
	rule(span rec.Map) string
	pageTotal(node rec.Map) string
	rowAnchor(node rec.Map) string
	mathSpan(span, owner rec.Map) string
}

// export is SemanticExport.
type export struct {
	document      rec.Map
	nodes         rec.Map
	directory     string
	allowFallback string
	borrowed      Borrowed
	// imageFallback renders formulas as images (allow_fallback image), created on first use.
	imageFallback func(id string) rec.Map
	losses        []any
	// representations are Ruby's hash in its insertion order, each entry in Ruby's key order.
	representations ojson.Object
	format          string
	w               writer
	// tableHTML renders a table grid span as the page does, created on first use.
	tableHTML func(span rec.Map) string
	// names is the figures of this output, in reading order (decision 068).
	names *figures.Names
}

// Borrowed are the renderings a semantic export takes from other outputs; the orchestration
// supplies them, since an output imports no other output (decision 102). Each is created on
// first use.
type Borrowed struct {
	// MathImages renders formulas as image files in the directory, for allow_fallback image:
	// the browser's fallback capture, one record per formula.
	MathImages func(document rec.Map, directory string) func(id string) rec.Map
	// TableHTML renders a table grid span as the page's HTML table, which the Markdown carries
	// as raw HTML.
	TableHTML func(document rec.Map) func(span rec.Map) string
}

// Render is MarkdownExport/TypstExport.new(document, directory:, allow_fallback:).render with
// the writer's losses and math representations (in Ruby's order).
func Render(format string, document rec.Map, directory, allowFallback string, borrowed Borrowed) (string, []any, ojson.Object) {
	paperdoc.Validate(document)
	if allowFallback != "" && allowFallback != "image" {
		rec.Fail("Unknown fallback mode")
	}
	e := &export{document: document, nodes: rec.FetchMap(document, "nodes"), directory: directory, allowFallback: allowFallback,
		losses: []any{}, representations: ojson.Object{}, format: format, borrowed: borrowed}
	switch format {
	case "markdown":
		e.w = &markdown{export: e}
	case "typst":
		e.w = &typst{export: e}
	default:
		rec.Fail("Unknown export format")
	}
	text := e.render()
	if format == "markdown" {
		text = closeInlineMath(text)
	}
	return text, e.losses, e.representations
}

func (e *export) node(id any) rec.Map { return rec.Node(e.nodes, id) }

func (e *export) render() string {
	main := e.renderNode(rec.FetchString(e.document, "root"), 0)
	notes := []string{}
	for _, id := range rec.FetchList(e.document, "note_order") {
		notes = append(notes, e.renderNode(rec.AsString(id), 0))
	}
	return e.w.header() + main + "\n\n" + strings.Join(notes, "\n\n") + "\n"
}

func (e *export) renderNode(id string, depth int) string {
	if depth > 64 {
		rec.Fail("Export structure depth limit exceeded")
	}
	node := e.node(id)
	if node["type"] == "document_page_total" {
		return e.w.pageTotal(node)
	}
	if _, ok := node["content"]; ok {
		return e.renderBlock(node)
	}
	switch node["type"] {
	case "math":
		return ""
	case "math_row":
		return e.w.rowAnchor(node)
	}
	if node["type"] == "code" {
		if text, ok := e.codeText(node); ok {
			return e.w.code(node, text)
		}
	}
	children := []string{}
	for _, child := range rec.FetchList(node, "children") {
		children = append(children, e.renderNode(rec.AsString(child), depth+1))
	}
	if node["type"] == "note" {
		labels := ""
		for _, row := range rec.FetchList(node, "labels") {
			labels += e.w.styledText(rec.Merge(rec.AsMap(row), rec.Map{"type": "text"}))
		}
		backlinks := []string{}
		for _, row := range rec.FetchList(node, "callsites") {
			backlinks = append(backlinks, e.w.internalLink("↩", rec.Fetch(rec.AsMap(row), "id")))
		}
		children = append(append([]string{labels}, children...), strings.Join(backlinks, " "))
	}
	return e.w.renderGroup(node, children)
}

func (e *export) renderBlock(node rec.Map) string {
	var body strings.Builder
	for _, span := range rec.FetchList(node, "content") {
		body.WriteString(e.renderSpan(rec.AsMap(span), node))
	}
	var rows strings.Builder
	for _, id := range rec.FetchList(node, "children") {
		if child := e.node(id); child["type"] == "math_row" {
			rows.WriteString(e.w.rowAnchor(child))
		}
	}
	return e.w.block(node, body.String()) + rows.String() + e.longDescriptions(node)
}

// longDescriptions are the long descriptions of the images node holds (decision 110), each as
// text of its own after the block.
func (e *export) longDescriptions(node rec.Map) string {
	var out strings.Builder
	for _, span := range rec.FetchList(node, "content") {
		long, ok := rec.AsMap(span)["long_description"].(string)
		if ok && strings.TrimSpace(long) != "" && rec.AsMap(span)["type"] == "image" {
			out.WriteString("\n\n" + e.w.longDescription(long))
		}
	}
	return out.String()
}

func (e *export) renderSpan(span, owner rec.Map) string {
	value := e.renderInline(span, owner)
	if _, linked := span["external_link"]; linked && (span["type"] == "math" || span["type"] == "image") {
		value = e.w.externalLink(value, paperdoc.ExternalHref(e.document, span))
	}
	// A formula set wholly inside a \hyperref text is part of that link.
	if target, linked := span["link_target"]; linked && span["type"] == "math" {
		value = e.w.internalLink(value, target)
	}
	link, ok := span["citation_link"]
	if !ok {
		return value
	}
	graph := rec.FetchMap(e.document, "citations")
	row := findID(rec.FetchList(graph, "links"), link)
	entry := findID(rec.FetchList(graph, "entries"), rec.Fetch(row, "entry"))
	return e.w.internalLink(value, rec.FetchList(entry, "blocks")[0])
}

func findID(rows []any, id any) rec.Map {
	for _, row := range rows {
		if item := rec.AsMap(row); rec.Equal(rec.Fetch(item, "id"), id) {
			return item
		}
	}
	panic(fmt.Errorf("undefined method 'fetch' for nil"))
}

func (e *export) renderInline(span, owner rec.Map) string {
	switch kind := rec.Fetch(span, "type"); kind {
	case "reference":
		return e.w.reference(span)
	case "note_reference":
		return e.w.noteReference(span)
	case "math":
		return e.w.mathSpan(span, owner)
	case "image":
		return e.w.image(span)
	case "table_grid":
		return e.w.table(rec.FetchMap(span, "grid"), span)
	case "symbol":
		return e.w.symbol(span)
	case "rule":
		return e.w.rule(span)
	case "text":
		return e.linkedText(span, owner)
	default:
		rec.Fail("Unsupported export inline node: %s", toS(kind))
	}
	return ""
}

func (e *export) linkedText(span, owner rec.Map) string {
	if _, linked := span["external_link"]; linked {
		return e.w.externalLink(e.w.styledText(span), paperdoc.ExternalHref(e.document, span))
	}
	position := -1
	for index, candidate := range rec.FetchList(owner, "content") {
		if item, ok := candidate.(rec.Map); ok && rec.Identity(item) == rec.Identity(span) {
			position = index
			break
		}
	}
	links := []rec.Map{}
	if front, ok := e.document["frontmatter"].(rec.Map); ok {
		listed, _ := front["links"].([]any)
		for _, value := range listed {
			row := rec.AsMap(value)
			if rec.Equal(row["block"], owner["id"]) && position >= 0 && rec.Equal(row["span"], int64(position)) {
				links = append(links, row)
			}
		}
	}
	if len(links) == 0 {
		return e.w.styledText(span)
	}
	sort.SliceStable(links, func(a, b int) bool { return rec.FetchInt(links[a], "start") < rec.FetchInt(links[b], "start") })
	text := []rune(rec.FetchString(span, "text"))
	cursor := 0
	var content strings.Builder
	for _, link := range links {
		start := int(rec.FetchInt(link, "start"))
		if start < cursor {
			rec.Fail("Overlapping external links")
		}
		content.WriteString(e.w.styledText(rec.Merge(span, rec.Map{"text": runeSlice(text, cursor, start)})))
		cursor = start + len([]rune(rec.FetchString(link, "text")))
		content.WriteString(e.w.externalLink(e.w.styledText(rec.Merge(span, rec.Map{"text": rec.Fetch(link, "text")})), rec.Fetch(link, "href")))
	}
	return content.String() + e.w.styledText(rec.Merge(span, rec.Map{"text": runeSlice(text, cursor, len(text))}))
}

func runeSlice(text []rune, from, to int) string {
	from, to = min(from, len(text)), min(to, len(text))
	if to < from {
		return ""
	}
	return string(text[from:to])
}

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// semanticMath is SemanticExport#math_span: the formula in the writer's math, else its
// native projection, else (with allow_fallback image) a browser rendering, else a Failure.
func (e *export) semanticMath(span, owner rec.Map) (result string) {
	id := rec.FetchString(span, "node")
	node := e.node(id)
	mathml := rec.FetchString(node, "mathml")
	value, failure := convertMath(e.w, mathml)
	if failure == "" {
		e.representations = e.representations.Set(id, ojson.Object{{"kind", "normalized_export_math"}, {"format", e.w.mathFormat()},
			{"source_mathml_sha256", digest(mathml)}})
		return e.w.math(value, owner["type"] == "display_math", id)
	}
	if native, ok := e.w.nativeMath(node, id, failure); ok {
		return native
	}
	if e.allowFallback != "image" {
		rec.Fail("%s: %s", id, failure)
	}
	if e.imageFallback == nil {
		e.imageFallback = e.borrowed.MathImages(e.document, e.directory)
	}
	fallback := e.imageFallback(id)
	e.losses = append(e.losses, rec.Map{"node": id, "original": "structured MathML", "replacement": fallback["path"],
		"reason": failure, "properties": []any{"editability", "structured_math"}, "evidence_asset": fallback})
	return e.w.fallbackImage(rec.FetchString(fallback, "path"), id)
}

// convertMath is the writer's conversion, or the message of the Failure it raises.
func convertMath(w writer, mathml string) (value, failure string) {
	defer rescueFailure(&failure)
	return w.convert(mathml), ""
}

func rescueFailure(failure *string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	caught, ok := recovered.(rec.Failure)
	if !ok || caught.Lookup {
		panic(recovered)
	}
	*failure = caught.Message
}

func (e *export) assetFile(span rec.Map) string {
	items := rec.FetchMap(rec.FetchMap(e.document, "assets"), "items")
	asset, ok := items[rec.FetchString(span, "asset")].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + quote(rec.FetchString(span, "asset")), Lookup: true})
	}
	placement, _ := span["placement"].(rec.Map)
	figure := mediamodel.NewMediaAsset(asset)
	item := figure.Representation(e.format, placement)
	extension, known := mediamodel.Extensions[rec.FetchString(item, "mime_type")]
	if !known {
		rec.Fail("key not found: %s", quote(rec.FetchString(item, "mime_type")))
	}
	names := e.figureNames()
	name := names.Claim(rec.FetchString(item, "sha256"), names.Stem(rec.FetchString(span, "asset"), span)+"."+extension)
	return figure.Materialize(e.directory, e.format, placement, name)
}

// figureNames is the figures of this output, in the document's reading order (decision 068).
func (e *export) figureNames() *figures.Names {
	if e.names == nil {
		e.names = figures.Stems(e.document)
	}
	return e.names
}

func (e *export) tableCellContent(cell rec.Map) string {
	owner := rec.Merge(cell, rec.Map{"type": "table_cell"})
	var builder strings.Builder
	for _, span := range rec.FetchList(cell, "content") {
		builder.WriteString(e.renderSpan(rec.AsMap(span), owner))
	}
	return builder.String()
}

// tableGridHTML is the page's HTML table of a table grid span.
func (e *export) tableGridHTML(span rec.Map) string {
	if e.tableHTML == nil {
		e.tableHTML = e.borrowed.TableHTML(e.document)
	}
	return e.tableHTML(span)
}

func toS(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	}
	return mathexport.RubyValue(value)
}

func quote(value string) string { return mathexport.Quote(value) }

var spaceCharacters = " \t\n\v\f\r"

func truthy(value any) bool { return value != nil && value != false }

func contains(list []string, value string) bool { return slices.Contains(list, value) }
