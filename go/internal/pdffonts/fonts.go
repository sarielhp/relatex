// Package pdffonts makes the simple fonts of a PDF (an included figure's above all) conform to
// PDF/UA-2's font rules without changing what any page shows (decisions 104, 105): a standard
// font is embedded as its URW equivalent (8.4.5.5.1), every code the pages show gets a usable
// ToUnicode value (8.4.5.8, 8.4.3), and a Widths entry that disagrees with its embedded program is
// corrected where no glyph's position depends on it (8.4.5.6). Text stays text, never outlines. It
// is an output library shared by both routes of the tagged PDF (internal/pdfua, internal/pdftag),
// which give it their PDF objects through Store.
package pdffonts

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/glyphnames"
)

// Store is a PDF's objects as qpdf's JSON shows them: names "/N", references "N 0 R", numbers as
// float64 or json.Number.
type Store interface {
	Resolve(value any) any                                  // a value with references followed (a stream: its dictionary)
	StreamData(ref string) ([]byte, error)                  // a stream's decoded data
	Set(ref string, value map[string]any)                   // replaces an object's value
	SetStream(ref string, dict map[string]any, data []byte) // replaces a stream's dictionary and data
	Add(value map[string]any) string                        // a new object's reference
	AddStream(dict map[string]any, data []byte) string      // a new stream object's reference
}

// Report says what the fonts' repair did.
type Report struct {
	Embedded  []string `json:"standard_fonts_embedded,omitempty"` // base font -> URW program
	Fonts     int      `json:"fonts_completed"`                   // fonts whose ToUnicode map changed
	Codes     int      `json:"codes_added"`
	Replaced  int      `json:"values_replaced,omitempty"` // U+0000, U+FEFF, U+FFFE or private-use values respelled
	Widths    int      `json:"widths_corrected,omitempty"`
	Stretched int      `json:"widths_stretched,omitempty"` // codes the tagged PDF scales to their old width (Stretch)
	GlyphMaps int      `json:"cid_to_gid_maps,omitempty"`  // TrueType CIDFonts given their implicit /Identity map
	Unspelled []string `json:"unspelled,omitempty"`        // font: code (glyph name) mapped to U+FFFD
	Drawn     []string `json:"drawing_pieces,omitempty"`   // a drawing font's codes shown (U+FFFD, no text)
	// Identified are the figure fonts whose glyphs were read from the installed original; Unidentified
	// those with glyphs left at U+FFFD (decision 118).
	Identified   []Identified   `json:"figure_fonts_identified,omitempty"`
	Unidentified []Unidentified `json:"figure_fonts_unidentified,omitempty"`
	// Unwrapped are the figure fonts whose /FontFile2 was a whole TrueType collection and now is
	// the one face the font names (pdfua.font_unwrapped).
	Unwrapped []Unwrapped `json:"collections_unwrapped,omitempty"`
	// CmapsAdded are the figure fonts whose program got a (3,1) cmap record (pdfua.font_cmap_added).
	CmapsAdded []CmapAdded `json:"cmaps_added,omitempty"`
}

// Use is what the pages do with one font: the codes they show, and those whose advance width
// positions a later glyph (moving it if the Widths entry changed). A use without codes is unknown
// (a figure file before LaTeX includes it): every code the font names counts as shown and as
// placing a later glyph.
type Use struct {
	Codes    map[int]bool
	Advanced map[int]bool
	// Compensated: the caller moves every glyph placed after a code whose width changes back by
	// the change (Fixer.Moved), so that nothing moves; every width shown may then be corrected, and
	// a font with no standard equivalent may be embedded as the one a viewer draws it with.
	Compensated bool
}

// Fixer repairs fonts of one PDF.
type Fixer struct {
	store    Store
	table    glyphnames.Table
	programs programs
	// Locator finds the installed original of a figure font; nil identifies nothing (SystemFonts is the machine's).
	Locator   Locator
	originals map[string]*originalEntry
	// documentFonts are the names of the originals of every font the document uses (Prepare), the
	// candidates for a glyph whose own original is not found.
	documentFonts []string
	Report        Report
	// Moved are the width changes of compensated fonts: font reference -> code -> the new advance
	// less the old, in thousandths of the font size, as a TJ kern would undo it.
	Moved map[string]map[int]float64
	// Stretched are the codes, by font reference, of the fonts embedded in place of a viewer's
	// substitute whose Widths changed: their glyphs are scaled to the width the viewer drew (Stretch).
	Stretched map[string]map[int]Stretch
	// Drawn are the drawing fonts shown (Drawing), by reference: their glyphs are no text.
	Drawn map[string]bool
}

// NewFixer is a fixer of the fonts in store, spelling glyph names by table.
func NewFixer(store Store, table glyphnames.Table) *Fixer {
	return &Fixer{store: store, table: table}
}

// font is one simple font being repaired.
type font struct {
	ref        string
	dict       map[string]any
	program    *Type1
	symbolic   bool
	embedded   bool           // a standard font embedded here
	substitute bool           // embedded in place of a viewer substitute, not as a standard 14 family font (a viewer stretches those)
	drawing    bool           // a drawing font (Drawing): every code shown reads U+FFFD
	builtin    map[int]string // a CFF program's built-in encoding (FontFile3 /Type1C)
	missing    float64        // the advance of a code outside FirstChar..LastChar (the original MissingWidth)
	changed    bool
	texts      *glyphTexts // the program's glyph texts (unnamedValue), read once
	textsRead  bool
}

// simpleFonts are the font types whose codes are one byte, each a glyph the encoding names.
var simpleFonts = map[string]bool{"/Type1": true, "/MMType1": true, "/TrueType": true, "/Type3": true}

// Fix repairs the font at ref (a simple font: Type1, MMType1, TrueType, Type3) for the use the pages make
// of it; other fonts are left alone. It reports whether the font changed.
func (x *Fixer) Fix(ref string, use Use) bool {
	dict, _ := x.store.Resolve(ref).(map[string]any)
	subtype, _ := dict["/Subtype"].(string)
	if subtype == "/Type0" {
		named := x.identityGlyphMap(dict)
		return x.fixComposite(ref, dict, use) || named
	}
	if dict == nil || !simpleFonts[subtype] {
		return false
	}
	f := &font{ref: ref, dict: maps.Clone(dict)}
	if name, _ := dict["/BaseFont"].(string); Drawing(name) {
		f.drawing = true
		x.markDrawn(ref)
	}
	original, _ := x.store.Resolve(f.dict["/FontDescriptor"]).(map[string]any)
	f.missing = Number(original["/MissingWidth"])
	x.embedStandard(f, use.Compensated)
	x.readProgram(f)
	descriptor, _ := x.store.Resolve(f.dict["/FontDescriptor"]).(map[string]any)
	f.symbolic = int(Number(descriptor["/Flags"]))&4 != 0
	names := x.names(f)
	if use.Codes == nil {
		use = namedUse(f.dict, names)
	}
	if f.embedded && f.dict["/Widths"] == nil {
		addWidths(f, use, names)
	}
	x.completeMap(f, use, names)
	x.finishIdentity(f.dict["/BaseFont"], f.texts)
	x.correctWidths(f, use, names)
	if f.changed {
		x.store.Set(ref, f.dict)
	}
	return f.changed
}

// namedUse is the use of a font whose use is unknown: every code its encoding names within its
// FirstChar and LastChar, each placing a later glyph.
func namedUse(dict map[string]any, names map[int]string) Use {
	use := Use{Codes: map[int]bool{}, Advanced: map[int]bool{}}
	first, last := Number(dict["/FirstChar"]), 255.0
	if dict["/LastChar"] != nil {
		last = Number(dict["/LastChar"])
	}
	for code, name := range names {
		if name != "" && name != ".notdef" && float64(code) >= first && float64(code) <= last {
			use.Codes[code], use.Advanced[code] = true, true
		}
	}
	return use
}

// embedStandard embeds a standard font without a program as its URW equivalent; Widths are
// added from the program when the font has none (a viewer took them from its own copy). A
// TrueType font of a standard family without a program (ArialMT, "Helvetica,Italic" as TrueType)
// reads its glyphs by name through its encoding, as a Type 1 font does, and becomes one; so does
// one flagged symbolic (Word's TimesNewRomanPSMT and ArialMT, with or without an Encoding), which a
// viewer draws with the standard font's glyphs through the encoding or the program's own. With
// substitute (the caller compensates width changes), a font of no standard family (Tahoma) is
// embedded as the standard font a viewer draws in its place (SubstituteFont): it looked so before.
func (x *Fixer) embedStandard(f *font, substitute bool) {
	descriptor, _ := x.store.Resolve(f.dict["/FontDescriptor"]).(map[string]any)
	if descriptor["/FontFile"] != nil || descriptor["/FontFile2"] != nil || descriptor["/FontFile3"] != nil {
		return
	}
	if f.dict["/Subtype"] == "/Type3" {
		return
	}
	name, _ := f.dict["/BaseFont"].(string)
	standard := StandardFont(name)
	if standard == "" && substitute {
		standard = SubstituteFont(name, descriptor)
		f.substitute = standard != ""
	}
	program := x.programs.load(standard)
	if standard == "" || program == nil {
		return
	}
	streamDict, data := fontFileStream(program)
	fontFile := x.store.AddStream(streamDict, data)
	f.dict["/FontDescriptor"] = x.store.Add(standardDescriptor(name, standard, program, fontFile))
	f.dict["/Subtype"], f.program, f.embedded, f.changed = "/Type1", program, true, true
	x.Report.Embedded = append(x.Report.Embedded, fmt.Sprintf("%s %s -> %s", f.ref, name, x.programs.files[standard]))
}

// addWidths gives an embedded standard font the Widths of its program over the codes it shows.
func addWidths(f *font, use Use, names map[int]string) {
	codes := slices.Sorted(maps.Keys(use.Codes))
	if len(codes) == 0 {
		return
	}
	first, last := codes[0], codes[len(codes)-1]
	widths := make([]any, 0, last-first+1)
	for code := first; code <= last; code++ {
		widths = append(widths, round(f.program.Widths[names[code]]))
	}
	f.dict["/FirstChar"], f.dict["/LastChar"], f.dict["/Widths"] = first, last, widths
}

// readProgram parses an embedded Type 1 program (FontFile), or a bare CFF one's (FontFile3
// /Type1C) built-in encoding.
func (x *Fixer) readProgram(f *font) {
	if f.program != nil {
		return
	}
	descriptor, _ := x.store.Resolve(f.dict["/FontDescriptor"]).(map[string]any)
	ref, ok := descriptor["/FontFile"].(string)
	if !ok {
		x.readCFF(f, descriptor)
		return
	}
	data, err := x.store.StreamData(ref)
	if err != nil {
		return
	}
	streamDict, _ := x.store.Resolve(ref).(map[string]any)
	f.program, _ = ParseType1(data, int(Number(streamDict["/Length1"])), int(Number(streamDict["/Length2"])))
}

// readCFF reads the built-in encoding of a font's FontFile3 /Type1C program.
func (x *Fixer) readCFF(f *font, descriptor map[string]any) {
	ref, ok := descriptor["/FontFile3"].(string)
	streamDict, _ := x.store.Resolve(ref).(map[string]any)
	if !ok || streamDict["/Subtype"] != "/Type1C" {
		return
	}
	if data, err := x.store.StreamData(ref); err == nil {
		f.builtin, _ = ParseCFFEncoding(data)
	}
}

// names are the font's glyph names by code: its program's built-in encoding (or StandardEncoding
// for a nonsymbolic font without one), the named base encoding over it, its Differences over that.
func (x *Fixer) names(f *font) map[int]string {
	names := map[int]string{}
	if f.program != nil {
		maps.Copy(names, f.program.Encoding)
	} else if f.builtin != nil {
		maps.Copy(names, f.builtin)
	} else if !f.symbolic && (f.dict["/Subtype"] == "/Type1" || f.dict["/Subtype"] == "/MMType1") {
		names = standardEncoding()
	}
	encoding := x.store.Resolve(f.dict["/Encoding"])
	if named, ok := encoding.(string); ok {
		maps.Copy(names, NamedEncoding(named))
		return names
	}
	dict, _ := encoding.(map[string]any)
	if base, ok := dict["/BaseEncoding"].(string); ok {
		maps.Copy(names, NamedEncoding(base))
	}
	differences, _ := x.store.Resolve(dict["/Differences"]).([]any)
	code := 0
	for _, value := range differences {
		if name, ok := value.(string); ok && strings.HasPrefix(name, "/") {
			names[code] = strings.TrimPrefix(name, "/")
			code++
			continue
		}
		code = int(Number(value))
	}
	return names
}

// Number reads a number of qpdf's JSON (float64 or json.Number), 0 for anything else.
func Number(value any) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case int:
		return float64(number)
	case json.Number:
		parsed, _ := number.Float64()
		return parsed
	}
	return 0
}

// round is a width as an integer when it is one, else to a tenth.
func round(width float64) any {
	if width == math.Trunc(width) {
		return int(width)
	}
	return math.Round(width*10) / 10
}
