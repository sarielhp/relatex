package pdffonts

import "strings"

// Drawing fonts (decision 105, "Drawing pieces"): fonts whose glyphs are pieces of a drawing, not
// characters. LaTeX's picture mode draws \line and \vector from slanted segments and arrowheads
// (line10, linew10) and \circle from quarter circles (lcircle10, lcirclew10); xy-pic draws arrow
// tips, tails, squiggles, dashes and circle arcs from its own fonts (xyatip10, xybtip10, xybsql10,
// xycirc10, xydash10, and the Computer Modern, Euler and Lucida tips xycmat10 ... xylubt12). Their
// glyph names (a1, d15) collide with Zapf Dingbats' or name nothing: no table may spell them. Each
// code shown reads U+FFFD, which PDF/UA-2 accepts (8.4.5.8), and is reported as a drawing piece;
// the caller shows the glyphs as drawing, with no text (pdftag: an empty ActualText, inside the
// formula or figure they belong to, else an Artifact).

// drawingPrograms are the drawing fonts' PostScript names (their programs' FontName) and their TFM
// names, upper case, as a PDF names a Type 1 or a Type 3 font.
var drawingPrograms = map[string]bool{
	"LINE10": true, "LINEW10": true, "LCIRCLE10": true, "LCIRCLEW10": true,
	"LCIRCLE1": true, "LCIRCLEW": true,
}

// drawingFamilies are xy-pic's font families: their programs are named FAMILY-Medium, their TFMs
// FAMILY10, FAMILY11, FAMILY12.
var drawingFamilies = []string{"XYATIP", "XYBTIP", "XYBSQL", "XYCIRC", "XYDASH",
	"XYCMAT", "XYCMBT", "XYEUAT", "XYEUBT", "XYLUAT", "XYLUBT"}

// Drawing reports whether a font dictionary's BaseFont (a PDF name, /NAME or /ABCDEF+NAME) is a
// drawing font's.
func Drawing(baseFont string) bool {
	name := strings.ToUpper(baseName(baseFont))
	if drawingPrograms[name] {
		return true
	}
	for _, family := range drawingFamilies {
		rest, found := strings.CutPrefix(name, family)
		if found && (rest == "-MEDIUM" || rest == "10" || rest == "11" || rest == "12") {
			return true
		}
	}
	return false
}
