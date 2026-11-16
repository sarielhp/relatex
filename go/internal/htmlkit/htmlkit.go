// Package htmlkit is the HTML the text outputs share (an output library, decision 102): the
// escaping, the stylesheet pieces and the painted-rule box that both the page (html) and the
// Markdown (raw HTML inside it) write. It reads the document model only.
package htmlkit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// LinkCSS is PaperTableHTML::LINK_CSS.
const LinkCSS = `table[data-schema="table-2"] a{color:inherit}`

var escaper = strings.NewReplacer("'", "&#39;", "&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")

// fontFormats is SourceFonts::FORMATS.
var fontFormats = map[string]string{"truetype": "truetype", "opentype": "opentype"}

// Escape is CGI.escapeHTML.
func Escape(text string) string { return escaper.Replace(text) }

// FontItems are the document's font records (fonts.items), none when it has no fonts.
func FontItems(document rec.Map) []rec.Map {
	fonts, ok := document["fonts"].(rec.Map)
	if !ok {
		return nil
	}
	items, _ := fonts["items"].([]any)
	result := []rec.Map{}
	for _, item := range items {
		result = append(result, rec.AsMap(item))
	}
	return result
}

// SourceFontsCSS is SourceFonts.css: a @font-face per linked font family, its file in fonts/.
func SourceFontsCSS(document rec.Map) string {
	var builder strings.Builder
	seen := map[any]bool{}
	for _, font := range FontItems(document) {
		if file, linked := font["file"]; !linked || file == nil || file == false {
			continue
		}
		family := rec.Fetch(font, "css_family")
		if seen[family] {
			continue
		}
		seen[family] = true
		file := rec.FetchMap(font, "file")
		format, ok := fontFormats[rec.FetchString(file, "format")]
		if !ok {
			panic(rec.Failure{Message: "key not found: " + inspect(rec.FetchString(file, "format")), Lookup: true})
		}
		builder.WriteString("@font-face{font-family:\"" + rec.ToS(family) + "\";src:url(\"fonts/" + rec.ToS(rec.Fetch(file, "name")) +
			"\") format(\"" + format + "\");font-display:swap;}")
	}
	return builder.String()
}

// RuleCSS is PaperHTML.rule_css: a painted rule is a filled box of its TeX size standing on
// the baseline at its depth.
func RuleCSS(span rec.Map) string {
	color := "currentColor"
	if value, ok := span["color"]; ok {
		color = rec.ToS(value)
	}
	css := "display:inline-block;width:" + Pixels(rec.Fetch(span, "width_sp")) + "px;" +
		"height:" + Pixels(add(rec.Fetch(span, "height_sp"), rec.Fetch(span, "depth_sp"))) + "px;" +
		"vertical-align:" + Pixels(mul(rec.Fetch(span, "depth_sp"), int64(-1))) + "px;background:" + color
	if _, ok := span["margin_left_sp"]; !ok {
		return css
	}
	return css + ";margin:0 " + Pixels(rec.Fetch(span, "margin_right_sp")) + "px 0 " + Pixels(rec.Fetch(span, "margin_left_sp")) + "px"
}

// Pixels is sp * 96.0 / (65_536 * 72.27) as format('%.6f'): TeX scaled points as CSS pixels.
func Pixels(sp any) string {
	value, _ := mul(sp, 96.0).(float64)
	return strconv.FormatFloat(value/(65_536*texPoints()), 'f', 6, 64)
}

func texPoints() float64 { return 72.27 }

// inspect is String#inspect for the identifiers in lookup messages.
func inspect(text string) string {
	encoded, err := canonjson.Marshal(text)
	if err != nil {
		return text
	}
	return string(encoded)
}

// num normalizes a JSON number to Ruby's Integer (int64) or Float (float64).
func num(value any) any {
	switch typed := value.(type) {
	case int64:
		return typed
	case float64:
		return typed
	case canonjson.Float:
		return float64(typed)
	case int:
		return int64(typed)
	}
	panic(fmt.Errorf("expected a number, got %T", value))
}

func float(value any) float64 {
	number, ok := rec.Number(num(value))
	if !ok {
		panic(fmt.Errorf("expected a number, got %T", value))
	}
	return number
}

// add and mul are Ruby's + and *: Integer stays Integer, anything with a Float is a Float.
func add(a, b any) any {
	x, okX := num(a).(int64)
	y, okY := num(b).(int64)
	if okX && okY {
		return x + y
	}
	return float(a) + float(b)
}

func mul(a, b any) any {
	x, okX := num(a).(int64)
	y, okY := num(b).(int64)
	if okX && okY {
		return x * y
	}
	return float(a) * float(b)
}
