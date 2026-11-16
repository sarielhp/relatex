package stylemodel

import (
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// Properties is TextStyle.properties.
func Properties(style rec.Map) rec.Map {
	Validate(style)
	shape := Shapes[rec.FetchString(style, "shape")]
	return rec.Map{"fontWeight": strconv.Itoa(Weights[rec.FetchString(style, "series")]), "fontStyle": shape[0],
		"fontVariantCaps": shape[1], "fontSize": Pixels(rec.FetchInt(style, "size_sp")),
		"fontFamily": rec.Fetch(style, "generic")}
}

// DefaultBase is the body text size assumed when no text run is styled: 10pt.
const DefaultBase int64 = 655_360

// Pixels is the CSS pixel size of a TeX size in scaled points.
func Pixels(sizeSP int64) float64 { return float64(sizeSP) * 96.0 / (65_536 * 72.27) }

// BaseSize is the document body text size in scaled points: the style size that sets the most
// characters of the nodes content, the smaller size on a tie, DefaultBase without styled text.
// Reading HTML sizes its text runs against it, so the body reads at the reader own text size.
func BaseSize(document rec.Map) int64 {
	characters := map[int64]int{}
	nodes, _ := document["nodes"].(rec.Map)
	for _, node := range nodes {
		content, _ := rec.AsMap(node)["content"].([]any)
		for _, value := range content {
			span := rec.AsMap(value)
			style, ok := span["style"].(rec.Map)
			if !ok {
				continue
			}
			text, _ := span["text"].(string)
			characters[rec.FetchInt(style, "size_sp")] += utf8.RuneCountInString(text)
		}
	}
	base, most := DefaultBase, -1
	for size, count := range characters {
		if count > most || (count == most && size < base) {
			base, most = size, count
		}
	}
	return base
}

// Size is the font-size declaration of a style: relative to the base (rem, so the reader text
// size and zoom apply) when base is positive, else the TeX size in pixels for pixel-exact boxes.
func Size(style rec.Map, base int64) string {
	if base > 0 {
		return fmt.Sprintf("%.6frem", float64(rec.FetchInt(style, "size_sp"))/float64(base))
	}
	return fmt.Sprintf("%.6fpx", Pixels(rec.FetchInt(style, "size_sp")))
}

// CSS is TextStyle.css with the font size expressed by Size.
func CSS(style rec.Map, base int64) string {
	values := Properties(style)
	family := rec.FetchString(style, "generic")
	if family == "unknown" {
		family = "inherit"
	}
	return fmt.Sprintf("font-weight:%s;font-style:%s;font-variant-caps:%s;font-size:%s;font-family:%s",
		values["fontWeight"], values["fontStyle"], values["fontVariantCaps"], Size(style, base), family)
}
