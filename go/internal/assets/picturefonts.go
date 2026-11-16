package assets

import (
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcefonts"
)

// pictureFace is how a picture's label glyph is set as SVG text (decision 137): the document's
// CSS font stack for the PDF font, its weight and style, and whether the font is TeX's math italic,
// whose letters are written in the Mathematical Alphanumeric alphabet the math font draws them from.
type pictureFace struct {
	family   string
	weight   string
	style    string
	alphabet string
}

// texFace is a TeX font program's NFSS family key in profiles/font-families.json and its face.
type texFace struct {
	prefix, family, weight, style string
	alphabet                      string
}

// texFaces names TeX's own font programs by their PostScript names (lowercase prefixes, longest
// first where one prefixes another): the Computer Modern and Latin Modern programs, whose names are
// not NFSS families, and the AMS symbol fonts.
var texFaces = []texFace{
	{prefix: "cmmib", family: "cmm", weight: "bold", alphabet: "italic"},
	{prefix: "cmmi", family: "cmm", alphabet: "italic"},
	{prefix: "lmmathitalic", family: "lmm", alphabet: "italic"},
	{prefix: "cmbsy", family: "cmsy", weight: "bold", alphabet: "script"},
	{prefix: "cmsy", family: "cmsy", alphabet: "script"},
	{prefix: "lmmathsymbols", family: "cmsy", alphabet: "script"},
	{prefix: "cmex", family: "cmex"},
	{prefix: "lmmathextension", family: "cmex"},
	{prefix: "msam", family: "msa"},
	{prefix: "msbm", family: "msb"},
	{prefix: "cmbxti", family: "cmr", weight: "bold", style: "italic"},
	{prefix: "cmbxsl", family: "cmr", weight: "bold", style: "oblique"},
	{prefix: "cmbx", family: "cmr", weight: "bold"},
	{prefix: "cmssbx", family: "cmss", weight: "bold"},
	{prefix: "cmssi", family: "cmss", style: "oblique"},
	{prefix: "cmss", family: "cmss"},
	{prefix: "cmitt", family: "cmtt", style: "italic"},
	{prefix: "cmsltt", family: "cmtt", style: "oblique"},
	{prefix: "cmtt", family: "cmtt"},
	{prefix: "cmti", family: "cmr", style: "italic"},
	{prefix: "cmsl", family: "cmr", style: "oblique"},
	{prefix: "cmb", family: "cmr", weight: "bold"},
	{prefix: "cmr", family: "cmr"},
	// cm-super's Type 1 programs of the EC fonts (T1-encoded Computer Modern): sfrm1000 is ecrm1000.
	{prefix: "sfbi", family: "cmr", weight: "bold", style: "italic"},
	{prefix: "sfbx", family: "cmr", weight: "bold"},
	{prefix: "sfrm", family: "cmr"},
	{prefix: "sfti", family: "cmr", style: "italic"},
	{prefix: "sfsl", family: "cmr", style: "oblique"},
	{prefix: "sfsx", family: "cmss", weight: "bold"},
	{prefix: "sfsi", family: "cmss", style: "oblique"},
	{prefix: "sfss", family: "cmss"},
	{prefix: "sftt", family: "cmtt"},
	{prefix: "lmroman", family: "lmr"},
	{prefix: "lmsans", family: "lmss"},
	{prefix: "lmmono", family: "lmtt"},
}

// drawingFonts are fonts whose glyphs are pieces of a drawing, not text (lowercase PostScript name
// prefixes): the picture environment's line and circle segments and chess diagram fonts. Their glyphs
// stay outlines.
var drawingFonts = []string{"line10", "linew10", "lcircle10", "lcirclew10", "skak"}

// drawingFont: the font's glyphs draw, they do not write.
func drawingFont(font string) bool {
	lower := strings.ToLower(subsetTag.ReplaceAllString(font, ""))
	return slices.ContainsFunc(drawingFonts, func(prefix string) bool { return strings.HasPrefix(lower, prefix) })
}

// subsetTag is a subset font's six-letter prefix.
var subsetTag = regexp.MustCompile(`\A[A-Z]{6}\+`)

// pictureFonts maps a PDF font's PostScript name to its face: the document's own fonts first (the
// stacks the page names, decision 058), then TeX's programs by name.
type pictureFonts struct {
	document map[string]rec.Map
}

func newPictureFonts(document rec.Map) pictureFonts {
	fonts := pictureFonts{document: map[string]rec.Map{}}
	documentFonts, _ := document["fonts"].(rec.Map)
	items, _ := documentFonts["items"].([]any)
	for _, value := range items {
		item := rec.AsMap(value)
		names := []any{item["postscript_name"]}
		more, _ := item["postscript_names"].([]any)
		names = append(names, more...)
		for _, name := range names {
			if text, ok := name.(string); ok && text != "" {
				fonts.document[text] = item
			}
		}
	}
	return fonts
}

// face is the font's face, false when no stack names it.
func (f pictureFonts) face(font string) (pictureFace, bool) {
	name := subsetTag.ReplaceAllString(font, "")
	lower := strings.ToLower(name)
	known, isTeX := texFaceOf(lower)
	if item, ok := f.document[name]; ok {
		face, found := documentFace(name, item)
		if found && isTeX {
			face.family = opticalFamily(known, lower) + face.family
			// A TeX program's name does not spell its style part (sfti0900 is italic): its table entry does.
			if known.weight != "" || known.style != "" {
				face.weight, face.style = known.weight, known.style
			}
		}
		return face, found
	}
	if !isTeX {
		return pictureFace{}, false
	}
	stack := cssStack(sourcefonts.TeXFamilies(known.family), "serif")
	if stack == "" {
		return pictureFace{}, false
	}
	weight, style := known.weight, known.style
	if weight == "" && style == "" {
		weight, style = nameStyle(lower)
	}
	return pictureFace{family: opticalFamily(known, lower) + stack, weight: weight, style: style, alphabet: known.alphabet}, true
}

func texFaceOf(lower string) (texFace, bool) {
	for _, known := range texFaces {
		if strings.HasPrefix(lower, known.prefix) {
			return known, true
		}
	}
	return texFace{}, false
}

// opticalSizes are the design sizes Latin Modern installs as their own families ("LM Roman 7"), by
// the NFSS family of the TeX program.
var opticalSizes = map[string]struct {
	family string
	sizes  []string
}{
	"cmr":  {"LM Roman", []string{"5", "6", "7", "8", "9", "10", "12", "17"}},
	"lmr":  {"LM Roman", []string{"5", "6", "7", "8", "9", "10", "12", "17"}},
	"cmss": {"LM Sans", []string{"8", "9", "10", "12", "17"}},
	"lmss": {"LM Sans", []string{"8", "9", "10", "12", "17"}},
	"cmtt": {"LM Mono", []string{"8", "9", "10", "12"}},
	"lmtt": {"LM Mono", []string{"8", "9", "10", "12"}},
}

var designSize = regexp.MustCompile(`\d+`)

// opticalFamily is the Latin Modern family of a TeX program's design size, quoted and followed by a
// comma ("LM Roman 7", for cmr7), or nothing: a TeX font at 7 points is drawn from its 7-point design.
func opticalFamily(known texFace, lower string) string {
	optical, ok := opticalSizes[known.family]
	size := designSize.FindString(lower)
	// An EC font names its size in hundredths of a point: sfrm0800 is 8 points.
	if len(size) == 4 && strings.HasSuffix(size, "00") {
		size = strings.TrimLeft(strings.TrimSuffix(size, "00"), "0")
	}
	if !ok || size == "" || !slices.Contains(optical.sizes, size) {
		return ""
	}
	return `"` + optical.family + " " + size + `", `
}

func documentFace(name string, item rec.Map) (pictureFace, bool) {
	generic, _ := item["generic"].(string)
	families, _ := item["families"].([]any)
	stack := cssStack(families, generic)
	if stack == "" {
		return pictureFace{}, false
	}
	weight, style := nameStyle(strings.ToLower(name))
	return pictureFace{family: stack, weight: weight, style: style}, true
}

// nameStyle reads a weight and a style from a PostScript name's style part (after the last "-"),
// or the whole name: "URWPalladioL-BoldItal", "LMRoman10-Italic", "NimbusSanL-ReguItal".
func nameStyle(lower string) (weight, style string) {
	part := lower
	if index := strings.LastIndexByte(lower, '-'); index >= 0 {
		part = lower[index+1:]
	}
	weight = nameWeight(part)
	if strings.Contains(part, "ital") || strings.HasSuffix(part, "it") {
		style = "italic"
	}
	if strings.Contains(part, "oblique") || strings.Contains(part, "slant") || strings.Contains(part, "obl") {
		style = "oblique"
	}
	return weight, style
}

// nameWeights are the weight words of a PostScript name's style part and their CSS weights, the
// longer word first where one contains another ("semibold" before "bold", "extralight" before "light").
var nameWeights = []struct{ word, weight string }{
	{"extralight", "200"}, {"ultralight", "200"}, {"thin", "100"}, {"light", "300"}, {"medium", "500"},
	{"semibold", "600"}, {"extrabold", "800"}, {"ultrabold", "800"}, {"black", "900"}, {"heavy", "900"},
	{"bold", "bold"}, {"demi", "bold"},
}

// nameWeight is the CSS weight a style part names ("Light" is 300, as the font's own OS/2 class),
// empty for the regular weight.
func nameWeight(part string) string {
	for _, known := range nameWeights {
		if strings.Contains(part, known.word) {
			return known.weight
		}
	}
	if strings.HasSuffix(part, "bd") {
		return "bold"
	}
	return ""
}

// cssStack is the families quoted and joined, the generic class last; empty without a family.
func cssStack(families []any, generic string) string {
	parts := []string{}
	for _, family := range families {
		if name, ok := family.(string); ok && name != "" {
			parts = append(parts, `"`+strings.NewReplacer(`"`, "", `\`, "", "&", "", "<", "").Replace(name)+`"`)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if generic == "" {
		generic = "serif"
	}
	return strings.Join(append(parts, generic), ", ")
}

// scriptCapitals are the Letterlike Symbols the Mathematical Script capitals leave out.
var scriptCapitals = map[rune]rune{'B': 0x212C, 'E': 0x2130, 'F': 0x2131, 'H': 0x210B, 'I': 0x2110, 'L': 0x2112,
	'M': 0x2133, 'R': 0x211B}

// mathLetters writes the letters of TeX's math fonts as the Mathematical Alphanumeric characters the
// math font draws them with: math italic's Latin and Greek letters as Mathematical Italic (U+1D434
// on), the symbol font's capitals (\mathcal) as Mathematical Script (U+1D49C on).
func mathLetters(text, alphabet string) string {
	var out strings.Builder
	for _, letter := range text {
		switch alphabet {
		case "italic":
			out.WriteRune(mathItalic(letter))
		case "script":
			out.WriteRune(mathScript(letter))
		default:
			out.WriteRune(letter)
		}
	}
	return out.String()
}

func mathScript(letter rune) rune {
	if replaced, ok := scriptCapitals[letter]; ok {
		return replaced
	}
	if letter >= 'A' && letter <= 'Z' {
		return 0x1D49C + (letter - 'A')
	}
	return letter
}

// italicSymbols are math italic's other letters: the micro sign a ToUnicode table writes for \mu,
// and the Greek variant forms.
var italicSymbols = map[rune]rune{0xB5: 0x1D707, 0x3F5: 0x1D716, 0x3D1: 0x1D717, 0x3F0: 0x1D718, 0x3D5: 0x1D719,
	0x3F1: 0x1D71A, 0x3D6: 0x1D71B}

func mathItalic(letter rune) rune {
	if replaced, ok := italicSymbols[letter]; ok {
		return replaced
	}
	switch {
	case letter == 'h':
		return 0x210E
	case letter >= 'A' && letter <= 'Z':
		return 0x1D434 + (letter - 'A')
	case letter >= 'a' && letter <= 'z':
		return 0x1D44E + (letter - 'a')
	case letter >= 0x391 && letter <= 0x3A9 && letter != 0x3A2:
		return 0x1D6E2 + (letter - 0x391)
	case letter >= 0x3B1 && letter <= 0x3C9:
		return 0x1D6FC + (letter - 0x3B1)
	}
	return letter
}
