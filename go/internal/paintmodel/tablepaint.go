package paintmodel

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// The proof versions of PDFRichTablePaint (the current one, with the precision and font
// bindings, and the older ones a stored document may hold) and their schemas.
const (
	RichVersion     = "table-paint-4"
	precisionProof  = "table-paint-3"
	legacyRichProof = "table-paint-2"
)

var richContracts = map[string]string{RichVersion: "table-paint-v4.schema.json", precisionProof: "table-paint-v3.schema.json",
	legacyRichProof: "table-paint-v2.schema.json"}

func contract(name string) rec.Map {
	file, err := os.Open(filepath.Join(tablemodel.Root, "schemas", name))
	if err != nil {
		panic(fmt.Sprintf("pdfpaint: read contract: %v", err)) // Ruby reads it at load time.
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		panic(fmt.Sprintf("pdfpaint: parse contract: %v", err))
	}
	return rec.AsMap(value)
}

// DigestOf is the canonical JSON SHA-256 of a value (a grid's grid_sha256).
func DigestOf(value any) string {
	text, err := canonjson.Digest(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return text
}

// RichGrid is whether a table grid is a rich one (RichTable::VERSION).
func RichGrid(grid any) bool {
	m, ok := grid.(rec.Map)
	return ok && m["schema_version"] == tablemodel.Version
}

// ValidateTables is PDFTablePaint.validate!: a document's table paint proof against its grids.
func ValidateTables(document rec.Map) {
	if !truthy(document["table_paint"]) {
		return
	}
	proof := rec.AsMap(document["table_paint"])
	if version, _ := proof["schema_version"].(string); richContracts[version] != "" {
		validateRich(document)
		return
	}
	schema.Validate(document["table_paint"], contract("table-paint.schema.json"), "$")
	spans := gridSpans(document, true)
	ids := []string{}
	expected := []string{}
	for _, entry := range rec.FetchList(proof, "tables") {
		ids = append(ids, rec.FetchString(rec.AsMap(entry), "id"))
	}
	for _, span := range spans {
		expected = append(expected, rec.FetchString(span, "id"))
	}
	sorted := slices.Clone(ids)
	sort.Strings(sorted)
	sort.Strings(expected)
	if len(ids) == 0 || len(slices.Compact(slices.Clone(sorted))) != len(ids) || !slices.Equal(sorted, expected) {
		rec.Fail("Table paint coverage differs")
	}
	for _, item := range rec.FetchList(proof, "tables") {
		entry := rec.AsMap(item)
		span := findSpan(spans, entry["id"])
		if !rec.Equal(rec.Fetch(entry, "grid_sha256"), DigestOf(rec.Fetch(span, "grid"))) {
			rec.Fail("Table paint belongs to another grid")
		}
	}
	for _, node := range PageTotals(document) {
		valid := true
		for _, item := range rec.FetchList(proof, "tables") {
			valid = valid && !less(rec.Fetch(node, "count"), rec.Fetch(rec.AsMap(item), "page"))
		}
		if !rec.Equal(rec.Fetch(node, "pdf_sha256"), proof["pdf_sha256"]) || !valid {
			rec.Fail("Table paint PDF identity differs from page totals")
		}
	}
}

func less(left, right any) bool {
	a, okA := rec.Number(left)
	b, okB := rec.Number(right)
	if !okA || !okB {
		rec.Fail("comparison of %T with %T failed", right, left)
	}
	return a < b
}

func findSpan(spans []rec.Map, id any) rec.Map {
	for _, span := range spans {
		if rec.Equal(span["id"], id) {
			return span
		}
	}
	rec.Fail("undefined method 'fetch' for nil")
	return nil
}

// gridSpans: the table_grid spans of the blocks in block order (ordered), or of every node.
func gridSpans(document rec.Map, ordered bool) []rec.Map {
	nodes := rec.FetchMap(document, "nodes")
	ids := []string{}
	if ordered {
		for _, id := range rec.FetchList(document, "block_order") {
			ids = append(ids, rec.AsString(id))
		}
	} else {
		for id := range nodes {
			ids = append(ids, id)
		}
	}
	result := []rec.Map{}
	for _, id := range ids {
		node := rec.FetchMap(nodes, id)
		content, ok := node["content"]
		if !ok && !ordered {
			continue
		}
		if ordered {
			content = rec.Fetch(node, "content")
		}
		for _, item := range rec.AsList(content) {
			if span := rec.AsMap(item); span["type"] == "table_grid" {
				result = append(result, span)
			}
		}
	}
	return result
}

// validateRich is PDFRichTablePaint.validate!.
func validateRich(document rec.Map) {
	proof := rec.FetchMap(document, "table_paint")
	version := rec.Fetch(proof, "schema_version")
	name, known := richContracts[ToS(version)]
	if !known {
		rec.Fail("Unknown rich paint proof")
	}
	schema.Validate(proof, contract(name), "$")
	if version != legacyRichProof {
		validateMaps(rec.Fetch(proof, "font_encodings"))
	}
	grids := gridSpans(document, false)
	names := rec.Map{}
	if version == RichVersion {
		names = validateRichFonts(proof, grids)
	}
	ids := []string{}
	for _, item := range rec.FetchList(proof, "tables") {
		ids = append(ids, rec.FetchString(rec.AsMap(item), "id"))
	}
	expected := []string{}
	for _, span := range grids {
		expected = append(expected, rec.FetchString(span, "id"))
	}
	sorted := slices.Clone(ids)
	sort.Strings(sorted)
	sort.Strings(expected)
	if !slices.Equal(sorted, expected) || len(slices.Compact(slices.Clone(sorted))) != len(ids) {
		rec.Fail("Rich paint table inventory differs")
	}
	for _, item := range rec.FetchList(proof, "tables") {
		validateRichTable(proof, rec.AsMap(item), findSpan(grids, rec.AsMap(item)["id"]), names)
	}
	if version != legacyRichProof {
		validatePageBindings(proof)
	}
	for _, total := range PageTotals(document) {
		within := true
		for _, item := range rec.FetchList(proof, "tables") {
			for _, fragment := range rec.FetchList(rec.AsMap(item), "fragments") {
				within = within && !less(rec.Fetch(total, "count"), rec.Fetch(rec.AsMap(fragment), "page"))
			}
		}
		if !rec.Equal(rec.Fetch(total, "pdf_sha256"), proof["pdf_sha256"]) || !within {
			rec.Fail("Rich paint PDF binding differs")
		}
	}
}

func validateRichTable(proof, table, span rec.Map, names rec.Map) {
	if !rec.Equal(rec.Fetch(table, "grid_sha256"), DigestOf(rec.Fetch(span, "grid"))) {
		rec.Fail("Rich paint belongs to another grid")
	}
	fragments := rec.FetchList(table, "fragments")
	pages := []int64{}
	for _, fragment := range fragments {
		pages = append(pages, rec.FetchInt(rec.AsMap(fragment), "page"))
	}
	unique := slices.Compact(slices.Sorted(slices.Values(pages)))
	if len(pages) == 0 || !slices.Equal(pages, unique) {
		rec.Fail("Rich paint has empty or duplicate fragments")
	}
	if !RichGrid(span["grid"]) {
		return
	}
	expected := rec.FetchList(rec.FetchMap(span, "grid"), "native_fragments")
	expectedPages := []int64{}
	for _, fragment := range expected {
		expectedPages = append(expectedPages, rec.FetchInt(rec.AsMap(fragment), "page"))
	}
	if !slices.Equal(pages, expectedPages) {
		rec.Fail("Rich paint page coverage differs")
	}
	for index, item := range fragments {
		observed, native := rec.AsMap(item), rec.AsMap(expected[index])
		if !rec.Equal(observed["glyphs"], int64(len(rec.FetchList(native, "glyphs")))) ||
			!rec.Equal(observed["rules"], int64(len(rec.FetchList(native, "rules")))) {
			rec.Fail("Rich paint node coverage differs")
		}
		geometry.Numbers(rec.FetchList(observed, "origin_bp"), 2)
		if proof["schema_version"] != legacyRichProof {
			validateFragment(native, observed, rec.FetchMap(proof, "font_encodings"), names)
			continue
		}
		value, ok := rec.Number(rec.Fetch(observed, "maximum_error_bp"))
		if !ok || !(value >= 0 && value <= RichToleranceBP) {
			rec.Fail("Rich paint geometry exceeds tolerance")
		}
	}
}

func validateRichFonts(proof rec.Map, grids []rec.Map) rec.Map {
	glyphs := []rec.Map{}
	for _, span := range grids {
		fragments, _ := rec.FetchMap(span, "grid")["native_fragments"].([]any)
		for _, fragment := range fragments {
			for _, glyph := range rec.FetchList(rec.AsMap(fragment), "glyphs") {
				glyphs = append(glyphs, rec.AsMap(glyph))
			}
		}
	}
	fonts := rec.FetchMap(proof, "font_bindings")
	ValidateFonts(fonts, glyphs)
	if !rec.Equal(rec.Fetch(fonts, "pdf_sha256"), proof["pdf_sha256"]) || !rec.Equal(FontEncodings(fonts), proof["font_encodings"]) {
		rec.Fail("Rich paint font/PDF binding differs")
	}
	return FontNames(fonts)
}

func validatePageBindings(proof rec.Map) {
	pages := map[any][]rec.Map{}
	order := []any{}
	for _, item := range rec.FetchList(proof, "tables") {
		for _, value := range rec.FetchList(rec.AsMap(item), "fragments") {
			fragment := rec.AsMap(value)
			page := rec.Fetch(fragment, "page")
			if _, seen := pages[page]; !seen {
				order = append(order, page)
			}
			pages[page] = append(pages[page], fragment)
		}
	}
	for _, page := range order {
		traces := []any{}
		ids := []string{}
		for _, fragment := range pages[page] {
			if !slices.Contains(traces, rec.Fetch(fragment, "trace_sha256")) {
				traces = append(traces, fragment["trace_sha256"])
			}
		}
		if len(traces) != 1 {
			rec.Fail("Table paint traces differ on one page")
		}
		for _, fragment := range pages[page] {
			for _, key := range []string{"glyph_bindings", "rule_bindings"} {
				list, _ := fragment[key].([]any)
				for _, binding := range list {
					ids = append(ids, ToS(rec.Fetch(rec.FetchMap(rec.AsMap(binding), "pdf"), "id")))
				}
			}
		}
		if len(slices.Compact(slices.Sorted(slices.Values(ids)))) != len(ids) {
			rec.Fail("Tables borrow the same PDF paint")
		}
	}
}
