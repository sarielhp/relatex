package bundle

import (
	"encoding/base64"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcemap"
	"github.com/sarielhp/relatex/internal/svgoutline"
)

// Layout contract constants (PDFLayout, PDFGlyphOutlines, PDFUnicodeGlyphs).
var layoutCoordinates = rec.Map{"unit": "bp", "origin": "top-left", "source": "MuPDF page coordinates"}

const (
	metricOptions = "preserve-ligatures,preserve-whitespace,accurate-bboxes=no,clip=no"
	inkOptions    = "preserve-ligatures,preserve-whitespace,accurate-bboxes=yes,accurate-side-bearings=no,clip=no"
	inkExtent     = "MuPDF outline height with advance width; horizontal outline bounds are not measured"
	outlineExtent = "Unclipped glyph fill outlines from SVG curves; excludes stroke expansion, clipping, hinting and compositing"
	maxComponents = 32
)

var layoutVersions = []any{"layout-1.0", "layout-1.1", "layout-1.2"}

// Validate is BundleValidation.validate!: the IR, its embedded assets and the cross-layer
// bindings of document, layout and source map.
func Validate(payload rec.Map) rec.Map {
	document := rec.FetchMap(payload, "document")
	paperdoc.Validate(document)
	images, _ := rec.FetchMap(document, "assets")["items"].(rec.Map)
	for _, sha := range sortedKeys(images) {
		image := rec.AsMap(images[sha])
		mediamodel.NewMediaAsset(image)
		if rec.Fetch(image, "sha256") != sha {
			rec.Fail("Bundle media source identity differs")
		}
	}
	fonts := []any{}
	if value, ok := document["fonts"].(rec.Map); ok {
		fonts, _ = value["items"].([]any)
	}
	layout, hasLayout := payload["layout"].(rec.Map)
	if hasLayout && layout != nil {
		ValidateLayout(layout)
		validateDocumentLayout(document, layout)
	}
	if m, ok := payload["source_map"].(rec.Map); ok && m != nil {
		if rec.Fetch(m, "schema_version") == sourcemap.Schema {
			sourcemap.Validate(m)
		} else {
			validateMap(m, rec.Fetch(payload, "layout"))
		}
		if binding, bound := m["document_sha256"]; bound && binding != nil {
			if digest, _ := canonjson.Digest(document); binding != digest {
				rec.Fail("Source synchronization belongs to another document model")
			}
		}
	}
	return rec.Map{"status": "verified", "scope": "IR, embedded assets and cross-layer bindings; no TeX/browser or source-semantic acceptance",
		"qualified": false, "nodes": int64(len(rec.FetchMap(document, "nodes"))), "images": int64(len(images)), "fonts": int64(len(fonts))}
}

func sortedKeys(m rec.Map) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func validateDocumentLayout(document, layout rec.Map) {
	identities := []any{}
	for _, key := range []string{"text_paint", "table_paint"} {
		if paint, ok := document[key].(rec.Map); ok && paint["pdf_sha256"] != nil {
			identities = append(identities, paint["pdf_sha256"])
		}
	}
	totals := []rec.Map{}
	nodes := rec.FetchMap(document, "nodes")
	for _, id := range sortedKeys(nodes) {
		if node := rec.AsMap(nodes[id]); node["type"] == "document_page_total" {
			totals = append(totals, node)
			identities = append(identities, rec.Fetch(node, "pdf_sha256"))
		}
	}
	valid := true
	for _, sha := range identities {
		valid = valid && rec.Equal(sha, rec.Fetch(layout, "pdf_sha256"))
	}
	for _, node := range totals {
		valid = valid && rec.Equal(rec.Fetch(node, "count"), int64(LayoutPageCount(layout)))
	}
	if !valid {
		rec.Fail("Document and layout belong to different PDF evidence")
	}
}

// finite is Numeric#finite?, failing as Ruby does on anything else.
func finite(value any) float64 {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("undefined method 'finite?' for %T", value)
	}
	if math.IsInf(number, 0) || math.IsNaN(number) {
		return math.NaN()
	}
	return number
}

func positiveFinite(value any) bool {
	number := finite(value)
	return !math.IsNaN(number) && number > 0
}

// ValidateLayout is BundleValidation.validate_layout!.
func ValidateLayout(layout rec.Map) {
	version := rec.Fetch(layout, "schema_version")
	if !slices.Contains(layoutVersions, version) {
		rec.Fail("Unsupported layout schema")
	}
	if !rec.Equal(rec.Fetch(layout, "coordinates"), layoutCoordinates) {
		rec.Fail("Unsupported layout coordinates")
	}
	for _, key := range []string{"pdf_sha256", "helper_sha256"} {
		sourcemap.ValidateDigest(rec.Fetch(layout, key))
	}
	if version != "layout-1.0" {
		validateMeasurement(layout)
	}
	if LayoutPageCount(layout) == 0 {
		rec.Fail("Layout physical page inventory differs")
	}
	// A stored layout is checked part by part (decision 087), each part as a whole layout is.
	EachLayoutPages(layout, func(first int, pages []any) { validatePages(pages, first, version == "layout-1.2") })
}

// validatePages checks pages numbered from first+1: their numbers first, then each page, side by
// side; the first failing page in order is the one raised.
func validatePages(pages []any, first int, outlines bool) {
	for index, page := range pages {
		if !rec.Equal(rec.Fetch(rec.AsMap(page), "number"), int64(first+index+1)) {
			rec.Fail("Layout physical page inventory differs")
		}
	}
	failures := make([]any, len(pages))
	var group sync.WaitGroup
	for index, page := range pages {
		group.Add(1)
		go func() {
			defer group.Done()
			defer func() { failures[index] = recover() }()
			validatePage(rec.AsMap(page), outlines)
		}()
	}
	group.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
}

func validateMeasurement(layout rec.Map) {
	measurement := rec.FetchMap(layout, "measurement")
	if measurement["metric_options"] != metricOptions || measurement["ink_options"] != inkOptions {
		rec.Fail("Unsupported glyph measurement options")
	}
	outlines := rec.Fetch(layout, "schema_version") == "layout-1.2"
	extent := inkExtent
	if outlines {
		extent = outlineExtent
	}
	if rec.Fetch(measurement, "ink_extent") != extent {
		rec.Fail("Unsupported ink extent")
	}
	if outlines && rec.Fetch(measurement, "outline_contract") != "svg-glyph-outlines-1" {
		rec.Fail("Unsupported glyph outline contract")
	}
	sourcemap.ValidateDigest(rec.Fetch(measurement, "paint_sha256"))
}

// decodeSVG is Base64.strict_decode64.
func decodeSVG(text string) []byte {
	svg, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil || strings.ContainsAny(text, "\r\n") {
		rec.Fail("invalid base64")
	}
	return svg
}

// PageSVG is a layout page's SVG, checked against its hash and validated.
func PageSVG(page rec.Map) []byte {
	svg := decodeSVG(rec.FetchString(page, "svg"))
	if hexSum(svg) != rec.Fetch(page, "svg_sha256") {
		rec.Fail("Layout SVG hash differs")
	}
	return mediamodel.ValidateSVG(svg)
}

func validatePage(page rec.Map, outlines bool) {
	if !positiveFinite(rec.Fetch(page, "width")) || !positiveFinite(rec.Fetch(page, "height")) {
		rec.Fail("Invalid layout dimensions")
	}
	svg := PageSVG(page)
	glyphs := rec.FetchList(page, "glyphs")
	seen := map[any]bool{}
	for _, value := range glyphs {
		id := rec.Fetch(rec.AsMap(value), "id")
		if seen[id] {
			rec.Fail("Duplicate layout glyphs")
		}
		seen[id] = true
	}
	inventory := map[any]rec.Map{}
	for _, value := range glyphs {
		glyph := rec.AsMap(value)
		validateGlyph(glyph)
		if _, ok := glyph["unicode_binding"]; ok {
			validateUnicodeGlyph(glyph)
		}
		inventory[rec.Fetch(glyph, "id")] = glyph
	}
	if outlines {
		svgoutline.Validate(svg, glyphs)
	}
	pageWords := rec.FetchList(page, "words")
	wordIDs := map[any]bool{}
	for _, value := range pageWords {
		id := rec.Fetch(rec.AsMap(value), "id")
		if wordIDs[id] {
			rec.Fail("Duplicate layout words")
		}
		wordIDs[id] = true
	}
	for _, value := range pageWords {
		word := rec.AsMap(value)
		if !rec.Equal(rec.Fetch(word, "page"), rec.Fetch(page, "number")) {
			rec.Fail("Word belongs to another page")
		}
		validateWord(word, inventory)
		if outlines {
			validateWordBounds(word, inventory)
		}
		for _, box := range rec.FetchList(word, "boxes") {
			if !validBox(box) {
				rec.Fail("Invalid word box")
			}
		}
	}
}

func validBox(value any) bool {
	box := rec.AsList(value)
	if len(box) != 4 {
		return false
	}
	numbers := make([]float64, 4)
	for index, item := range box {
		numbers[index] = finite(item)
		if math.IsNaN(numbers[index]) {
			return false
		}
	}
	return numbers[0] <= numbers[2] && numbers[1] <= numbers[3]
}

func validateGlyph(glyph rec.Map) {
	for _, field := range []struct {
		key   string
		count int
	}{{"metric_quad", 8}, {"ink_quad", 8}, {"baseline", 2}} {
		value := rec.FetchList(glyph, field.key)
		valid := len(value) == field.count
		for _, number := range value {
			parsed, ok := rec.Number(number)
			valid = valid && ok && !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
			if !valid {
				break
			}
		}
		if !valid {
			rec.Fail("Invalid native glyph geometry")
		}
	}
	if !positiveFinite(rec.Fetch(glyph, "size")) {
		rec.Fail("Invalid native glyph size")
	}
}

func without(m rec.Map, keys ...string) rec.Map {
	result := rec.Map{}
	for key, value := range m {
		if !slices.Contains(keys, key) {
			result[key] = value
		}
	}
	return result
}

func validateUnicodeGlyph(glyph rec.Map) {
	binding := rec.FetchMap(glyph, "unicode_binding")
	parts, ok := rec.Fetch(glyph, "unicode_components").([]any)
	if !ok || len(parts) < 1 || len(parts) > maxComponents {
		rec.Fail("Invalid PDF Unicode component coverage")
	}
	var joined strings.Builder
	for _, value := range parts {
		part := rec.AsMap(value)
		_, nestedBinding := part["unicode_binding"]
		_, nestedComponents := part["unicode_components"]
		if nestedBinding || nestedComponents {
			rec.Fail("Nested PDF Unicode binding")
		}
		validateGlyph(part)
	}
	geometry.Numbers(rec.FetchList(binding, "baseline"), 2)
	geometry.Numbers(rec.FetchList(binding, "advance"), 2)
	derived := []string{}
	if _, measured := glyph["outline_binding"]; measured {
		derived = []string{"outline_binding", "ink_quad"}
	}
	for _, value := range parts {
		joined.WriteString(rec.FetchString(rec.AsMap(value), "text"))
	}
	text := rec.FetchString(binding, "text")
	first := rec.AsMap(parts[0])
	if text != rec.Fetch(glyph, "text") || utf8.RuneCountInString(text) <= 1 || joined.String() != rec.Fetch(glyph, "text") ||
		!rec.Equal(without(glyph, append([]string{"text", "unicode_binding", "unicode_components"}, derived...)...),
			without(first, append([]string{"text"}, derived...)...)) {
		rec.Fail("PDF Unicode binding differs from components")
	}
}

func validateWord(word rec.Map, inventory map[any]rec.Map) {
	offset := int64(0)
	seen := map[any]bool{}
	var text strings.Builder
	for _, value := range rec.FetchList(word, "characters_to_glyphs") {
		row := rec.AsMap(value)
		id := rec.Fetch(row, "glyph")
		glyph, ok := inventory[id]
		if !ok || seen[id] {
			rec.Fail("Word glyph relationship is dangling or repeated")
		}
		seen[id] = true
		glyphText := rec.FetchString(glyph, "text")
		length := int64(utf8.RuneCountInString(norm.NFKC.String(glyphText)))
		if !rec.Equal(rec.Fetch(row, "characters"), []any{offset, offset + length}) {
			rec.Fail("Word character-to-glyph offsets differ")
		}
		offset += length
		text.WriteString(glyphText)
	}
	joined := text.String()
	if joined != "" && rec.Fetch(word, "text") == joined && rec.Fetch(word, "normalized_text") == norm.NFKC.String(joined) {
		return
	}
	rec.Fail("Word text differs from native glyphs")
}

// extreme is Array#min or #max on JSON numbers: the element itself, as Ruby returns it.
func extreme(values []any, less bool) any {
	result := values[0]
	best, _ := rec.Number(result)
	for _, value := range values[1:] {
		number, _ := rec.Number(value)
		if (less && number < best) || (!less && number > best) {
			result, best = value, number
		}
	}
	return result
}

func validateWordBounds(word rec.Map, inventory map[any]rec.Map) {
	xs, ys := []any{}, []any{}
	for _, value := range rec.FetchList(word, "characters_to_glyphs") {
		glyph, ok := inventory[rec.Fetch(rec.AsMap(value), "glyph")]
		if !ok {
			panic(rec.Failure{Message: "key not found", Lookup: true})
		}
		quad := rec.FetchList(glyph, "ink_quad")
		xs = append(xs, quad[0], quad[2], quad[4], quad[6])
		ys = append(ys, quad[1], quad[3], quad[5], quad[7])
	}
	if len(xs) == 0 {
		rec.Fail("Word bounds differ from glyph outlines")
	}
	expected := []any{[]any{extreme(xs, true), extreme(ys, true), extreme(xs, false), extreme(ys, false)}}
	if !rec.Equal(rec.Fetch(word, "boxes"), expected) || rec.Fetch(word, "box_kind") != "ink" {
		rec.Fail("Word bounds differ from glyph outlines")
	}
}
