package bundle

import (
	"encoding/base64"
	"maps"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// A page SVG defines, in its own <defs>, the outline of every glyph it uses, so a book repeats
// each outline on every page (Lebl's Basic Analysis: 42,201 definitions of 677 distinct
// outlines, 39 MB of its 88 MB of page SVG). The bundle stores each page SVG as text, not
// Base64, with every glyph outline definition line replaced by a reference to one shared list
// of those lines; reading restores the page's exact bytes (its svg_sha256 still holds).

const (
	sharedPathsKey = "svg_shared_paths"
	svgTextKey     = "svg_text"
	// sharedMark begins a reference line: NUL cannot occur in an XML document.
	sharedMark = "\x00"
)

// isOutlineDefinition is a glyph outline definition line, as MuPDF writes it.
func isOutlineDefinition(line string) bool {
	return strings.HasPrefix(line, `<path id="font_`) && strings.HasSuffix(line, `"/>`)
}

// shareLayoutSVG stores each page SVG as text with shared outline definitions; a page whose
// SVG would not come back byte for byte keeps its Base64 form.
func shareLayoutSVG(layout any) any {
	source, ok := layout.(rec.Map)
	if !ok {
		return layout
	}
	pages, ok := source["pages"].([]any)
	if !ok {
		return layout
	}
	shared := []any{}
	index := map[string]int{}
	result := maps.Clone(source)
	stored := make([]any, len(pages))
	for position, value := range pages {
		stored[position] = value
		page, isMap := value.(rec.Map)
		encoded, hasSVG := page["svg"].(string)
		if !isMap || !hasSVG {
			continue
		}
		svg, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || !utf8.Valid(svg) || strings.Contains(string(svg), sharedMark) {
			continue
		}
		lines := strings.Split(string(svg), "\n")
		for number, line := range lines {
			if !isOutlineDefinition(line) {
				continue
			}
			at, seen := index[line]
			if !seen {
				at = len(shared)
				index[line] = at
				shared = append(shared, line)
			}
			lines[number] = sharedMark + strconv.Itoa(at)
		}
		text := strings.Join(lines, "\n")
		if base64.StdEncoding.EncodeToString([]byte(expandSharedLines(text, shared))) != encoded {
			continue
		}
		copied := maps.Clone(page)
		delete(copied, "svg")
		copied[svgTextKey] = text
		stored[position] = copied
	}
	result["pages"] = stored
	if len(shared) > 0 {
		result[sharedPathsKey] = shared
	}
	return result
}

// unshareLayoutSVG restores the Base64 page SVGs shareLayoutSVG stored as text.
func unshareLayoutSVG(layout any) any {
	source, ok := layout.(rec.Map)
	if !ok {
		return layout
	}
	pages, hasPages := source["pages"].([]any)
	if !hasPages {
		return layout
	}
	shared, _ := source[sharedPathsKey].([]any)
	result := maps.Clone(source)
	delete(result, sharedPathsKey)
	restored := make([]any, len(pages))
	for position, value := range pages {
		restored[position] = value
		page, isMap := value.(rec.Map)
		text, stored := page[svgTextKey].(string)
		if !isMap || !stored {
			continue
		}
		copied := maps.Clone(page)
		delete(copied, svgTextKey)
		copied["svg"] = base64.StdEncoding.EncodeToString([]byte(expandSharedLines(text, shared)))
		restored[position] = copied
	}
	result["pages"] = restored
	return result
}

func expandSharedLines(text string, shared []any) string {
	lines := strings.Split(text, "\n")
	for number, line := range lines {
		reference, isReference := strings.CutPrefix(line, sharedMark)
		if !isReference {
			continue
		}
		at, err := strconv.Atoi(reference)
		if err != nil || at < 0 || at >= len(shared) {
			rec.Fail("Invalid shared SVG outline reference")
		}
		definition, isText := shared[at].(string)
		if !isText {
			rec.Fail("Invalid shared SVG outline reference")
		}
		lines[number] = definition
	}
	return strings.Join(lines, "\n")
}
