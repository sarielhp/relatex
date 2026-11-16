package pdffonts

import (
	"encoding/binary"

	"github.com/sarielhp/relatex/internal/glyphnames"
)

// glyphTexts tells which characters the glyphs of one embedded TrueType program stand for, from
// what the program itself says and nothing else: a Unicode cmap read backwards (glyph -> code), or
// the glyph's name in its post table through the Adobe Glyph List. A glyph neither source reaches
// has no text (no guess from glyph order or from a symbol cmap's codes, whose meaning only the
// font's own encoding knows).
type glyphTexts struct {
	program  *TrueType
	unicode  map[int]int // glyph -> Unicode, from the cmap
	spell    func(name string) string
	symbol   map[int]string // Adobe Symbol's names by code, for a font named Symbol, else nil
	glyphMap []byte         // a composite font's CIDToGIDMap stream, nil for Identity
	cff      *cffOutlines   // a CFF CID program's glyphs, nil for a TrueType one
	identity *identity      // the installed original that names glyphs the program does not
}

// newGlyphTexts reads the program at fontFile (a FontFile2 stream); nil when it cannot be read.
func (x *Fixer) newGlyphTexts(fontFile any, base string) *glyphTexts {
	ref, ok := fontFile.(string)
	if !ok {
		return nil
	}
	data, err := x.store.StreamData(ref)
	if err != nil {
		return nil
	}
	program, err := ParseTrueType(data)
	if err != nil {
		return nil
	}
	texts := &glyphTexts{program: program, unicode: program.UnicodeOf(),
		spell: func(name string) string { return x.table.SpellIn(base, name) }}
	if outlines, err := parseOutlines(data); err == nil {
		texts.identity = x.newIdentity(originalName(base, outlines.tables["name"]), outlines.shape)
	}
	if base == "Symbol" {
		if standard := x.programs.load("Symbol"); standard != nil {
			texts.symbol = standard.Encoding
		}
	}
	return texts
}

// ofGlyph is the ToUnicode value of a glyph ("" when the program does not tell): its Unicode
// cmap code, else its post name spelled by the tables, else (a font named Symbol, whose cmap
// maps Symbol codes to glyphs) the Adobe Symbol name of the code that reaches it.
func (g *glyphTexts) ofGlyph(gid int) string {
	if code, ok := g.unicode[gid]; ok && !Unusable(glyphnames.UTF16Hex(string(rune(code)))) {
		return glyphnames.UTF16Hex(string(rune(code)))
	}
	if gid >= 0 && gid < len(g.program.Names) {
		if name := g.program.Names[gid]; name != "" && name != ".notdef" {
			if spelled := g.spell(name); spelled != "" {
				return glyphnames.UTF16Hex(spelled)
			}
		}
	}
	if text := g.symbolText(gid); text != "" {
		return text
	}
	return g.identity.glyph(gid)
}

// symbolCode is the smallest Symbol code that reaches a glyph: through the program's (3,0) cmap
// (U+F000 offset removed) or, without one, its (1,0) cmap; -1 when none does.
func symbolCode(program *TrueType, gid int) int {
	best := -1
	for _, key := range [][2]int{{3, 0}, {1, 0}} {
		for code, glyph := range program.Cmaps[key] {
			if glyph == gid && (best < 0 || code&0xFF < best) {
				best = code & 0xFF
			}
		}
		if best >= 0 {
			break
		}
	}
	return best
}

// symbolText spells a glyph of a font named Symbol through the Adobe Symbol encoding: the smallest
// code of its (3,0) cmap (U+F000 offset removed) or, without one, of its (1,0) cmap that reaches it.
func (g *glyphTexts) symbolText(gid int) string {
	if g.symbol == nil {
		return ""
	}
	best := symbolCode(g.program, gid)
	if name := g.symbol[best]; best >= 0 && name != "" {
		if spelled := g.spell(name); spelled != "" {
			return glyphnames.UTF16Hex(spelled)
		}
	}
	return ""
}

// ofCID is the value of a composite font's CID, which reaches its glyph through the CIDToGIDMap.
func (g *glyphTexts) ofCID(cid int) string {
	if g.cff != nil {
		if gid, ok := g.cff.cidGlyph[cid]; ok {
			return g.ofGlyph(gid)
		}
		return ""
	}
	if g.glyphMap == nil {
		return g.ofGlyph(cid)
	}
	if 2*cid+2 > len(g.glyphMap) {
		return ""
	}
	return g.ofGlyph(int(binary.BigEndian.Uint16(g.glyphMap[2*cid:])))
}

// hintCID tells the identification what the PDF's own ToUnicode says of a composite font's CID.
func (g *glyphTexts) hintCID(cid int, value string) {
	if gid, ok := g.cidGlyphOf(cid); ok {
		g.identity.hint(gid, value)
	}
}

// cidGlyphOf is the glyph a CID reaches.
func (g *glyphTexts) cidGlyphOf(cid int) (int, bool) {
	switch {
	case g.cff != nil:
		gid, ok := g.cff.cidGlyph[cid]
		return gid, ok
	case g.glyphMap == nil:
		return cid, true
	case 2*cid+2 > len(g.glyphMap):
		return 0, false
	}
	return int(binary.BigEndian.Uint16(g.glyphMap[2*cid:])), true
}

// hintCode tells the identification what the PDF's own ToUnicode says of a simple font's code.
func (g *glyphTexts) hintCode(code int, value string) {
	if gid, ok := g.codeGlyph(code); ok {
		g.identity.hint(gid, value)
	}
}

// ofCode is the value of a simple font's code with no Encoding: it reaches its glyph through the
// program's symbolic cmap as the PDF specification reads it (9.6.5.4): the (3,0) subtable at
// the code or offset by U+F000, U+F100, U+F200, else the (1,0) subtable at the code.
func (g *glyphTexts) ofCode(code int) string {
	if gid, ok := g.codeGlyph(code); ok {
		return g.ofGlyph(gid)
	}
	return ""
}

// codeGlyph is the glyph a code of a font with no Encoding reaches (ofCode).
func (g *glyphTexts) codeGlyph(code int) (int, bool) {
	if table := g.program.Cmaps[[2]int{3, 0}]; table != nil {
		for _, offset := range []int{0, 0xF000, 0xF100, 0xF200} {
			if gid, ok := table[offset+code]; ok {
				return gid, true
			}
		}
		return 0, false
	}
	gid, ok := g.program.Cmaps[[2]int{1, 0}][code]
	return gid, ok
}

// cidValue is a composite font's CID value from the program, U+FFFD without one.
func cidValue(texts *glyphTexts, cid int) string {
	if texts != nil {
		if value := texts.ofCID(cid); value != "" {
			return value
		}
	}
	return replacement
}

// compositeTexts reads the TrueType program of a Type 0 font's only CIDFontType2 descendant with
// its CIDToGIDMap, nil for any other composite font.
func (x *Fixer) compositeTexts(dict map[string]any) *glyphTexts {
	descendants, _ := x.store.Resolve(dict["/DescendantFonts"]).([]any)
	if len(descendants) != 1 {
		return nil
	}
	cid, _ := x.store.Resolve(descendants[0]).(map[string]any)
	descriptor, _ := x.store.Resolve(cid["/FontDescriptor"]).(map[string]any)
	if cid["/Subtype"] == "/CIDFontType0" && descriptor["/FontFile3"] != nil {
		return x.cffTexts(descriptor["/FontFile3"], figureName(cid["/BaseFont"]))
	}
	if cid["/Subtype"] != "/CIDFontType2" || descriptor["/FontFile2"] == nil {
		return nil
	}
	texts := x.newGlyphTexts(descriptor["/FontFile2"], figureName(cid["/BaseFont"]))
	if texts == nil {
		return nil
	}
	if ref, ok := cid["/CIDToGIDMap"].(string); ok && ref != "/Identity" {
		texts.glyphMap, _ = x.store.StreamData(ref)
		if texts.glyphMap == nil {
			return nil
		}
	}
	return texts
}

// unnamedValue is the value of a code of an embedded TrueType font with no Encoding, read from its
// program (unnamedTrueType); U+FFFD when the program does not tell or f is another kind of font.
func (x *Fixer) unnamedValue(f *font, code int) string {
	if !x.unnamedTrueType(f) {
		return replacement
	}
	if texts := x.programTexts(f); texts != nil {
		if value := texts.ofCode(code); value != "" {
			return value
		}
	}
	return replacement
}

// programTexts is the glyph texts of an embedded TrueType font's program, read once.
func (x *Fixer) programTexts(f *font) *glyphTexts {
	if !f.textsRead {
		descriptor, _ := x.store.Resolve(f.dict["/FontDescriptor"]).(map[string]any)
		f.texts = x.newGlyphTexts(descriptor["/FontFile2"], figureName(f.dict["/BaseFont"]))
		f.textsRead = true
	}
	return f.texts
}
