package browser

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/frontmatter"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stylemodel"
	"github.com/sarielhp/relatex/internal/tablelayout"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// The page inventories: PaperTextStyleBrowser, PaperCitationBrowser and
// PaperFrontmatterBrowser's INVENTORY.
const (
	textStylesScript = `[...document.querySelectorAll('main [data-text-style]')].map(e => {
  const s = getComputedStyle(e);
  return {style: JSON.parse(e.dataset.textStyle), fontWeight: s.fontWeight,
          fontStyle: s.fontStyle, fontVariantCaps: s.fontVariantCaps,
          fontSize: parseFloat(s.fontSize), fontFamily: s.fontFamily,color:s.color,
          root: parseFloat(getComputedStyle(document.documentElement).fontSize)};
})`
	citationsScript = `[...document.querySelectorAll('main [data-block]')].flatMap(block => [...block.children].flatMap(c => c.classList.contains('runin-heading') ? [...c.children] : [c]).flatMap((e,index) => {
  const identities = {};
  for (const field of ['citation','citation_link','bibliography_entry']) {
    const value = e.getAttribute('data-' + field.replaceAll('_','-'));
    if (value !== null) identities[field] = Number(value);
  }
  if (!Object.keys(identities).length) return [];
  return [{block:block.id,index,identities,tag:e.localName,href:e.getAttribute('href'),
           keys:e.dataset.citationKeys ? JSON.parse(e.dataset.citationKeys) : null,
           targets:e.dataset.citationTargets ? JSON.parse(e.dataset.citationTargets) : null}];
}))`
	frontmatterScript = `(() => {
  /* A formula's TeX annotation (decisions 100, 141) is not shown: the link's text leaves it out. */
  const shownText = e => { const c = e.cloneNode(true); c.querySelectorAll("annotation, annotation-xml").forEach(a => a.remove()); return c.textContent; };
  const visible = e => e.getBoundingClientRect().width > 0 && e.getBoundingClientRect().height > 0 &&
    getComputedStyle(e).visibility === 'visible';
  const textStart = e => {
    const range = document.createRange();
    range.setStart(e.parentElement,0); range.setEndBefore(e);
    return [...range.toString()].length;
  };
  const front = document.querySelector('meta[name="author"]');
  return {
    metadata: front ? {title:document.title,authors:JSON.parse(front.content),
      ...(document.querySelector('meta[name="description"]') ?
        {abstract:document.querySelector('meta[name="description"]').content} : {})} : null,
    decorations:[...document.querySelectorAll('main [data-page-decoration]')].map(e => ({
      id:e.id,page:Number(e.dataset.sourcePage),source:e.dataset.decorationSource,
      candidate:Number(e.dataset.candidate),glyphs:Number(e.dataset.glyphs),text:e.textContent,visible:visible(e),
      ...(e.children.length ? {runs:[...e.children].map(c => ({text:c.textContent,...(c.dataset.math ? {math:true} : {style:JSON.parse(c.dataset.textStyle)})}))} : {})})),
    links:[...document.querySelectorAll('a')].filter(e => !e.getAttribute('href').startsWith('#')).map(e => ({
      id:e.dataset.externalLink || null,href:e.getAttribute('href'),text:shownText(e).replace(/[\u2061-\u2064]/g, ''),
      ...(e.querySelector('math') ? {math_node:e.querySelector('math').id} : {
        block:e.closest('[data-block]')?.id || null,
        ...(e.closest('[data-rich-cell]') ? {cell:e.closest('[data-rich-cell]').dataset.richCell} : {}),
        span:[...(e.closest('[data-cell-content]') || e.closest('[data-block]'))?.children || []].flatMap(c => c.classList.contains('runin-heading') ? [...c.children] : [c]).indexOf(e.parentElement),
        start:textStart(e)}),rel:e.getAttribute('rel'),visible:visible(e)}))
  };
})()`
)

// textPaintColor is PDFTextPaint::COLOR.
const textPaintColor = "rgb(0, 0, 0)"

// rubySpaceOrBOM is [\p{Space}\uFEFF].
func rubySpaceOrBOM(character rune) bool { return unicode.IsSpace(character) || character == 0xFEFF }

// styled is an expected text run style; table marks the pixel-exact table projection, whose
// runs keep their TeX pixel size while the others are relative to the reader text size.
type styled struct {
	style rec.Map
	table bool
}

// expectedStyles is PaperTextStyleBrowser.expected_styles.
func expectedStyles(document rec.Map) []styled {
	nodes := rec.FetchMap(document, "nodes")
	result := []styled{}
	for _, id := range rec.FetchList(document, "block_order") {
		for _, span := range fontmodel.NoteLabelSpans(document, id) {
			result = append(result, styled{style: rec.FetchMap(span, "style")})
		}
		for _, value := range rec.FetchList(rec.Node(nodes, id), "content") {
			result = append(result, spanStyles(rec.AsMap(value))...)
		}
	}
	decorations := frontmatter.Decorations(document)
	for _, row := range decorations {
		runs, _ := rec.AsMap(row)["runs"].([]any)
		for _, run := range runs {
			if math, _ := rec.AsMap(run)["math"].(bool); !math {
				result = append(result, styled{style: rec.FetchMap(rec.AsMap(run), "style")})
			}
		}
	}
	return result
}

func spanStyles(span rec.Map) []styled {
	if style, ok := span["style"]; ok {
		return []styled{{style: rec.AsMap(style)}}
	}
	if span["type"] != "table_grid" {
		return nil
	}
	grid := rec.FetchMap(span, "grid")
	result := []styled{}
	if grid["schema_version"] == tablemodel.Version {
		classic := tablelayout.ClassicProjection(grid) != nil
		for _, inner := range inline.Flat([]any{span}, true) {
			if style, ok := inner["style"]; ok {
				result = append(result, styled{style: rec.AsMap(style), table: classic})
			}
		}
		return result
	}
	for _, row := range rec.FetchList(grid, "rows") {
		for _, cell := range rec.AsList(row) {
			runs, _ := rec.AsMap(cell)["runs"].([]any)
			for _, run := range runs {
				if style, ok := rec.AsMap(run)["style"]; ok {
					result = append(result, styled{style: rec.AsMap(style), table: true})
				}
			}
		}
	}
	return result
}

// verifyTextStyles is PaperTextStyleBrowser.verify!.
func verifyTextStyles(inventory, document rec.Map) {
	expected := expectedStyles(document)
	actual, _ := inventory["text_styles"].([]any)
	if len(expected) != len(actual) {
		rec.Fail("Browser changed text style count")
	}
	base := stylemodel.BaseSize(document)
	for index, want := range expected {
		observed := rec.AsMap(actual[index])
		if !rec.Equal(rec.Fetch(observed, "style"), want.style) {
			rec.Fail("Browser changed observed text font identity")
		}
		properties := stylemodel.Properties(want.style)
		if !want.table {
			// Reading text is sized against the body size, at the browser root text size.
			root, _ := rec.Number(rec.Fetch(observed, "root"))
			properties["fontSize"] = float64(rec.FetchInt(want.style, "size_sp")) / float64(base) * root
		}
		if fontmodel.ForStyle(document, want.style) != nil {
			properties["fontFamily"] = htmlkit.FontFamily(document, want.style)
		}
		verifyStyleProperties(observed, properties, want.style)
	}
}

var styleKeys = []string{"fontWeight", "fontStyle", "fontVariantCaps"}

func verifyStyleProperties(observed, properties, style rec.Map) {
	for _, key := range styleKeys {
		if rec.Equal(rec.Fetch(observed, key), properties[key]) {
			continue
		}
		rec.Fail("Browser changed visible text styling: %s for %s (%s)", inspectKeys(observed), inspectKeys(properties),
			rec.Inspect(rec.Pick(style, "family", "series", "shape")))
	}
	size, _ := rec.Number(rec.Fetch(observed, "fontSize"))
	expected, _ := rec.Number(rec.Fetch(properties, "fontSize"))
	if math.Abs(size-expected) >= 1.0/64 {
		rec.Fail("Browser changed text font size")
	}
	if properties["fontFamily"] != "unknown" && !rec.Equal(rec.Fetch(observed, "fontFamily"), properties["fontFamily"]) {
		rec.Fail("Browser changed text font family: %s for %s", rec.ToS(observed["fontFamily"]), rec.ToS(properties["fontFamily"]))
	}
}

// inspectKeys is keys.zip(values).to_h.inspect for the style keys.
func inspectKeys(values rec.Map) string {
	parts := []string{}
	for _, key := range styleKeys {
		parts = append(parts, rec.InspectString(key)+"=>"+rec.Inspect(values[key]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// verifyPlatformFonts is SourceFonts.verify_platform!: a linked font must be the face the
// browser used; named fonts may resolve to any face.
func verifyPlatformFonts(inventory, document rec.Map) {
	fonts, _ := document["fonts"].(rec.Map)
	items, _ := fonts["items"].([]any)
	linked := false
	for _, font := range items {
		linked = linked || truthy(rec.AsMap(font)["file"])
	}
	if !linked {
		return
	}
	expected := fontmodel.PresentationSpans(document)
	actual := rec.FetchList(inventory, "platform_fonts")
	if len(actual) != len(expected) {
		rec.Fail("Browser source-font coverage changed")
	}
	for index, span := range expected {
		font := fontmodel.ForStyle(document, rec.FetchMap(span, "style"))
		if font == nil || !truthy(font["file"]) {
			continue
		}
		verifyFaces(rec.AsList(actual[index]), span, font)
	}
}

func verifyFaces(faces []any, span, font rec.Map) {
	text := rec.FetchString(span, "text")
	if len(faces) == 0 && rec.Strip(text) != "" {
		rec.Fail("Linked font did not render")
	}
	used := []string{}
	all := true
	for _, value := range faces {
		face := rec.AsMap(value)
		all = all && truthy(rec.Fetch(face, "isCustomFont")) && rec.Equal(rec.Fetch(face, "postScriptName"), font["postscript_name"])
		used = append(used, rec.ToS(face["postScriptName"])+":"+rec.ToS(face["glyphCount"]))
	}
	if !all {
		rec.Fail("Browser substituted a linked font %s in %s: %s", rec.ToS(rec.Fetch(font, "postscript_name")),
			rec.InspectString(text), strings.Join(used, " "))
	}
}

// verifyTextPaint is PDFTextPaint.verify_browser!.
func verifyTextPaint(inventory, document rec.Map) {
	if !truthy(document["text_paint"]) {
		return
	}
	observed := rec.FetchList(inventory, "text_paint")
	allowed := map[string]bool{textPaintColor: true}
	for _, hex := range paintmodel.SpanColors(document) {
		allowed[cssRGB(hex)] = true
	}
	all := len(observed) > 0
	for _, value := range observed {
		row := rec.AsMap(value)
		all = all && allowed[rec.ToS(row["color"])] && rec.Equal(row["opacity"], "1")
	}
	if !all {
		rec.Fail("Browser changed native text ink")
	}
}

// citationFields is PaperCitations::FIELDS.
var citationFields = []string{"citation", "citation_link", "bibliography_entry"}

// verifyCitations is PaperCitationBrowser.verify!.
func verifyCitations(inventory, document rec.Map) {
	catalogs := map[string]map[any]rec.Map{}
	graph, _ := document["citations"].(rec.Map)
	for _, field := range []string{"calls", "links", "entries"} {
		catalogs[field] = map[any]rec.Map{}
		if graph == nil {
			continue
		}
		for _, row := range rec.FetchList(graph, field) {
			catalogs[field][rec.Fetch(rec.AsMap(row), "id")] = rec.AsMap(row)
		}
	}
	external := map[any]rec.Map{}
	frontmatter, _ := document["frontmatter"].(rec.Map)
	links, _ := frontmatter["links"].([]any)
	for _, value := range links {
		if row := rec.AsMap(value); hasKey(row, "math_node") {
			external[rec.Fetch(row, "math_node")] = row
		}
	}
	expected := []any{}
	for _, id := range rec.FetchList(document, "block_order") {
		for index, value := range rec.FetchList(rec.Node(rec.FetchMap(document, "nodes"), id), "content") {
			span := rec.AsMap(value)
			link := external[span["node"]]
			// A linked text span holds its links as nested anchors; only math becomes the link itself.
			if hasKey(span, extlinks.Field) && span["type"] != "text" {
				link = rec.Map{"href": paperdoc.ExternalHref(document, span)}
			}
			if row := expectedCitation(span, id, int64(index), catalogs, link); row != nil {
				expected = append(expected, row)
			}
		}
	}
	if !rec.Equal(rec.FetchList(inventory, "citations"), expected) {
		rec.Fail("Browser changed citation clusters or bibliography links")
	}
}

func hasKey(m rec.Map, key string) bool {
	_, ok := m[key]
	return ok
}

func expectedCitation(span rec.Map, block any, index int64, catalogs map[string]map[any]rec.Map, external rec.Map) rec.Map {
	fields := rec.Pick(span, citationFields...)
	if len(fields) == 0 {
		return nil
	}
	var call, link rec.Map
	if truthy(span["citation"]) {
		call = fetchCatalog(catalogs["calls"], span["citation"])
	}
	if truthy(span["citation_link"]) {
		link = fetchCatalog(catalogs["links"], span["citation_link"])
	}
	if link != nil && external != nil {
		rec.Fail("External URL overlaps an internal citation link")
	}
	var href any
	if link != nil {
		entry := fetchCatalog(catalogs["entries"], rec.Fetch(link, "entry"))
		href = "#" + rec.ToS(rec.FetchList(entry, "blocks")[0])
	}
	if external != nil {
		href = rec.Fetch(external, "href")
	}
	if span["type"] == "reference" {
		href = "#" + rec.ToS(rec.Fetch(span, "target"))
	}
	tag := "span"
	if href != nil {
		tag = "a"
	}
	var keys, targets any
	if call != nil {
		keys, targets = rec.Fetch(call, "keys"), rec.Fetch(call, "targets")
	}
	return rec.Map{"block": block, "index": index, "identities": fields, "tag": tag, "href": href, "keys": keys, "targets": targets}
}

func fetchCatalog(catalog map[any]rec.Map, id any) rec.Map {
	row, ok := catalog[id]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rec.Inspect(id), Lookup: true})
	}
	return row
}

// verifyFrontmatter is PaperFrontmatterBrowser.verify!: the decorations' ids.
func verifyFrontmatter(inventory, document rec.Map) []string {
	observed := rec.FetchMap(inventory, "frontmatter")
	front, _ := document["frontmatter"].(rec.Map)
	var metadata any
	if front != nil {
		metadata = rec.Pick(front, "title", "authors", "abstract")
	}
	if !rec.Equal(rec.Fetch(observed, "metadata"), metadata) {
		rec.Fail("Browser changed structured front matter")
	}
	decorations := frontmatter.Decorations(document)
	expected := []any{}
	ids := []string{}
	for _, row := range decorations {
		expected = append(expected, rec.Merge(rec.AsMap(row), rec.Map{"visible": true}))
		ids = append(ids, rec.ToS(rec.Fetch(rec.AsMap(row), "id")))
	}
	if !rec.Equal(rec.Fetch(observed, "decorations"), expected) {
		rec.Fail("Browser changed retained page decorations")
	}
	descriptors := extlinks.Descriptors(document, true)
	for _, row := range descriptors {
		extlinks.SafeHref(rec.Fetch(row, "href"))
	}
	// A blank text link is plain text on the page (decision 135): no anchor may show it.
	links := []any{}
	for _, row := range extlinks.Anchored(document, descriptors) {
		links = append(links, rec.Merge(row, rec.Map{"rel": "noopener noreferrer", "visible": true}))
	}
	// Each link names its block and position; the listing order (amsart prints addresses at
	// the end) carries nothing.
	if seen, wanted := sortByID(rec.FetchList(observed, "links")), sortByID(links); !rec.Equal(seen, wanted) {
		rec.Fail("Browser changed external links or admitted unsafe targets: %s", firstDifference(seen, wanted))
	}
	return ids
}

// firstDifference names the first link the page shows otherwise than the document lists it.
func firstDifference(seen, wanted []any) string {
	for index := range max(len(seen), len(wanted)) {
		var shown, listed any
		if index < len(seen) {
			shown = seen[index]
		}
		if index < len(wanted) {
			listed = wanted[index]
		}
		if !rec.Equal(shown, listed) {
			return fmt.Sprintf("the page shows %s where the document lists %s", rec.Inspect(shown), rec.Inspect(listed))
		}
	}
	return "no single link differs"
}

// sortByID is sort_by { |row| row['id'] } (stable here; links have distinct ids).
func sortByID(rows []any) []any {
	sorted := append([]any{}, rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return rec.ToS(rec.AsMap(sorted[i])["id"]) < rec.ToS(rec.AsMap(sorted[j])["id"])
	})
	return sorted
}

// cssRGB is how the browser reports a #rrggbb color it computed.
func cssRGB(hex string) string {
	var red, green, blue int
	_, _ = fmt.Sscanf(hex, "#%02x%02x%02x", &red, &green, &blue)
	return fmt.Sprintf("rgb(%d, %d, %d)", red, green, blue)
}
