package pdffonts

import (
	"fmt"
	"maps"
	"slices"

	"github.com/sarielhp/relatex/internal/glyphnames"
)

// replacement is U+FFFD as a ToUnicode value: a glyph no table spells reads as "a character that
// cannot be represented" (decision 104), never as a wrong one.
const replacement = "FFFD"

// completeMap gives every code the pages show a usable ToUnicode value: one the map lacks, or
// whose value is U+0000, U+FEFF, U+FFFE or private use, is spelled from its glyph name by the
// tables, else U+FFFD. A font without a map gets one when a validator could not read a shown
// glyph through its name (a symbolic font, a name the Adobe Glyph List lacks).
func (x *Fixer) completeMap(f *font, use Use, names map[int]string) {
	mapped, width := map[int]string{}, 1
	toUnicode, hasMap := f.dict["/ToUnicode"].(string)
	if hasMap {
		data, err := x.store.StreamData(toUnicode)
		if err != nil {
			return
		}
		mapped, width = ReadCMap(string(data))
	}
	if width != 1 || (!hasMap && !x.needsMap(f, use, names)) {
		return
	}
	x.hintedTexts(f, mapped)
	added, replaced := 0, 0
	for _, code := range slices.Sorted(maps.Keys(use.Codes)) {
		value, ok := mapped[code]
		if f.drawing && value == replacement {
			continue
		}
		if !f.drawing && ((ok && !Unusable(value)) || (!hasMap && names[code] == "" && !x.unnamedTrueType(f))) {
			continue // readable, or left to the font: no map and no name to spell
		}
		mapped[code] = x.spell(f, code, names[code])
		if ok {
			replaced++
		} else {
			added++
		}
	}
	if added+replaced == 0 {
		return
	}
	f.dict["/ToUnicode"] = x.store.AddStream(map[string]any{}, []byte(WriteCMap(mapped, 1)))
	f.changed = true
	x.Report.Fonts++
	x.Report.Codes += added
	x.Report.Replaced += replaced
}

// hintedTexts gives the identification of an unnamed embedded TrueType font the usable values its own
// ToUnicode map has: they confirm an ambiguous outline match, never identify a glyph alone.
func (x *Fixer) hintedTexts(f *font, mapped map[int]string) {
	if len(mapped) == 0 || !x.unnamedTrueType(f) {
		return
	}
	if texts := x.programTexts(f); texts != nil {
		for code, value := range mapped {
			texts.hintCode(code, value)
		}
	}
}

// fixComposite replaces the unusable values of a composite (Type 0) font's ToUnicode map
// (U+0000, U+FEFF, U+FFFE, private use) by U+FFFD, and maps the two-byte codes the pages show that
// the map lacks to U+FFFD (a Mathematica or Word font's map without its first glyph's code, which
// veraPDF then cannot read, 8.4.5.8): its glyphs have no names to spell. A composite font with no
// map at all (an included figure's Mac Symbol or Calibri, Identity-H) gets one the same way.
func (x *Fixer) fixComposite(ref string, dict map[string]any, use Use) bool {
	mapped, width := map[int]string{}, 2
	if toUnicode, ok := dict["/ToUnicode"].(string); ok {
		data, err := x.store.StreamData(toUnicode)
		if err != nil {
			return false
		}
		mapped, width = ReadCMap(string(data))
	} else if dict["/Encoding"] != "/Identity-H" {
		return false
	}
	replaced, added := 0, 0
	texts := x.compositeTexts(dict)
	if texts != nil {
		for code, value := range mapped {
			texts.hintCID(code, value)
		}
	}
	defer x.finishIdentity(dict["/BaseFont"], texts)
	for code, value := range mapped {
		if Unusable(value) {
			mapped[code] = cidValue(texts, code)
			replaced++
		}
	}
	if width == 2 {
		for code := range use.Codes {
			if _, found := mapped[code]; !found {
				mapped[code] = cidValue(texts, code)
				added++
			}
		}
	}
	if replaced+added == 0 {
		return false
	}
	changed := maps.Clone(dict)
	changed["/ToUnicode"] = x.store.AddStream(map[string]any{}, []byte(WriteCMap(mapped, width)))
	x.store.Set(ref, changed)
	x.Report.Fonts++
	x.Report.Replaced += replaced
	x.Report.Codes += added
	return true
}

// identityGlyphMap names /Identity as the CIDToGIDMap of a composite font's embedded TrueType
// CIDFont without one (an included figure's: Word's and matplotlib's Times New Roman, papers 0041,
// 0052). PDF 1.x reads a missing map as Identity; PDF 2.0, and so PDF/UA-2 (8.4.5.3.2), wants it
// said: the glyphs drawn stay the same. It reports whether it named one.
func (x *Fixer) identityGlyphMap(dict map[string]any) bool {
	descendants, _ := x.store.Resolve(dict["/DescendantFonts"]).([]any)
	if len(descendants) != 1 {
		return false
	}
	ref, isRef := descendants[0].(string)
	font, _ := x.store.Resolve(ref).(map[string]any)
	if !isRef || font == nil || font["/Subtype"] != "/CIDFontType2" || font["/CIDToGIDMap"] != nil {
		return false
	}
	descriptor, _ := x.store.Resolve(font["/FontDescriptor"]).(map[string]any)
	if descriptor["/FontFile2"] == nil {
		return false
	}
	changed := maps.Clone(font)
	changed["/CIDToGIDMap"] = "/Identity"
	x.store.Set(ref, changed)
	x.Report.GlyphMaps++
	return true
}

// needsMap reports whether a font without a ToUnicode map needs one for the codes shown: a code
// whose name the Adobe Glyph List lacks, or a symbolic font whose names are known (its Type 1
// program's, a Type 3 font's), which a validator does not read by name.
func (x *Fixer) needsMap(f *font, use Use, names map[int]string) bool {
	known := f.program != nil || f.builtin != nil || f.dict["/Subtype"] == "/Type3"
	if f.drawing || x.unnamedTrueType(f) {
		return len(use.Codes) > 0
	}
	for code := range use.Codes {
		if names[code] != "" && ((f.symbolic && known) || !x.table.Readable(names[code])) {
			return true
		}
	}
	return false
}

// spell is a code's ToUnicode value from its glyph name, U+FFFD for a name no table spells, a
// code without a name in a font whose map lacks it, or a drawing font's code (a piece of a
// drawing, whatever its name: linew10's a1 is no Zapf Dingbat).
func (x *Fixer) spell(f *font, code int, name string) string {
	if f.drawing {
		x.Report.Drawn = append(x.Report.Drawn, fmt.Sprintf("%s %v: %d (%s)", f.ref, f.dict["/BaseFont"], code, name))
		return replacement
	}
	if name == "" {
		return x.unnamedValue(f, code)
	}
	if spelled := x.table.SpellIn(baseName(f.dict["/BaseFont"]), name); spelled != "" {
		return glyphnames.UTF16Hex(spelled)
	}
	if name != ".notdef" {
		x.Report.Unspelled = append(x.Report.Unspelled, fmt.Sprintf("%s %v: %d (%s)", f.ref, f.dict["/BaseFont"], code, name))
	}
	return replacement
}

// markDrawn records a drawing font shown.
func (x *Fixer) markDrawn(ref string) {
	if x.Drawn == nil {
		x.Drawn = map[string]bool{}
	}
	x.Drawn[ref] = true
}

// baseName is a base font's name without its subset tag (ABCDEF+).
func baseName(value any) string {
	name, _ := value.(string)
	name = name[min(1, len(name)):]
	if len(name) > 7 && name[6] == '+' {
		return name[7:]
	}
	return name
}

// unnamedTrueType reports whether f is an embedded TrueType font that names no glyph: no Encoding,
// so its codes reach glyphs through the program's own cmap (an included figure's symbolic
// Ghostscript or Mac font). No table can spell it, and veraPDF reads no value for its codes
// without a map (8.4.5.8), so each shown code reads U+FFFD: a character that cannot be represented.
func (x *Fixer) unnamedTrueType(f *font) bool {
	if f.dict["/Subtype"] != "/TrueType" || f.dict["/Encoding"] != nil || f.program != nil {
		return false
	}
	descriptor, _ := x.store.Resolve(f.dict["/FontDescriptor"]).(map[string]any)
	return descriptor["/FontFile2"] != nil
}
