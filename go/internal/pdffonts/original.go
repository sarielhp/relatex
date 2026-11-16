package pdffonts

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/sarielhp/relatex/internal/glyphnames"
)

// matchTolerance is how far (in em) two anchor points of "the same" outline may lie apart: a
// subset cut from an installed TrueType program has the very same points (0), one converted to
// CFF with other units per em rounds each point to its integer grid (at most 0.5 unit of 1000
// or 2048 per axis, two roundings of a hinted-away fraction), so 1.5/1000 em accepts both and is
// far below the distance between two different letters' points.
const matchTolerance = 0.0015

// spaceTolerance is how far (in em) an empty glyph's advance may differ from the font's space.
const spaceTolerance = 0.02

// Installed is a font file of the machine: the original of a figure's subset.
type Installed struct {
	File string // path
	Data []byte
}

// Locator finds the installed original of a figure font by its PDF base font name (subset tag
// removed): nil when the machine has none. Lookalikes (Carlito for Calibri) are not originals.
type Locator interface {
	Locate(base string) *Installed
}

// fontconfig lists the machine's fonts once and reads the file of the one a name denotes.
type fontconfig struct {
	listed  bool
	entries []fcEntry
}

// fcEntry is one font of fc-list: its file, face index in a collection, families, styles and PostScript name.
type fcEntry struct {
	file, postscript string
	face             int
	families, styles []string
}

// fcFile is a font file and the face in it fontconfig matched (0 unless the file is a collection).
type fcFile struct {
	file string
	face int
}

// Locate finds the font whose PostScript name is base, or else whose family and style are those
// base spells (Calibri-Bold, Calibri,Bold, Calibri); the first of several files by path.
func (f *fontconfig) Locate(base string) *Installed {
	if !f.listed {
		f.entries, f.listed = listFonts(), true
	}
	want := fold(base)
	family, style := splitStyle(base)
	var byName, byFamily []fcFile
	for _, e := range f.entries {
		switch {
		case e.postscript != "" && fold(e.postscript) == want:
			byName = append(byName, fcFile{e.file, e.face})
		case slices.ContainsFunc(e.families, func(name string) bool { return fold(name) == fold(family) }) &&
			slices.ContainsFunc(e.styles, func(name string) bool { return styleKey(name) == styleKey(style) }):
			byFamily = append(byFamily, fcFile{e.file, e.face})
		}
	}
	for _, files := range [][]fcFile{byName, byFamily} {
		slices.SortFunc(files, func(a, b fcFile) int { return strings.Compare(a.file, b.file) })
		for _, file := range files {
			data, err := os.ReadFile(file.file)
			if err == nil && isCollection(data) {
				data = collectionFace(data, file.face)
			}
			if data != nil && isSfnt(data) {
				return &Installed{File: file.file, Data: data}
			}
		}
	}
	return nil
}

// listFonts runs fc-list; no fontconfig means no installed fonts. Windows has no fontconfig: its
// font folders are scanned instead (decision 129).
func listFonts() []fcEntry {
	if runtime.GOOS == "windows" {
		return scanFonts(windowsFontDirectories())
	}
	out, err := exec.Command("fc-list", "-f", "%{file}\t%{family}\t%{style}\t%{postscriptname}\t%{index}\n").Output()
	if err != nil {
		return nil
	}
	var entries []fcEntry
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 5 {
			continue
		}
		face, _ := strconv.Atoi(parts[4])
		entries = append(entries, fcEntry{file: parts[0], families: strings.Split(parts[1], ","),
			styles: strings.Split(parts[2], ","), postscript: parts[3], face: face})
	}
	return entries
}

// isCollection reports a TrueType collection (.ttc): several fonts sharing one file.
func isCollection(data []byte) bool {
	return len(data) > 12 && string(data[:4]) == "ttcf"
}

// collectionFace is one font of a collection as an sfnt the readers take: a copy of the file
// with that font's table directory moved to the start. Table offsets in a collection count from
// the file start, and the directories lie before every table, so the copy changes no table.
// nil when the face is not in the file.
func collectionFace(data []byte, face int) []byte {
	count := int(binary.BigEndian.Uint32(data[8:]))
	if face < 0 || face >= count || 16+4*face > len(data) {
		return nil
	}
	at := int(binary.BigEndian.Uint32(data[12+4*face:]))
	if at < 0 || at+12 > len(data) {
		return nil
	}
	size := 12 + 16*int(binary.BigEndian.Uint16(data[at+4:]))
	if at+size > len(data) {
		return nil
	}
	sfnt := slices.Clone(data)
	copy(sfnt, data[at:at+size])
	return sfnt
}

// isSfnt reports a TrueType program of glyf outlines (not a collection, not CFF).
func isSfnt(data []byte) bool {
	return len(data) > 12 && (binary.BigEndian.Uint32(data) == 0x00010000 || string(data[:4]) == "true")
}

// fold is a name without spaces, dashes and case.
func fold(name string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(name))
}

// splitStyle reads a base font name as family and style: Calibri-Bold, TimesNewRoman,Italic.
func splitStyle(base string) (string, string) {
	if at := strings.LastIndexAny(base, "-,"); at > 0 {
		return base[:at], base[at+1:]
	}
	return base, "Regular"
}

// styleKey makes style names comparable: Regular, Roman, Book and none are one, "Bold Italic" is BoldItalic.
func styleKey(style string) string {
	key := fold(style)
	if key == "roman" || key == "book" || key == "normal" || key == "" {
		return "regular"
	}
	return key
}

// originalFont is an installed font read for identification: the glyph shapes, Unicode values,
// ligatures and post names of its program, and where it came from.
type originalFont struct {
	outlines  *outlineFont
	unicode   map[int]int
	names     []string
	ligature  map[int][][]int
	derived   map[int][]derivation // glyph -> the glyphs it is a MATH variant, part or GSUB substitution of
	shapes    map[int]glyphShape
	space     float64
	file      string
	sha       string
	version   string
	spellName func(name string) string
	program   *TrueType      // its cmaps, for symbol codes
	symbol    map[int]string // Adobe Symbol's names by code when the original is a Symbol font (SymbolMT), else nil
}

// loadOriginal reads an installed font; nil when it is unusable (no outlines).
func loadOriginal(found *Installed, spell func(string) string) *originalFont {
	outlines, err := parseOutlines(found.Data)
	if err != nil {
		return nil
	}
	program, err := ParseTrueType(found.Data)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(found.Data)
	original := &originalFont{outlines: outlines, unicode: program.UnicodeOf(), names: program.Names,
		ligature: ligatures(outlines.tables["GSUB"]),
		derived:  derivations(outlines.tables["MATH"], outlines.tables["GSUB"]), file: filepath.Base(found.File), program: program,
		sha: hex.EncodeToString(sum[:]), version: fontVersion(outlines.tables["head"]), spellName: spell}
	if gid, ok := program.Cmaps[[2]int{3, 1}][0x20]; ok {
		original.space = outlines.advance(gid)
	}
	return original
}

// fontVersion is the head table's fontRevision ("6.23") and checksum adjustment.
func fontVersion(head []byte) string {
	if len(head) < 12 {
		return ""
	}
	revision := float64(binary.BigEndian.Uint32(head[4:])) / 65536
	return fmt.Sprintf("%s head:%08x", strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", revision), "0"), "."),
		binary.BigEndian.Uint32(head[8:]))
}

// shapeOf is the shape of an original glyph, read once.
func (o *originalFont) shapeOf(gid int) (glyphShape, bool) {
	if shape, ok := o.shapes[gid]; ok {
		return shape, true
	}
	shape, ok := o.outlines.shape(gid)
	if ok {
		if o.shapes == nil {
			o.shapes = map[int]glyphShape{}
		}
		o.shapes[gid] = shape
	}
	return shape, ok
}

// match lists the original's glyphs that draw shape.
func (o *originalFont) match(shape glyphShape) []int {
	var hits []int
	for gid := 1; gid < o.outlines.glyphs; gid++ {
		if candidate, ok := o.shapeOf(gid); ok && !candidate.empty() && shapesAlike(shape, candidate, matchTolerance) {
			hits = append(hits, gid)
		}
	}
	return hits
}

// textOf is the characters of an original glyph and how they were found: its Unicode cmap code or
// post name ("exact"), else the glyph sequence it is the ligature of ("ligature").
func (o *originalFont) textOf(gid int) (string, string) {
	if text := o.exactText(gid); text != "" {
		return text, "exact"
	}
	if text := o.ligatureText(gid, 0); text != "" {
		return text, "ligature"
	}
	if text, _ := o.derivedText(gid, 0); text != "" {
		return text, "variant"
	}
	return "", ""
}

func (o *originalFont) exactText(gid int) string {
	if code, ok := o.unicode[gid]; ok {
		return string(rune(code))
	}
	if gid >= 0 && gid < len(o.names) && o.names[gid] != "" && o.names[gid] != ".notdef" {
		return o.spellName(o.names[gid])
	}
	if code := symbolCode(o.program, gid); o.symbol != nil && code >= 0 && o.symbol[code] != "" {
		return o.spellName(o.symbol[code])
	}
	return ""
}

// ligatureText reads a ligature glyph as its components' characters; "" when its sequences do
// not all read the same.
func (o *originalFont) ligatureText(gid, depth int) string {
	var text string
	for _, sequence := range o.ligature[gid] {
		var spelled strings.Builder
		for _, part := range sequence {
			piece := o.exactText(part)
			if piece == "" && depth < 3 {
				piece = o.ligatureText(part, depth+1)
			}
			spelled.WriteString(piece)
			if piece == "" {
				break
			}
		}
		if text != "" && spelled.String() != text || spelled.Len() == 0 {
			return ""
		}
		text = spelled.String()
	}
	return text
}

// widthCharacter is the character of an empty glyph by its advance (em): U+200B for none, the
// space for the font's own space width, else the nearest of the fixed spaces.
func widthCharacter(width, space float64) string {
	if width < 0.001 {
		return "\u200B"
	}
	if space > 0 && math.Abs(width-space) <= spaceTolerance {
		return " "
	}
	best, distance := " ", math.Inf(1)
	for _, fixed := range []struct {
		char  string
		width float64
	}{{"\u2002", 0.5}, {"\u2003", 1}, {"\u2004", 1.0 / 3}, {"\u2005", 0.25}, {"\u2006", 1.0 / 6}, {"\u2009", 0.2}, {"\u200A", 0.1}} {
		if d := math.Abs(width - fixed.width); d < distance {
			best, distance = fixed.char, d
		}
	}
	return best
}

// identify reads what a subset glyph stands for from the original: its text and the method
// ("exact", "ligature", "width"), or "" and why not.
func (o *originalFont) identify(shape glyphShape) (text, method, why string) {
	return o.identifyWith(shape, "")
}

// identifyWith is identify with the ToUnicode value (UTF-16 hex) the PDF itself gives the glyph, "" when
// it gives none. The value decides only between outline matches that read as different characters, when it
// is exactly one of them (decision 118, amendment 2); it never identifies a glyph alone.
func (o *originalFont) identifyWith(shape glyphShape, toUnicode string) (text, method, why string) {
	if shape.empty() {
		return widthCharacter(shape.width, o.space), "width", ""
	}
	hits := o.match(shape)
	if len(hits) == 0 {
		return "", "", "no glyph of the installed font has this outline"
	}
	texts := map[string]string{}
	for _, gid := range hits {
		if found, how := o.textOf(gid); found != "" {
			texts[found] = how
		}
	}
	switch len(texts) {
	case 0:
		return "", "", o.noCharacter(hits)
	case 1:
		for found, how := range texts {
			return found, how, ""
		}
	}
	return o.arbitrate(texts, toUnicode)
}

// arbitrate settles several outline matches that read as different characters by the PDF's own
// ToUnicode value: accepted when it is exactly one of the candidates, else the glyph stays unidentified.
func (o *originalFont) arbitrate(texts map[string]string, toUnicode string) (string, string, string) {
	const several = "several glyphs match with different characters"
	if toUnicode == "" || Unusable(toUnicode) {
		return "", "", several
	}
	for found := range texts {
		if hexOf(found) == toUnicode {
			return found, "tounicode-confirmed", ""
		}
	}
	return "", "", several + ", and the ToUnicode " + uPlus(toUnicode) + " is none of them (ToUnicode disagrees)"
}

// uPlus spells a UTF-16 hex value as code points (U+1D45C).
func uPlus(hex string) string {
	var units []uint16
	for at := 0; at+4 <= len(hex); at += 4 {
		unit, _ := strconv.ParseUint(hex[at:at+4], 16, 16)
		units = append(units, uint16(unit))
	}
	var spelled []string
	for _, r := range utf16.Decode(units) {
		spelled = append(spelled, fmt.Sprintf("U+%04X", r))
	}
	return strings.Join(spelled, " ")
}

// noCharacter is why none of the matching glyphs reads as text: the first reason the derivation gave, else none.
func (o *originalFont) noCharacter(hits []int) string {
	for _, gid := range hits {
		if _, why := o.derivedText(gid, 0); why != "" {
			return why
		}
	}
	return "the matching glyph has no character"
}

// hexOf is a text as a ToUnicode value.
func hexOf(text string) string { return glyphnames.UTF16Hex(text) }

// SystemFonts is the Locator of the machine's fonts, found through fontconfig.
func SystemFonts() Locator { return &fontconfig{} }
