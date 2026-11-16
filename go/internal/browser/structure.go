package browser

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/rec"
)

// mediaVersion is MediaAsset::VERSION.
const mediaVersion = "media-1"

// texPoints is 72.27, a variable: Ruby computes 65_536 * 72.27 at run time.
var texPoints = 72.27

func pixels(value any) float64 {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("undefined method '*' for %T", value)
	}
	return number * 96.0 / (65_536 * texPoints)
}

// imageSpans is PaperBrowserCheck#image_spans: the images of the main reading order, rich
// cells in display order.
func imageSpans(document rec.Map) []rec.Map {
	result := []rec.Map{}
	nodes := rec.FetchMap(document, "nodes")
	for _, id := range rec.FetchList(document, "block_order") {
		for _, span := range inline.Flat(rec.FetchList(rec.Node(nodes, id), "content"), true) {
			// A symbol drawn as a picture (an author's QED mark, the duck; decision 176) shows its image as an img,
			// as PaperImages.occurrences counts it (books/0004: "Browser changed paper image count").
			if image, ok := rec.AsMap(span)["image"].(rec.Map); ok && rec.Fetch(span, "type") == "symbol" {
				span = image
			}
			if rec.Fetch(span, "type") == "image" {
				result = append(result, span)
			}
		}
	}
	return result
}

// withoutSources is PaperBrowserCheck#without_sources: the page's src carries a figure's name,
// so the LaTeX source that named it is dropped where the inventory the DOM reads back is
// compared (validateImages checks the src).
func withoutSources(value any) any {
	switch typed := value.(type) {
	case []any:
		projected := make([]any, 0, len(typed))
		for _, item := range typed {
			projected = append(projected, withoutSources(item))
		}
		return projected
	case rec.Map:
		projected := rec.Map{}
		for key, item := range typed {
			projected[key] = withoutSources(item)
		}
		if projected["type"] == "image" {
			// The page shows a figure's description (its alt) but not where the text came from.
			delete(projected, "source")
			delete(projected, "source_file")
			delete(projected, "source_line")
			delete(projected, "alt_provenance")
		}
		return projected
	default:
		return value
	}
}

// verifyImageFiles is PaperImages.verify_files!: every figure file holds the bytes the
// document shows or keeps. A name no longer carries its content, so the document is the
// authority.
func verifyImageFiles(document rec.Map, directory string) {
	displayed, originals := figures.Plan(document)
	for _, figures := range []struct {
		paths   map[string]string
		problem string
	}{{displayed, "Linked image file changed"}, {originals, "Figure source file changed"}} {
		digests := make([]string, 0, len(figures.paths))
		for digest := range figures.paths {
			digests = append(digests, digest)
		}
		sort.Strings(digests)
		for _, digest := range digests {
			relative := figures.paths[digest]
			if command.FileDigest(filepath.Join(directory, relative)) != digest {
				rec.Fail("%s: %s", figures.problem, relative)
			}
		}
	}
}

var imageAttributes = []string{"alt", "data-asset", "data-height-sp", "data-kind", "data-width-sp", "id", "src", "style"}

// validateImages is PaperBrowserCheck#validate_images!: the images' ids (an inline figure's with the
// ids its markup holds, decision 146) and how many show as img.
func validateImages(inventory, document rec.Map) ([]string, int) {
	spans := imageSpans(document)
	images := rec.FetchList(inventory, "images")
	if len(images) != len(spans) {
		rec.Fail("Browser changed paper image count")
	}
	ids, shown := []string{}, 0
	for index, span := range spans {
		image := rec.AsMap(images[index])
		asset, ok := rec.FetchMap(rec.FetchMap(document, "assets"), "items")[rec.FetchString(span, "asset")].(rec.Map)
		if !ok {
			panic(rec.Failure{Message: "key not found: " + rec.Inspect(span["asset"]), Lookup: true})
		}
		expected := htmlkit.ImageSource(document, span)
		if !rec.Equal([]any{image["id"], image["asset"], image["src"]}, []any{rec.Fetch(span, "id"), rec.Fetch(span, "asset"), expected}) ||
			!truthy(rec.Fetch(image, "visible")) {
			rec.Fail("Browser changed media bytes, decoding or occurrence")
		}
		_, inline := htmlkit.InlinePicture(document, span)
		if inline != truthy(image["inline"]) {
			rec.Fail("Browser changed how a figure is shown (inline SVG or img)")
		}
		attributes := slices.Clone(imageAttributes)
		if inline {
			ids = append(ids, validateInlinePicture(image, span, document)...)
			attributes = append(slices.Clone(inlineAttributes), rootAttributeNames(span, document)...)
		} else {
			if !strings.HasSuffix(rec.FetchString(image, "current"), "/"+expected) || !truthy(rec.Fetch(image, "complete")) {
				rec.Fail("Browser changed media bytes, decoding or occurrence")
			}
			validateImageDecoding(image, span, asset)
			shown++
		}
		if hasKey(span, "placement") {
			attributes = append(attributes, "data-depth-sp", "data-placement")
		}
		// A decorative image's role, a long description's reference (decision 096), and the
		// placeholder flag and caption reference of an undescribed image.
		if span["decorative"] == true {
			attributes = append(attributes, "role")
		}
		if htmlkit.Undescribed(span) && span["external_link"] == nil {
			attributes = append(attributes, htmlkit.UndescribedAttribute)
		}
		if _, described := htmlkit.LongDescription(span); described {
			attributes = append(attributes, "aria-describedby", "aria-details")
		}
		slices.Sort(attributes)
		attributes = slices.Compact(attributes)
		if !rec.Equal(rec.Fetch(image, "attributes"), strings2any(attributes)) {
			rec.Fail("Unexpected paper image attributes")
		}
		validateImageSize(image, span)
		ids = append(ids, rec.ToS(rec.Fetch(span, "id")))
	}
	return ids, shown
}

func strings2any(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func validateImageDecoding(image, span, asset rec.Map) {
	if !hasKey(span, "placement") {
		expected := asset
		if asset["schema_version"] == mediaVersion {
			expected = rec.FetchMap(asset, "metadata")
		}
		if !rec.Equal([]any{image["natural_width"], image["natural_height"]}, []any{expected["pixel_width"], expected["pixel_height"]}) {
			rec.Fail("Browser changed source raster decoding")
		}
		return
	}
	for _, dimension := range []string{"width", "height"} {
		expected := pixels(rec.Fetch(span, dimension+"_sp"))
		observed, _ := rec.Number(rec.Fetch(image, "natural_"+dimension))
		if !(observed > 0) || math.Abs(observed-expected) >= 1.0 {
			rec.Fail("Browser changed SVG intrinsic dimensions")
		}
	}
}

// validateImageSize: the style is the size printed once to six places, as the HTML writes it;
// rounding first and printing after differs on an exact tie (13114596 sp is 265.8203125 px,
// written 265.820312; fifth cohort 1262).
func validateImageSize(image, span rec.Map) {
	style := imageStyle(span)
	if rec.Fetch(image, "style") != style {
		rec.Fail("Browser changed image style")
	}
	width, height := rec.Round(pixels(rec.Fetch(span, "width_sp")), 6), rec.Round(pixels(rec.Fetch(span, "height_sp")), 6)
	for dimension, expected := range map[string]float64{"width": width, "height": height} {
		observed, _ := rec.Number(rec.Fetch(image, dimension))
		if math.Abs(observed-expected) >= 1.0/64 {
			rec.Fail("Browser changed image display geometry")
		}
	}
}

// imageStyle is the inline style the HTML writes for an image span.
func imageStyle(span rec.Map) string {
	style := fmt.Sprintf("width:%.6fpx;height:%.6fpx", pixels(rec.Fetch(span, "width_sp")), pixels(rec.Fetch(span, "height_sp")))
	if hasKey(span, "placement") {
		style += fmt.Sprintf(";vertical-align:%.6fpx", pixels(negated(rec.Fetch(span, "depth_sp"))))
	}
	return style
}

// negated is -value in the value's own type: -0 of an Integer is 0, not -0.0.
func negated(value any) any {
	if integer, ok := value.(int64); ok {
		return -integer
	}
	return -number(value)
}

var listTags = map[string]string{"ordered_list": "ol", "unordered_list": "ul", "list_item": "li"}

// listStyleType: a list prints its markers as text (none); an item whose marker a beamer theme
// draws shows its shape (decision 127).
func listStyleType(node rec.Map) string {
	if marker, ok := node["marker"].(rec.Map); ok && node["type"] == "list_item" {
		return rec.FetchString(marker, "shape")
	}
	return "none"
}

// validateLists is PaperBrowserCheck#validate_list_rendering!.
func validateLists(inventory rec.Map, structures []rec.Map, nodes rec.Map) {
	expected := []any{}
	for _, node := range structures {
		if tag, ok := listTags[rec.FetchString(node, "type")]; ok {
			expected = append(expected, rec.Merge(rec.Pick(node, "id", "type"), rec.Map{"tag": tag, "list_style": listStyleType(rec.Node(nodes, node["id"]))}))
		}
	}
	if !rec.Equal(rec.FetchList(inventory, "lists"), expected) {
		rec.Fail("Browser changed semantic list elements or markers")
	}
}

var floatTypes = []string{"figure_float", "table_float", "float_caption"}

// validateFloats is PaperBrowserCheck#validate_float_rendering!.
func validateFloats(inventory rec.Map, structures []rec.Map, nodes rec.Map) {
	expected := []any{}
	for _, node := range structures {
		kind := rec.FetchString(node, "type")
		if !slices.Contains(floatTypes, kind) {
			continue
		}
		var parent, role any
		if kind != "float_caption" {
			// A named table's float is a group; no figure carries a name (decision 183, amendment 1).
			if htmlkit.FigureGrouped(node, nodes) {
				role = "group"
			}
		}
		tag := "figure"
		if kind == "float_caption" {
			parent = floatParent(structures, rec.Fetch(node, "id"))
			tag = captionTag(structures, nodes, parent, rec.Fetch(node, "id"))
		}
		expected = append(expected, rec.Merge(rec.Pick(node, "id", "type"), rec.Map{"tag": tag, "caption": nil, "role": role, "float_parent": parent}))
	}
	if !rec.Equal(rec.FetchList(inventory, "floats"), expected) {
		rec.Fail("Browser changed semantic float/caption associations")
	}
}

// captionTag is the element a caption is written as (htmlkit.CaptionTags): figcaption, or div between
// its float's blocks; figcaption when the caption has no float.
func captionTag(structures []rec.Map, nodes rec.Map, parent, id any) string {
	for _, row := range structures {
		if kind := rec.FetchString(row, "type"); parent != nil && kind != "float_caption" && slices.Contains(floatTypes, kind) && rec.Equal(rec.Fetch(row, "id"), parent) {
			if tag, ok := htmlkit.CaptionTags(row, nodes)[fmt.Sprint(id)]; ok {
				return tag
			}
		}
	}
	return "figcaption"
}

func floatParent(structures []rec.Map, id any) any {
	for _, row := range structures {
		if slices.ContainsFunc(rec.FetchList(row, "children"), func(child any) bool { return rec.Equal(child, id) }) {
			return rec.Fetch(row, "id")
		}
	}
	return nil
}

// verifyNotes is the note part of PaperBrowserCheck#verify!: the expected notes.
func verifyNotes(inventory, document rec.Map) []rec.Map {
	notes := []rec.Map{}
	for _, id := range rec.FetchList(document, "note_order") {
		note := rec.Node(rec.FetchMap(document, "nodes"), id)
		backlinks := []any{}
		for _, call := range rec.FetchList(note, "callsites") {
			backlinks = append(backlinks, rec.Fetch(rec.AsMap(call), "id"))
		}
		notes = append(notes, rec.Merge(rec.Pick(note, "id", "number", "children", "labels"), rec.Map{"backlinks": backlinks}))
	}
	if !rec.Equal(rec.FetchList(inventory, "notes"), maps(notes)) {
		rec.Fail("Browser changed paper note links or labels")
	}
	return notes
}

// verifyPageTotals is the document page-total part of PaperBrowserCheck#verify!.
func verifyPageTotals(inventory, document rec.Map) {
	expected := []any{}
	for _, node := range nodeValues(document) {
		if node["type"] != "document_page_total" {
			continue
		}
		expected = append(expected, rec.Merge(rec.Pick(node, "id", "count", "source_label", "pdf_sha256", "anchor"),
			rec.Map{"text": "Source PDF: " + rec.ToS(rec.Fetch(node, "count")) + " pages.", "visible": true}))
	}
	if !rec.Equal(rec.FetchList(inventory, "page_totals"), expected) {
		rec.Fail("Browser changed document page metadata")
	}
}

// safeInventory is the inventory validate_rendering! reads: the document's own images, links
// and linked files set aside.
func safeInventory(inventory, document rec.Map, images int) rec.Map {
	fonts := []string{}
	if catalog, ok := document["fonts"].(rec.Map); ok {
		items, _ := catalog["items"].([]any)
		for _, value := range items {
			if file, ok := rec.AsMap(value)["file"].(rec.Map); ok {
				fonts = append(fonts, rec.ToS(file["name"]))
			}
		}
	}
	if mathexport.AccentFontUsed(document) {
		fonts = append(fonts, mathexport.AccentFile)
	}
	paths := []string{}
	for _, span := range imageSpans(document) {
		// A figure shown as inline SVG (decision 146) loads no file.
		if _, inline := htmlkit.InlinePicture(document, span); inline {
			continue
		}
		if source := htmlkit.ImageSource(document, span); !slices.Contains(paths, source) {
			paths = append(paths, source)
		}
	}
	resources := []any{}
	preview := previewScripts(inventory)
	for _, value := range rec.FetchList(inventory, "resources") {
		uri := rec.ToS(value)
		if uri != browserFavicon && !linkedFile(uri, paths, "/") && !linkedFile(uri, fonts, "/fonts/") && !linkedFile(uri, preview, "/") {
			resources = append(resources, value)
		}
	}
	unsafe, _ := rec.Number(rec.Fetch(inventory, "unsafeNodes"))
	broken, _ := rec.Number(rec.Fetch(inventory, "brokenLinks"))
	return rec.Merge(inventory, rec.Map{"unsafeNodes": int64(unsafe) - int64(images) - int64(len(preview)),
		"brokenLinks": int64(broken) - int64(len(extlinks.Anchored(document, extlinks.Descriptors(document, true)))), "resources": resources})
}

// linkedFile is linked_file? and linked_font?: a file the document links next to index.html,
// served at the page's origin.
func linkedFile(uri string, names []string, directory string) bool {
	if !strings.HasPrefix(uri, Origin) {
		return false
	}
	for _, name := range names {
		if strings.HasSuffix(uri, directory+name) {
			return true
		}
	}
	return false
}

// previewScripts: the page's own script, preview.js beside it (html.WritePreviewScript), as the
// scripts the inventory may discount; any other script stays unsafe markup.
func previewScripts(inventory rec.Map) []string {
	scripts, _ := inventory["scripts"].([]any)
	if len(scripts) == 1 && scripts[0] == "preview.js" {
		return []string{"preview.js"}
	}
	return nil
}
