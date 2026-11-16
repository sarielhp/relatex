package pdffonts

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Figure fonts with no Unicode source (decision 118): an included figure's subset made by a
// producer that kept no cmap, no post names and no ToUnicode (Quartz's Symbol and Calibri,
// Ghostscript's CIDFont+F3) says nothing about its characters. Its ORIGINAL font installed on
// the machine does: each subset glyph's outline is matched with the original's glyphs, and the
// original's Unicode cmap, ligature substitutions or post names read the match back. A glyph
// that draws nothing reads as the space its advance width says. What is not identified stays
// U+FFFD and is reported.

// Identified records where the characters of one figure font came from.
type Identified struct {
	Font     string   `json:"font"`   // the PDF's base font name
	Family   string   `json:"family"` // the original's family
	File     string   `json:"file"`   // the installed file's name (never copied)
	Version  string   `json:"version"`
	SHA256   string   `json:"sha256"` // of the installed file
	Exact    int      `json:"exact"`  // glyphs read by outline match and the original's cmap or post names
	Ligature int      `json:"ligature"`
	Width    int      `json:"width"` // empty glyphs read as a space by advance width
	Glyphs   []string `json:"glyphs"`
}

// Unidentified records the glyphs of one figure font left at U+FFFD.
type Unidentified struct {
	Font      string   `json:"font"`
	Family    string   `json:"family"`
	Installed bool     `json:"installed"` // the original is installed
	Glyphs    int      `json:"glyphs"`
	Unmatched []string `json:"unmatched,omitempty"` // glyph: why, when the original is installed
}

// glyphResult is what the original said about one subset glyph.
type glyphResult struct {
	text, method, why string
	from              string        // the document font whose original matched, when not the glyph's own
	original          *originalFont // that original
}

// originalEntry is a located original: nil font when the machine has none or it is unusable.
type originalEntry struct {
	font      *originalFont
	installed bool
}

// identity identifies the glyphs of one subset program.
type identity struct {
	base     string
	locate   func() *originalEntry
	shape    func(gid int) (glyphShape, bool)
	document func(shape glyphShape) (glyphResult, bool)
	results  map[int]glyphResult
	hints    map[int]string // glyph -> the PDF's own ToUnicode value for it (hint)
}

// newIdentity prepares the identification of a subset program's glyphs (shape reads one).
func (x *Fixer) newIdentity(base string, shape func(int) (glyphShape, bool)) *identity {
	return &identity{base: base, shape: shape, results: map[int]glyphResult{},
		locate:   func() *originalEntry { return x.originalOf(cMapless(base)) },
		document: func(shape glyphShape) (glyphResult, bool) { return x.documentMatch(cMapless(base), shape) }}
}

// originalOf locates and reads the installed original of base once.
func (x *Fixer) originalOf(base string) *originalEntry {
	if entry, ok := x.originals[base]; ok {
		return entry
	}
	entry := &originalEntry{}
	if x.Locator != nil {
		if found := x.Locator.Locate(base); found != nil {
			entry.installed = true
			entry.font = loadOriginal(found, func(name string) string { return x.table.SpellIn(base, name) })
			x.symbolOriginal(base, entry.font)
		}
	}
	if x.originals == nil {
		x.originals = map[string]*originalEntry{}
	}
	x.originals[base] = entry
	return entry
}

// symbolOriginal gives an installed Symbol font (Windows' SymbolMT has no Unicode cmap, only Symbol
// codes at U+F0xx) the Adobe Symbol encoding that names its codes, as a figure font named Symbol has.
func (x *Fixer) symbolOriginal(base string, font *originalFont) {
	if family := fold(familyOf(base)); font == nil || (family != "symbol" && family != "symbolmt") {
		return
	}
	if standard := x.programs.load("Symbol"); standard != nil {
		font.symbol = standard.Encoding
	}
}

// glyph is the ToUnicode value of a subset glyph from the installed original, "" when it
// cannot be identified (the reason is kept for the report). The glyph without outline and width
// 0 (.notdef) is never identified.
func (i *identity) glyph(gid int) string {
	if i == nil || gid == 0 {
		return ""
	}
	result, seen := i.results[gid]
	if !seen {
		result = i.resolve(gid)
		i.results[gid] = result
	}
	if result.text == "" || Unusable(hexOf(result.text)) {
		return ""
	}
	return hexOf(result.text)
}

// hint records the PDF's own ToUnicode value for a glyph; two codes of one glyph that disagree give no hint.
func (i *identity) hint(gid int, value string) {
	if i == nil || Unusable(value) {
		return
	}
	if i.hints == nil {
		i.hints = map[int]string{}
	}
	if known, ok := i.hints[gid]; ok && known != value {
		i.hints[gid] = ""
		return
	}
	i.hints[gid] = value
}

func (i *identity) resolve(gid int) glyphResult {
	shape, ok := i.shape(gid)
	if !ok {
		return glyphResult{why: "its outline cannot be read"}
	}
	entry := i.locate()
	if entry.font == nil && i.document != nil {
		if result, ok := i.document(shape); ok {
			return result
		}
	}
	if entry.font == nil {
		why := "the original font is not installed"
		if entry.installed {
			why = "the installed original cannot be read"
		}
		return glyphResult{why: why}
	}
	text, method, why := entry.font.identifyWith(shape, i.hints[gid])
	return glyphResult{text: text, method: method, why: why}
}

var cMapName = regexp.MustCompile(`-(GBpc-EUC|GBK-EUC|GBK2K|GB-EUC|UniGB-UCS2|UniGB-UTF16|B5pc|ETen-B5|ETenms-B5|UniCNS-UCS2|90ms-RKSJ|90pv-RKSJ|UniJIS-UCS2|KSC-EUC|KSCms-UHC|UniKS-UCS2|Identity)-[HV]$`)

// cMapless is a composite font's name without the CMap name its producer appended.
func cMapless(base string) string { return cMapName.ReplaceAllString(base, "") }

// figureName is a base font name as text: #xx escapes decoded, no slash, no subset tag.
func figureName(raw any) string {
	name, _ := raw.(string)
	name = strings.TrimPrefix(name, "/")
	var decoded []byte
	for at := 0; at < len(name); at++ {
		if name[at] == '#' && at+2 < len(name) {
			if value, err := strconv.ParseUint(name[at+1:at+3], 16, 8); err == nil {
				decoded, at = append(decoded, byte(value)), at+2
				continue
			}
		}
		decoded = append(decoded, name[at])
	}
	return baseName("/" + string(decoded))
}

// familyOf is the family a base font name stands for: no CMap name, no style.
func familyOf(base string) string {
	family, _ := splitStyle(cMapless(base))
	return family
}

// finishIdentity moves what the identification of a font's glyphs found into the report.
func (x *Fixer) finishIdentity(raw any, g *glyphTexts) {
	if g == nil || g.identity == nil || len(g.identity.results) == 0 {
		return
	}
	base := figureName(raw)
	entry := g.identity.locate()
	found := Identified{Font: base, Family: familyOf(g.identity.base)}
	source := entry.font
	lost := Unidentified{Font: base, Family: found.Family, Installed: entry.font != nil}
	for _, gid := range slices.Sorted(maps.Keys(g.identity.results)) {
		result := g.identity.results[gid]
		if result.text == "" || Unusable(hexOf(result.text)) {
			lost.Glyphs++
			lost.Unmatched = append(lost.Unmatched, fmt.Sprintf("glyph %d: %s", gid, result.why))
			continue
		}
		found.count(result.method)
		how := result.method
		if result.from != "" {
			how, source = fmt.Sprintf("%s in %s, a font of the document", how, result.from), result.original
		}
		found.Glyphs = append(found.Glyphs, fmt.Sprintf("glyph %d: %+q (%s)", gid, result.text, how))
	}
	if found.Exact+found.Ligature+found.Width > 0 && source != nil {
		found.File, found.Version, found.SHA256 = source.file, source.version, source.sha
		x.Report.Identified = append(x.Report.Identified, found)
	}
	if lost.Glyphs > 0 {
		x.Report.Unidentified = append(x.Report.Unidentified, lost)
	}
}

func (f *Identified) count(method string) {
	switch method {
	case "ligature":
		f.Ligature++
	case "width":
		f.Width++
	default:
		f.Exact++
	}
}

// cffTexts reads the glyphs of a CID-keyed CFF subset (FontFile3) for identification; nil when
// the program cannot be read.
func (x *Fixer) cffTexts(fontFile any, base string) *glyphTexts {
	ref, ok := fontFile.(string)
	if !ok {
		return nil
	}
	data, err := x.store.StreamData(ref)
	if err != nil {
		return nil
	}
	outlines, err := parseCFFOutlines(data)
	if err != nil {
		return nil
	}
	return &glyphTexts{program: &TrueType{}, cff: outlines, spell: func(string) string { return "" },
		identity: x.newIdentity(base, outlines.shape)}
}

// Alert is a report finding for the conversion's diagnostics: a figure font family whose glyphs
// read as U+FFFD.
type Alert struct {
	Message string
	Glyphs  int
}

// Alerts says, per font family, why figure glyphs are still U+FFFD and what would help.
func (r Report) Alerts() []Alert {
	families := map[string][]Unidentified{}
	for _, lost := range r.Unidentified {
		families[lost.Family] = append(families[lost.Family], lost)
	}
	var alerts []Alert
	for _, family := range slices.Sorted(maps.Keys(families)) {
		alerts = append(alerts, familyAlert(family, families[family]))
	}
	return alerts
}

func familyAlert(family string, lost []Unidentified) Alert {
	glyphs, installed := 0, true
	var unmatched []string
	for _, entry := range lost {
		glyphs += entry.Glyphs
		installed = installed && entry.Installed
		unmatched = append(unmatched, entry.Unmatched...)
	}
	switch {
	case strings.HasPrefix(family, "CIDFont+"):
		return Alert{fmt.Sprintf("figure font %s has no character map and names no original font; its %d glyphs read as U+FFFD", family, glyphs), glyphs}
	case installed:
		unmatched = slices.Compact(slices.Sorted(slices.Values(unmatched)))
		return Alert{fmt.Sprintf("figure font %s has no character map and the installed %s did not identify %d of its glyphs (%s); they read as U+FFFD",
			family, family, glyphs, strings.Join(unmatched[:min(4, len(unmatched))], "; ")), glyphs}
	}
	return Alert{fmt.Sprintf("figure font %s has no character map; install %s so relatex can identify its %d glyphs (they read as U+FFFD until then)",
		family, family, glyphs), glyphs}
}
