package pdffonts

import (
	"maps"
	"slices"
	"strings"
)

// Prepare collects the names of the originals of every font refs reach, before any is fixed: a
// figure font whose own original is not found (a nameless or placeholder font) is then matched
// against the originals of the fonts its document names, and nothing wider. A placeholder name
// (CIDFont+F3) counts by the name its embedded program keeps. A collection embedded as a font's
// program is replaced by the one face its font names first, so every reader below reads it.
func (x *Fixer) Prepare(refs []string) {
	x.unwrapCollections(refs)
	x.addUnicodeCmaps(refs)
	names := map[string]bool{}
	for _, ref := range refs {
		dict, _ := x.store.Resolve(ref).(map[string]any)
		if dict["/Subtype"] == "/Type0" {
			descendants, _ := x.store.Resolve(dict["/DescendantFonts"]).([]any)
			if len(descendants) != 1 {
				continue
			}
			dict, _ = x.store.Resolve(descendants[0]).(map[string]any)
		}
		if name := x.originalNameOf(dict); name != "" {
			names[cMapless(name)] = true
		}
	}
	x.documentFonts = slices.Sorted(maps.Keys(names))
}

// originalNameOf is the name a font dictionary's original is looked up by; "" for a placeholder
// whose program keeps no name.
func (x *Fixer) originalNameOf(dict map[string]any) string {
	name := figureName(dict["/BaseFont"])
	if !strings.HasPrefix(cMapless(name), "CIDFont+") {
		return name
	}
	descriptor, _ := x.store.Resolve(dict["/FontDescriptor"]).(map[string]any)
	ref, ok := descriptor["/FontFile2"].(string)
	if !ok {
		return ""
	}
	data, err := x.store.StreamData(ref)
	if err != nil || len(data) < 12 {
		return ""
	}
	if own := originalName(name, tableDirectory(data)["name"]); own != name {
		return own
	}
	return ""
}

// documentMatch identifies a glyph whose own original is not found by the originals of the
// document's other fonts: an exact outline match only (no width, no ligature), and only when
// every matching font says the same character. ok is false when no font of the document matches.
func (x *Fixer) documentMatch(own string, shape glyphShape) (glyphResult, bool) {
	if shape.empty() {
		return glyphResult{}, false
	}
	var first glyphResult
	texts := map[string]bool{}
	for _, name := range x.documentFonts {
		entry := x.originalOf(name)
		if name == own || entry.font == nil {
			continue
		}
		if text, method, _ := entry.font.identify(shape); text != "" && method == "exact" {
			if len(texts) == 0 {
				first = glyphResult{text: text, method: method, from: familyOf(name), original: entry.font}
			}
			texts[text] = true
		}
	}
	switch len(texts) {
	case 0:
		return glyphResult{}, false
	case 1:
		return first, true
	}
	return glyphResult{why: "fonts of the document match it with different characters"}, true
}
