// Package stylemodel is the document model's observed text style (TextStyle::FIELDS and
// TextStyle.validate!): the NFSS declaration document.json records for a text font, and the series
// and shapes it accepts. The core (textstyle) reads styles from the records; the model's readers
// validate them here (decision 102).
package stylemodel

import (
	"slices"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// Fields is TextStyle::FIELDS.
var Fields = []string{"source", "font_id", "font_name", "size_sp", "encoding", "family", "series", "shape", "generic"}

// Weights are the NFSS series and fontawesome5's own series names (regular, solid), at the weights
// of the Font Awesome 5 faces. A series may carry a width after its weight (bc: bold condensed,
// c: medium condensed, sbc, mx; decision 104: DejaVu's and TeX Gyre's condensed faces); the width
// changes no weight.
var Weights = withWidths(map[string]int{"m": 400, "b": 700, "bx": 700, "sb": 600, "db": 600, "mb": 500,
	"eb": 800, "ub": 900, "l": 300, "el": 200, "ul": 100, "regular": 400, "solid": 900})

// withWidths adds each NFSS weight's condensed and extended series (c, x; medium's alone or as mc, mx).
func withWidths(weights map[string]int) map[string]int {
	for _, name := range []string{"m", "b", "sb", "db", "mb", "eb", "ub", "l", "el", "ul"} {
		for _, width := range []string{"c", "x"} {
			if _, taken := weights[name+width]; !taken {
				weights[name+width] = weights[name]
			}
		}
	}
	weights["c"], weights["x"] = 400, 400
	return weights
}

// slant: TeX slants lean about 9.5 degrees (1/6). Chromium reports a bare oblique as italic,
// so the angle is explicit.
const slant = "oblique 10deg"

// Shapes are the NFSS shapes and mathdesign's faked small caps (fsc, its \scdefault without the
// expert fonts) with their italic (fic), as CSS font-style and font-variant-caps; the upright italic
// (ui, u), outline (ol) and Latin Modern's inline (In, Isl) faces read upright or slanted.
var Shapes = map[string][2]string{"n": {"normal", "normal"}, "it": {"italic", "normal"}, "sl": {slant, "normal"},
	"sc": {"normal", "small-caps"}, "scit": {"italic", "small-caps"}, "scsl": {slant, "small-caps"},
	"fsc": {"normal", "small-caps"}, "fic": {"italic", "small-caps"}, "ui": {"normal", "normal"}, "u": {"normal", "normal"},
	"ol": {"normal", "normal"}, "In": {"normal", "normal"}, "Isl": {slant, "normal"}}

// Validate is TextStyle.validate!.
func Validate(value any) rec.Map {
	style, ok := value.(rec.Map)
	valid := ok && rec.SameKeys(style, Fields) && slices.Contains([]any{"nfss-font-id", "unobserved-tex-font-id"}, style["source"])
	for _, key := range []string{"font_id", "size_sp"} {
		number, isInt := style[key].(int64)
		valid = valid && isInt && number >= 1 && number <= 1_073_741_823
	}
	for _, key := range []string{"font_name", "encoding", "family", "series", "shape"} {
		text, isString := style[key].(string)
		size := utf8.RuneCountInString(text)
		valid = valid && isString && size >= 1 && size <= 256
	}
	if !valid {
		rec.Fail("Invalid observed text font")
	}
	if _, ok := Weights[style["series"].(string)]; !ok {
		rec.Fail("Unsupported observed text style (series %v, shape %v)", style["series"], style["shape"])
	}
	if _, ok := Shapes[style["shape"].(string)]; !ok {
		rec.Fail("Unsupported observed text style (series %v, shape %v)", style["series"], style["shape"])
	}
	if !slices.Contains([]any{"serif", "sans-serif", "monospace", "unknown"}, style["generic"]) {
		rec.Fail("Invalid generic font family")
	}
	return style
}
