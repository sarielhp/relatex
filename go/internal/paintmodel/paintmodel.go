// Package paintmodel is the document model's paint proofs (decision 102): the shape of the
// native text paint proof document.json holds (PDFTextPaint.validate!), whether the document
// keeps PDF media, the text colours its spans carry, and the table paint proof
// (PDFTablePaint.validate!, PDFRichTablePaint with its font bindings, ToUnicode maps and text
// precision model). The core (pdfpaint) captures the proofs from the PDF with the same
// precision model; the model's readers check and read them here.
package paintmodel

import (
	"regexp"
	"slices"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// The text paint proof of PDFTextPaint: a bounded native paint profile. Unsupported ink is
// rejected, never recolored.
const (
	// TextVersion is the schema of a proof that also covers PDF media groups.
	TextVersion = "text-paint-2"
	// MediaGroupProfile is the paint profile of the PDF media groups.
	MediaGroupProfile = "bounded-normal-pdf-media-1"
	// Color is the colour the proof records for natively painted text.
	Color = "rgb(0, 0, 0)"
)

var (
	textFields = []string{"status", "pdf_sha256", "helper_sha256", "trace_sha256", "pages", "glyphs", "color"}
	digest     = regexp.MustCompile(`\A[0-9a-f]{64}\z`)
	// InkPattern is one #rrggbb fill.
	InkPattern = regexp.MustCompile(`\A#[0-9a-f]{6}\z`)
)

func truthy(value any) bool { return value != nil && value != false }

// PDFMedia is PDFTextPaint.pdf_media?: whether the document keeps a PDF original of a media
// asset.
func PDFMedia(document rec.Map) bool {
	assets, _ := document["assets"].(rec.Map)
	items, _ := assets["items"].(rec.Map)
	for _, value := range items {
		asset, _ := value.(rec.Map)
		representations, _ := asset["representations"].(rec.Map)
		original, _ := representations["original"].(rec.Map)
		if asset["schema_version"] == mediamodel.MediaVersion && original["mime_type"] == "application/pdf" {
			return true
		}
	}
	return false
}

// ValidateText is PDFTextPaint.validate!.
func ValidateText(document rec.Map) {
	if !truthy(document["text_paint"]) {
		return
	}
	proof, ok := document["text_paint"].(rec.Map)
	if !ok {
		rec.Fail("Invalid native text paint proof")
	}
	fields := textFields
	if proof["schema_version"] == TextVersion {
		if proof["group_profile"] != MediaGroupProfile || !PDFMedia(document) {
			rec.Fail("Text paint media profile requires retained PDF media")
		}
		fields = append(slices.Clone(fields), "schema_version", "group_profile")
	}
	inks, inked := proof["inks"]
	if inked {
		fields = append(slices.Clone(fields), "inks")
	}
	if canvas, painted := proof["canvas"]; painted {
		// the colour every page is filled with before its text (decision 107), never the paper's white
		text, isString := canvas.(string)
		if !isString || !InkPattern.MatchString(text) || text == "#ffffff" {
			rec.Fail("Invalid native text paint proof")
		}
		fields = append(slices.Clone(fields), "canvas")
	}
	if backdrops, painted := proof["backdrops"]; painted {
		if !validBackdrops(backdrops) {
			rec.Fail("Invalid native text paint proof")
		}
		fields = append(slices.Clone(fields), "backdrops")
	}
	valid := rec.SameKeys(proof, fields) && proof["status"] == "native_text_paint_verified" && proof["color"] == Color
	for _, key := range []string{"pages", "glyphs"} {
		valid = valid && rec.Positive(proof[key])
	}
	for _, key := range []string{"pdf_sha256", "helper_sha256", "trace_sha256"} {
		text, isString := proof[key].(string)
		valid = valid && isString && digest.MatchString(text)
	}
	if !valid || inked && !validInks(inks) {
		rec.Fail("Invalid native text paint proof")
	}
	for _, color := range SpanColors(document) {
		if !inked || !slices.Contains(rec.AsList(inks), any(color)) {
			rec.Fail("Text color is not painted natively: %s", color)
		}
	}
	validateBinding(document, proof)
}

// validBackdrops: the fills the PDF paints behind faint inks' text (decision 113), each an ink, a
// fill other than the paper's white and the ink's text there, sorted by ink and fill.
func validBackdrops(value any) bool {
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	previous := ""
	for _, item := range list {
		entry, isMap := item.(rec.Map)
		ink, _ := entry["ink"].(string)
		fill, _ := entry["fill"].(string)
		text, _ := entry["text"].(string)
		if !isMap || len(entry) != 3 || !InkPattern.MatchString(ink) || !InkPattern.MatchString(fill) || fill == "#ffffff" ||
			text == "" || ink+fill <= previous {
			return false
		}
		previous = ink + fill
	}
	return true
}

// validateBinding: the fonts, the table paint and the page totals belong to the proof's PDF.
func validateBinding(document, proof rec.Map) {
	for _, key := range []string{"fonts", "table_paint"} {
		if other := document[key]; truthy(other) {
			if sha := rec.Fetch(rec.AsMap(other), "pdf_sha256"); truthy(sha) && !rec.Equal(sha, proof["pdf_sha256"]) {
				rec.Fail("Text paint belongs to another PDF")
			}
		}
	}
	for _, node := range PageTotals(document) {
		if !rec.Equal(node["pdf_sha256"], proof["pdf_sha256"]) || !rec.Equal(node["count"], proof["pages"]) {
			rec.Fail("Text paint and physical page totals differ")
		}
	}
}

// PageTotals are the document's physical page total nodes.
func PageTotals(document rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, value := range rec.FetchMap(document, "nodes") {
		if node := rec.AsMap(value); node["type"] == "document_page_total" {
			result = append(result, node)
		}
	}
	return result
}

// validInks: a sorted list of distinct, non-black #rrggbb fills.
func validInks(value any) bool {
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	previous := ""
	for _, item := range list {
		text, isString := item.(string)
		if !isString || !InkPattern.MatchString(text) || text == "#000000" || text <= previous {
			return false
		}
		previous = text
	}
	return true
}

// SpanColors are the distinct fill colors the document's text spans carry, sorted.
func SpanColors(document rec.Map) []string {
	seen := map[string]bool{}
	for _, node := range rec.FetchMap(document, "nodes") {
		content, _ := rec.AsMap(node)["content"].([]any)
		for _, span := range inline.Deep(content) {
			if color, ok := span["color"].(string); ok && span["type"] != "rule" {
				seen[color] = true
			}
		}
	}
	list := make([]string, 0, len(seen))
	for color := range seen {
		list = append(list, color)
	}
	slices.Sort(list)
	return list
}
