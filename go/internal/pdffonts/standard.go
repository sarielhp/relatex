package pdffonts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
)

// The standard 14 fonts a PDF may name without embedding them, which PDF/UA-2 (8.4.5.5.1) asks
// to be embedded. Their metric-compatible free equivalents are URW's base 35 Type 1 programs
// (Nimbus Sans, Nimbus Roman, Nimbus Mono PS, Standard Symbols PS, D050000L), which viewers such
// as MuPDF and Ghostscript draw in their place. The 2017 release (urw-base35) is the one MuPDF
// builds in, so a page draws the same with the embedded copy, pixel for pixel; TeX Live's older
// release (fonts/type1/urw, 2003: NimbusSanL-Regu as uhvr8a.pfb) has the same metrics but outlines
// that differ by a few pixels, and is the fallback.

// standardProgram is a standard font's URW program in each release: urw-base35's file and
// TeX Live's.
type standardProgram struct{ base35, texLive string }

// standardFonts maps a standard font name to its URW programs.
var standardFonts = map[string]standardProgram{
	"Helvetica":             {"NimbusSans-Regular.t1", "uhvr8a.pfb"},
	"Helvetica-Bold":        {"NimbusSans-Bold.t1", "uhvb8a.pfb"},
	"Helvetica-Oblique":     {"NimbusSans-Italic.t1", "uhvro8a.pfb"},
	"Helvetica-BoldOblique": {"NimbusSans-BoldItalic.t1", "uhvbo8a.pfb"},
	"Times-Roman":           {"NimbusRoman-Regular.t1", "utmr8a.pfb"},
	"Times-Bold":            {"NimbusRoman-Bold.t1", "utmb8a.pfb"},
	"Times-Italic":          {"NimbusRoman-Italic.t1", "utmri8a.pfb"},
	"Times-BoldItalic":      {"NimbusRoman-BoldItalic.t1", "utmbi8a.pfb"},
	"Courier":               {"NimbusMonoPS-Regular.t1", "ucrr8a.pfb"},
	"Courier-Bold":          {"NimbusMonoPS-Bold.t1", "ucrb8a.pfb"},
	"Courier-Oblique":       {"NimbusMonoPS-Italic.t1", "ucrro8a.pfb"},
	"Courier-BoldOblique":   {"NimbusMonoPS-BoldItalic.t1", "ucrbo8a.pfb"},
	"Symbol":                {"StandardSymbolsPS.t1", "usyr.pfb"},
	"ZapfDingbats":          {"D050000L.t1", "uzdr.pfb"},
}

// base35Directories are where systems install urw-base35's Type 1 programs (Debian's
// fonts-urw-base35, Fedora's urw-base35-fonts), when kpsewhich does not find them in TeX Live.
var base35Directories = []string{"/usr/share/fonts/type1/urw-base35", "/usr/share/fonts/urw-base35",
	"/usr/local/share/fonts/urw-base35"}

// standardStyles are the style suffixes of the "Family,Style" names Acrobat writes
// (Helvetica,Bold), with the standard face of each family they select.
var standardStyles = map[string]map[string]string{
	"Helvetica": {"Bold": "Helvetica-Bold", "Italic": "Helvetica-Oblique", "BoldItalic": "Helvetica-BoldOblique"},
	"Times":     {"Bold": "Times-Bold", "Italic": "Times-Italic", "BoldItalic": "Times-BoldItalic"},
	"Courier":   {"Bold": "Courier-Bold", "Italic": "Courier-Oblique", "BoldItalic": "Courier-BoldOblique"},
}

// standardFamilies are the families whose fonts the standard ones stand for: the standard
// families themselves and the metric-compatible core fonts Acrobat substitutes with them
// (ArialMT, TimesNewRomanPSMT, CourierNewPSMT).
var standardFamilies = map[string]string{"Helvetica": "Helvetica", "Arial": "Helvetica", "Times": "Times",
	"TimesNewRoman": "Times", "Courier": "Courier", "CourierNew": "Courier"}

// StandardFont is the standard font a base font name selects ("Helvetica,Bold" is
// Helvetica-Bold, "Arial-BoldMT,Italic" Helvetica-BoldOblique), "" when it is none of the 14.
func StandardFont(name string) string {
	name = strings.TrimPrefix(name, "/")
	if _, ok := standardFonts[name]; ok {
		return name
	}
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == ',' })
	if len(parts) == 0 {
		return ""
	}
	// A name with spaces ("Times New Roman,Bold", as Word writes it in figures: papers 0068, 0173)
	// is the family without them, as a viewer reads it; it was drawn with Times, not substituted.
	family := standardFamilies[strings.TrimSuffix(strings.TrimSuffix(strings.ReplaceAll(parts[0], " ", ""), "MT"), "PS")]
	bold, italic := false, false
	for _, part := range parts[1:] {
		switch strings.TrimSuffix(part, "MT") {
		case "Bold":
			bold = true
		case "Italic", "Oblique":
			italic = true
		case "BoldItalic", "BoldOblique":
			bold, italic = true, true
		case "Roman", "Regular":
		default:
			return ""
		}
	}
	return styledFace(family, bold, italic)
}

// SubstituteFont is the standard font a viewer draws a nonsymbolic font without a program and of no
// standard family with, as MuPDF chooses it (pdf_load_system_font): Courier for a fixed-pitch font,
// Times for a serif one, else Helvetica; bold when the name says Bold or the descriptor forces it,
// italic when the name says Italic or Oblique or the descriptor's flags do. "" for a symbolic font,
// whose codes a substitute would draw otherwise.
func SubstituteFont(name string, descriptor map[string]any) string {
	flags := int(Number(descriptor["/Flags"]))
	if flags&4 != 0 {
		return ""
	}
	family := "Helvetica"
	switch {
	case flags&1 != 0:
		family = "Courier"
	case flags&2 != 0:
		family = "Times"
	}
	bold := strings.Contains(name, "Bold") || flags&(1<<18) != 0
	italic := strings.Contains(name, "Italic") || strings.Contains(name, "Oblique") || flags&64 != 0
	return styledFace(family, bold, italic)
}

// styledFace is the standard face of a standard family in a style.
func styledFace(family string, bold, italic bool) string {
	style := map[[2]bool]string{{true, false}: "Bold", {false, true}: "Italic", {true, true}: "BoldItalic"}[[2]bool{bold, italic}]
	if style == "" {
		return map[string]string{"Helvetica": "Helvetica", "Times": "Times-Roman", "Courier": "Courier"}[family]
	}
	return standardStyles[family][style]
}

// programs reads the URW programs, each once.
type programs struct {
	loaded map[string]*Type1
	files  map[string]string // standard font -> the file read
}

// load is the URW program standing in for a standard font, nil when none can be read.
func (p *programs) load(standard string) *Type1 {
	if font, ok := p.loaded[standard]; ok {
		return font
	}
	if p.loaded == nil {
		p.loaded, p.files = map[string]*Type1{}, map[string]string{}
	}
	p.loaded[standard] = nil
	for _, path := range programPaths(standardFonts[standard]) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if font, err := ParseType1(data, 0, 0); err == nil && len(font.Widths) > 0 {
			p.loaded[standard], p.files[standard] = font, filepath.Base(path)
			break
		}
	}
	return p.loaded[standard]
}

// programPaths are the files that may hold a standard font's program, the preferred first.
func programPaths(program standardProgram) []string {
	if program.base35 == "" {
		return nil
	}
	var paths []string
	if found := kpseFind(program.base35); found != "" {
		paths = append(paths, found)
	}
	for _, directory := range base35Directories {
		paths = append(paths, filepath.Join(directory, program.base35))
	}
	if found := kpseFind(program.texLive); found != "" {
		paths = append(paths, found)
	}
	return paths
}

func kpseFind(name string) string {
	output, err := exec.Command(command.TeXLiveBin("kpsewhich"), name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// descriptorFlags are the font descriptor's flags for a standard font's program.
func descriptorFlags(standard string, program *Type1) int {
	flags := 32 // nonsymbolic
	if standard == "Symbol" || standard == "ZapfDingbats" {
		flags = 4
	}
	if program.FixedPitch {
		flags |= 1
	}
	if strings.HasPrefix(standard, "Times") {
		flags |= 2
	}
	if program.ItalicAngle != 0 {
		flags |= 64
	}
	return flags
}

// standardDescriptor is the font descriptor of an embedded standard font, its program in fontFile.
func standardDescriptor(name, standard string, program *Type1, fontFile string) map[string]any {
	box := []any{program.BBox[0], program.BBox[1], program.BBox[2], program.BBox[3]}
	stem := program.StemV
	if stem == 0 {
		stem = 80
	}
	return map[string]any{"/Type": "/FontDescriptor", "/FontName": name, "/Flags": descriptorFlags(standard, program),
		"/FontBBox": box, "/ItalicAngle": program.ItalicAngle, "/Ascent": program.BBox[3], "/Descent": program.BBox[1],
		"/CapHeight": program.BBox[3], "/StemV": stem, "/FontFile": fontFile}
}

// fontFileStream is a program's FontFile stream: its dictionary and data.
func fontFileStream(program *Type1) (map[string]any, []byte) {
	data := make([]byte, 0, len(program.Clear)+len(program.Binary)+len(program.Trailer))
	data = append(append(append(data, program.Clear...), program.Binary...), program.Trailer...)
	return map[string]any{"/Length1": len(program.Clear), "/Length2": len(program.Binary),
		"/Length3": len(program.Trailer)}, data
}
