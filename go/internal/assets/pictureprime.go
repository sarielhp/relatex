package assets

import (
	"math"
	"strings"
	"unicode"
)

// prime is the character of TeX's \prime.
const prime = "′"

// The math font's prime (decision 157): Latin Modern Math, the font the symbol font's stack names
// first, draws U+2032 as its script-size prime already raised. primeScript is the script scale of its
// MATH table (ScriptPercentScaleDown, 70%): the glyph at font size F is the script prime of size
// 0.7 F. primeCentre is the height of the glyph's ink centre above its baseline, in em (its ink runs
// from 0.430 to 0.748 em).
const (
	primeScript = 0.70
	primeCentre = 0.589
)

// raisePrimes writes each TeX prime as the character Unicode means by it (decisions 146 and 157).
// TeX's symbol font draws \prime as a full-size glyph that a formula sets at script size above the
// baseline (x' is x^{\prime}); the math font's U+2032 is that script prime already scaled and raised.
// A prime set from a TeX symbol font after an upright glyph whose baseline lies below it (its base: the
// nearest earlier glyph that is not a prime, to its left) is therefore written where TeX drew it: at
// the font size whose script size is the prime's own (its size over primeScript), at its own position
// along the line, on the baseline that puts the two glyphs' ink centres at one height. A prime with no
// such base, or whose outline is not read, keeps its own size and place. The renderings' comparison
// (decision 137) judges the result like any other text.
func raisePrimes(root *rxNode, glyphs []pictureGlyph) {
	paths := map[string]string{}
	collectPaths(root, paths)
	for index := range glyphs {
		glyph := &glyphs[index]
		if glyph.text != prime || !texSymbolFont(glyph.font) || !uniform(glyph.matrix) {
			continue
		}
		base := index - 1
		for base >= 0 && glyphs[base].text == prime {
			base--
		}
		if base < 0 || !uniform(glyphs[base].matrix) || glyphs[base].matrix[4] >= glyph.matrix[4] ||
			glyphs[base].matrix[5] <= glyph.matrix[5] {
			continue
		}
		href, _ := glyph.node.attribute("href")
		if href == "" {
			href, _ = glyph.node.attribute("xlink:href")
		}
		centre, ok := inkCentre(paths[strings.TrimPrefix(href, "#")])
		if !ok {
			continue
		}
		own := glyph.matrix[0]
		size := own / primeScript
		// SVG's y runs down: TeX's ink centre lies centre·own above its baseline, the math font's
		// primeCentre·size above the one written.
		y := glyph.matrix[5] - float64(centre*own) + float64(primeCentre*size)
		glyph.matrix = [6]float64{size, 0, 0, -size, glyph.matrix[4], y}
	}
}

// texSymbolFont: a PDF font that is TeX's math symbol font (cmsy, Latin Modern's lmsy), subset or not.
func texSymbolFont(font string) bool {
	lower := strings.ToLower(subsetTag.ReplaceAllString(font, ""))
	return strings.HasPrefix(lower, "cmsy") || strings.HasPrefix(lower, "lmsy") || strings.HasPrefix(lower, "lmmathsymbols")
}

// collectPaths maps each path element's id to its outline data (MuPDF's glyph definitions).
func collectPaths(node *rxNode, paths map[string]string) {
	for _, child := range node.children {
		if child.kind != "element" {
			continue
		}
		if localName(child.name) == "path" {
			id, hasID := child.attribute("id")
			data, hasData := child.attribute("d")
			if hasID && hasData {
				paths[id] = data
			}
		}
		collectPaths(child, paths)
	}
}

// inkCentre is the middle height of a glyph outline's points (em, y up), between its lowest and
// highest: an outline in absolute commands (M, L, C, Q, H, V, Z, as MuPDF writes them); false for
// another.
func inkCentre(data string) (float64, bool) {
	low, high := math.Inf(1), math.Inf(-1)
	command, count := byte(0), 0
	for _, token := range pathTokens(data) {
		if len(token) == 1 && unicode.IsLetter(rune(token[0])) {
			if !strings.Contains("MLCQHVZ", token) {
				return 0, false
			}
			command, count = token[0], 0
			continue
		}
		value := parseNumber(token)
		if math.IsNaN(value) || command == 0 || command == 'Z' {
			return 0, false
		}
		// H carries x values only, V y values only; the others alternate x and y.
		if command == 'V' || (command != 'H' && count%2 == 1) {
			low, high = min(low, value), max(high, value)
		}
		count++
	}
	if math.IsInf(low, 0) {
		return 0, false
	}
	return (low + high) / 2, true
}

// pathTokens splits path data into command letters and numbers (".5-.2" is ".5" and "-.2").
func pathTokens(data string) []string {
	tokens := []string{}
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for index := 0; index < len(data); index++ {
		char := data[index]
		switch {
		case char == ' ' || char == ',' || char == '\n' || char == '\t' || char == '\r':
			flush()
		case unicode.IsLetter(rune(char)) && char != 'e' && char != 'E':
			flush()
			tokens = append(tokens, string(char))
		case char == '-' && current.Len() > 0 && !strings.HasSuffix(current.String(), "e"):
			flush()
			current.WriteByte(char)
		case char == '.' && strings.Contains(current.String(), "."):
			flush()
			current.WriteByte(char)
		default:
			current.WriteByte(char)
		}
	}
	flush()
	return tokens
}
