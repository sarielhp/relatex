package pdftag

import (
	"encoding/hex"
	"maps"
	"slices"
	"strings"
	"unicode/utf16"
)

// FigureFont is a font an included figure brought into the tagged PDF, with the figure's file and
// the program that made it. Collection says its embedded program is a TrueType collection, which
// PDF does not allow and veraPDF does not count as embedded.
type FigureFont struct {
	Source     string `json:"source"`
	Font       string `json:"font"`
	Creator    string `json:"creator,omitempty"`
	Collection bool   `json:"collection,omitempty"`
}

// formFont is one font of a form XObject.
type formFont struct {
	name       string
	collection bool
}

// figureFonts are the fonts of every form XObject drawn for an image of the model that names its
// source file, so the PDF/UA report can name the figure an author must re-export.
func (t *tagger) figureFonts() []FigureFont {
	found := []FigureFont{}
	seen := map[string]bool{}
	for _, page := range t.prepared {
		if page.failure != "" {
			continue
		}
		for _, index := range slices.Sorted(maps.Keys(page.objects)) {
			if index >= len(page.items) {
				continue
			}
			source := t.builder.sources[page.objects[index]]
			entry := page.items[index]
			if source == "" || entry.xobject != "Form" || entry.ref == "" {
				continue
			}
			creator, fonts := t.file.formFonts(entry.ref)
			for _, font := range fonts {
				id := source + "\x00" + font.name
				if seen[id] {
					continue
				}
				seen[id] = true
				found = append(found, FigureFont{Source: source, Font: font.name, Creator: creator, Collection: font.collection})
			}
		}
	}
	return found
}

// formFonts reads a form XObject's creator and the BaseFont names of its fonts, recursively
// through the nested forms it draws.
func (f *pdfFile) formFonts(ref string) (string, []formFont) {
	creator := ""
	if info := f.dict(f.dict(ref)["/PTEX.InfoDict"]); info != nil {
		creator = pdfString(info["/Creator"])
	}
	fonts := map[string]bool{}
	f.collectFormFonts(ref, nil, 0, map[string]bool{}, fonts)
	names := slices.Sorted(maps.Keys(fonts))
	found := make([]formFont, 0, len(names))
	for _, name := range names {
		found = append(found, formFont{name: name, collection: fonts[name]})
	}
	return creator, found
}

// collectFormFonts adds a form's fonts, and those of the nested forms in its resources.
func (f *pdfFile) collectFormFonts(ref string, inherited map[string]any, depth int, seen map[string]bool, fonts map[string]bool) {
	if depth > 8 || seen[ref] {
		return
	}
	seen[ref] = true
	dict := f.dict(ref)
	if dict == nil {
		return
	}
	resources := f.dict(dict["/Resources"])
	if resources == nil {
		resources = inherited
	}
	if resources == nil {
		return
	}
	byName := f.dict(resources["/Font"])
	for _, name := range slices.Sorted(maps.Keys(byName)) {
		font := f.dict(byName[name])
		if font == nil {
			continue
		}
		base := fontName(f, font)
		if base == "" {
			continue
		}
		fonts[base] = fonts[base] || fontCollection(f, font)
	}
	nested := f.dict(resources["/XObject"])
	for _, name := range slices.Sorted(maps.Keys(nested)) {
		value := nested[name]
		child, ok := value.(string)
		if !ok || !isRef(child) {
			continue
		}
		if f.dict(child)["/Subtype"] != "/Form" {
			continue
		}
		f.collectFormFonts(child, resources, depth+1, seen, fonts)
	}
}

// fontName is a font's BaseFont with its subset tag, from the font or its descendant.
func fontName(f *pdfFile, font map[string]any) string {
	name, _ := font["/BaseFont"].(string)
	if name == "" {
		for _, child := range f.array(font["/DescendantFonts"]) {
			descendant := f.dict(child)
			if descendant == nil {
				continue
			}
			if name, _ = descendant["/BaseFont"].(string); name != "" {
				break
			}
		}
	}
	return strings.TrimPrefix(name, "/")
}

// fontCollection reports whether a font's embedded program is a TrueType collection.
func fontCollection(f *pdfFile, font map[string]any) bool {
	descriptor := f.dict(font["/FontDescriptor"])
	if descriptor == nil {
		for _, child := range f.array(font["/DescendantFonts"]) {
			if descendant := f.dict(child); descendant != nil {
				if descriptor = f.dict(descendant["/FontDescriptor"]); descriptor != nil {
					break
				}
			}
		}
	}
	if descriptor == nil {
		return false
	}
	for _, key := range []string{"/FontFile2", "/FontFile3"} {
		ref, ok := descriptor[key].(string)
		if !ok || !isRef(ref) {
			continue
		}
		if data, err := f.streamData(ref); err == nil && len(data) >= 4 && string(data[:4]) == "ttcf" {
			return true
		}
	}
	return false
}

// pdfString is a qpdf JSON text string as text: its UTF-8, or its bytes as a PDF text string
// (UTF-16BE with a BOM, else PDFDocEncoding).
func pdfString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	switch {
	case strings.HasPrefix(text, "u:"):
		return text[2:]
	case strings.HasPrefix(text, "b:"):
		data, err := hex.DecodeString(text[2:])
		if err != nil {
			return ""
		}
		return pdfTextString(data)
	}
	return ""
}

// pdfTextString reads a PDF text string.
func pdfTextString(data []byte) string {
	if len(data) >= 2 && data[0] == 0xfe && data[1] == 0xff {
		units := make([]uint16, 0, (len(data)-2)/2)
		for index := 2; index+1 < len(data); index += 2 {
			units = append(units, uint16(data[index])<<8|uint16(data[index+1]))
		}
		return string(utf16.Decode(units))
	}
	runes := make([]rune, 0, len(data))
	for _, b := range data {
		if r, ok := pdfDocEncoding[b]; ok {
			runes = append(runes, r)
			continue
		}
		runes = append(runes, rune(b))
	}
	return string(runes)
}

// pdfDocEncoding are PDFDocEncoding's characters above Latin-1.
var pdfDocEncoding = map[byte]rune{
	0x18: '\u02d8', 0x19: '\u02c7', 0x1a: '\u02c6', 0x1b: '\u02d9', 0x1c: '\u02dd', 0x1d: '\u02db',
	0x1e: '\u02da', 0x1f: '\u02dc', 0x80: '\u2022', 0x81: '\u2020', 0x82: '\u2021', 0x83: '\u2026',
	0x84: '\u2014', 0x85: '\u2013', 0x86: '\u0192', 0x87: '\u2044', 0x88: '\u2039', 0x89: '\u203a',
	0x8a: '\u2212', 0x8b: '\u2030', 0x8c: '\u201e', 0x8d: '\u201c', 0x8e: '\u201d', 0x8f: '\u2018',
	0x90: '\u2019', 0x91: '\u201a', 0x92: '\u2122', 0x93: '\ufb01', 0x94: '\ufb02', 0x95: '\u0141',
	0x96: '\u0152', 0x97: '\u0160', 0x98: '\u0178', 0x99: '\u017d', 0x9a: '\u0131', 0x9b: '\u0142',
	0x9c: '\u0153', 0x9d: '\u0161', 0x9e: '\u017e', 0xa0: '\u20ac',
}
