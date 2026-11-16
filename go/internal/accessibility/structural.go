// Package accessibility is the accessibility report of an export (decision 080, plan
// accessibility §7.1): axe-core's verdict from the browser stage and relatex's own structural
// checks, computed from the document. Phase 0 measures: nothing here fails a conversion or
// changes a grade; the report claims conformance only when every check passes (Report).
package accessibility

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/sarielhp/relatex/internal/gaplocate"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// itemLimit is how many items a structural row lists; its count keeps them all.
const itemLimit = 50

// walk is the document read once for the structural checks: its blocks in reading order with
// their ancestors' types, and the spans of each, table cells included.
type walk struct {
	document rec.Map
	nodes    rec.Map
	parents  map[string]string
	images   []placedSpan
	tables   []rec.Map
	parts    []rec.Map
	// places locates items in the author's source (decision 184).
	places *gaplocate.Locator
}

// placedSpan is an image span with the types of the nodes that hold it, nearest first.
type placedSpan struct {
	span      rec.Map
	ancestors []string
}

func newWalk(document, sourceMap rec.Map) *walk {
	w := &walk{document: document, nodes: rec.FetchMap(document, "nodes"), parents: map[string]string{},
		places: gaplocate.New(document, sourceMap)}
	for _, id := range sortedKeys(w.nodes) {
		children, _ := rec.AsMap(w.nodes[id])["children"].([]any)
		for _, child := range children {
			if name, ok := child.(string); ok {
				w.parents[name] = id
			}
		}
	}
	for _, value := range rec.FetchList(document, "block_order") {
		id, _ := value.(string)
		node := rec.AsMap(w.nodes[id])
		content, _ := node["content"].([]any)
		w.spans(content, w.ancestors(id))
	}
	return w
}

// ancestors are the types of node id and the nodes above it, nearest first.
func (w *walk) ancestors(id string) []string {
	types := []string{}
	for seen := 0; id != "" && seen < 1000; seen++ {
		types = append(types, str(rec.AsMap(w.nodes[id])["type"]))
		id = w.parents[id]
	}
	return types
}

func (w *walk) spans(content []any, ancestors []string) {
	for _, value := range content {
		span := rec.AsMap(value)
		if _, ok := span["lang"]; ok {
			w.parts = append(w.parts, span)
		}
		if mark, ok := span["image"].(rec.Map); ok && span["type"] == "symbol" {
			span = mark // a QED mark's image (decision 176) is an image of the page
		}
		switch span["type"] {
		case "image":
			w.images = append(w.images, placedSpan{span: span, ancestors: ancestors})
		case "table_grid":
			w.tables = append(w.tables, span)
			// A rich grid holds its cells; a legacy grid's cells are text runs only.
			cells, _ := rec.AsMap(span["grid"])["cells"].([]any)
			for _, cell := range cells {
				cellContent, _ := rec.AsMap(cell)["content"].([]any)
				w.spans(cellContent, ancestors)
			}
		}
	}
}

// Structural is the report's structural list: one row {"criterion", "check", "status", "count",
// "items"} per check axe cannot make. htmlRoot is the HTML export, read for its <html lang> and
// its figure originals. A document the checks cannot read is returned as the error: phase 0 never
// fails a conversion over a measurement.
func Structural(document, sourceMap rec.Map, htmlRoot string) (rows []any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			rows, err = nil, fmt.Errorf("%v", recovered)
		}
	}()
	w := newWalk(document, sourceMap)
	// A page that cannot be read has no formulas or headings to measure: the language row says so.
	page, _ := os.ReadFile(filepath.Join(htmlRoot, "index.html"))
	return []any{
		languageRow(document, htmlRoot),
		row("3.1.2", "language_parts", partItems(w.parts)),
		imageRow(w.images),
		row("1.1.1", "image_drafts_reviewed", draftItems(w.images)),
		row("1.1.1", "formula_alternatives", formulaItems(page)),
		tableRow(w.tables, w.nodes, w.places),
		row("2.4.6", "heading_levels", headingItems(page)),
		row("1.4.5", "images_of_text", textImageItems(document, w.images, htmlRoot)),
		row("1.3.1", "landmarks", landmarkItems(document, page)),
		row("1.3.1", "named_groups", groupItems(page)),
		row("2.4.7", "focus_visible", styleItems("focus_visible", page)),
		row("2.3.3", "reduced_motion", styleItems("reduced_motion", page)),
		row("1.4.11", "forced_colors", styleItems("forced_colors", page)),
		row("1.4.11", "non_text_contrast", contrastItems(page)),
		colourRow(document, w),
	}, nil
}

func row(criterion, check string, items []any) rec.Map {
	status := "pass"
	if len(items) > 0 {
		status = "gap"
	}
	listed := items
	if len(listed) > itemLimit {
		listed = listed[:itemLimit]
	}
	return rec.Map{"criterion": criterion, "check": check, "status": status, "count": int64(len(items)), "items": listed}
}

// colourRow is the use_of_color row: the captions that name colours and nothing else says, as
// gaps, and the ones the author attested (decision 188) as attested, which are listed and never a
// gap. A row with attested captions and no gap has the status attested.
func colourRow(document rec.Map, w *walk) rec.Map {
	gaps, attested := colourItems(document, w)
	result := row("1.4.1", "use_of_color", gaps)
	if len(attested) > 0 {
		result["attested"] = attested
		if len(gaps) == 0 {
			result["status"] = "attested"
		}
	}
	return result
}

var htmlLang = regexp.MustCompile(`<html\b[^>]*\blang="([^"]*)"`)

// tableRow is the table_headers row with how the tables' headers are known (decision 111).
func tableRow(tables []rec.Map, nodes rec.Map, places *gaplocate.Locator) rec.Map {
	items, headers := tableItems(tables, nodes, places)
	result := row("1.3.1", "table_headers", items)
	if len(headers) > 0 {
		result["headers"] = headers
	}
	return result
}

// languageRow is the language_declared row with where the language came from (decision 111: a
// language detected with certainty counts, and the row says it was detected).
func languageRow(document rec.Map, htmlRoot string) rec.Map {
	result := row("3.1.1", "language_declared", languageItems(document, htmlRoot))
	if source := str(rec.AsMap(document["language"])["source"]); source != "" {
		result["source"] = source
	}
	return result
}

// languageItems: the source declares its language (babel, polyglossia or a class option), it is
// known, and the page's <html lang> is that language.
func languageItems(document rec.Map, htmlRoot string) []any {
	language, _ := document["language"].(rec.Map)
	tag, source := str(language["tag"]), str(language["source"])
	items := []any{}
	if source == "" || source == "default" {
		items = append(items, rec.Map{"problem": "undeclared", "tag": tag})
	}
	if tag == "und" {
		items = append(items, rec.Map{"problem": "unknown", "name": str(language["name"])})
	}
	page, err := os.ReadFile(filepath.Join(htmlRoot, "index.html"))
	declared := ""
	if match := htmlLang.FindSubmatch(page); err == nil && match != nil {
		declared = string(match[1])
	}
	if declared != tag {
		items = append(items, rec.Map{"problem": "page_language", "page": declared, "document": tag})
	}
	return items
}

// partItems: every span in another language names a known one.
func partItems(parts []rec.Map) []any {
	items := []any{}
	for _, span := range parts {
		if span["lang"] == "und" {
			items = append(items, rec.Map{"id": span["id"], "text": clip(str(span["text"]), 80)})
		}
	}
	return items
}

// imageItems: images without an author's alternative text. The page's alt is then a placeholder.
// The ORCID logo needs none: its link is named "ORCID iD <iD>" and the logo inside is decorative.
func imageItems(images []placedSpan) []any {
	items := []any{}
	for _, image := range images {
		if _, linked := image.span["external_link"]; linked {
			continue
		}
		if htmlkit.Undescribed(image.span) {
			items = append(items, rec.Map{"id": image.span["id"]})
		}
	}
	return items
}

// imageRow is the image_alternatives row with how many images are decorative (decision 096): an
// empty alternative the author asked for (artifact, alt={}) is no gap, nor a mark relatex inferred
// (decision 111), counted apart.
func imageRow(images []placedSpan) rec.Map {
	result := row("1.1.1", "image_alternatives", imageItems(images))
	decorative, inferred := int64(0), int64(0)
	for _, image := range images {
		if image.span["decorative"] == true {
			decorative++
			inferred += boolCount(image.span["alt_provenance"] == "inferred")
		}
	}
	result["decorative"] = decorative
	if inferred > 0 {
		result["inferred"] = inferred // decision 111: marks relatex inferred
	}
	return result
}

// draftItems: images whose alternative is a model's draft nobody has reviewed (decision 086). They
// have a text, so image_alternatives passes them, but no claim may rest on an unreviewed draft.
func draftItems(images []placedSpan) []any {
	items := []any{}
	for _, image := range images {
		if image.span["alt_provenance"] == "model" {
			items = append(items, rec.Map{"id": image.span["id"], "alt": clip(str(image.span["alt"]), 80)})
		}
	}
	return items
}

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func sortedKeys(m rec.Map) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func boolCount(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
