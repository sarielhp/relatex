package contrast

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// fillPrefix starts the background kind of text on an author's fill: "fill:#rrggbb".
const fillPrefix = "fill:"

// fillMinimumSP is the least height plus depth of a fill (4 pt): a thinner coloured rule is a
// line beside the text, not a box behind it.
const fillMinimumSP = 262144

// FillOf is the colour a span paints behind the spans after it (decision 099): a rule in its own
// #rrggbb colour (\colorbox, a listings or tcolorbox background, a beamer box) whose right margin
// takes back exactly its width, so the text after it is set on it, at least 4 pt high. "" for
// any other span.
func FillOf(span rec.Map) string {
	color, ok := span["color"].(string)
	if span["type"] != "rule" || !ok {
		return ""
	}
	if _, valid := Parse(color); !valid {
		return ""
	}
	width := sp(span, "width_sp")
	if width <= 0 || sp(span, "margin_right_sp") != -width || sp(span, "margin_left_sp") != 0 {
		return ""
	}
	if sp(span, "height_sp")+sp(span, "depth_sp") < fillMinimumSP {
		return ""
	}
	return color
}

// sp is a dimension of a span in scaled points, 0 when absent.
func sp(span rec.Map, key string) float64 {
	value, _ := rec.Number(span[key])
	return value
}

// Behind is, for each span of one content list, the fill painted behind it: that of the last
// fill rule before it in the list, when the span's middle lies within the fill's width; "" when
// there is none. The spans after a fill advance by their TeX widths (rules and images) or by an
// estimate of half an em per character (text), so text set past a small box (a section number's
// box, paper 1021) or after a frame's rules (paper 344) is not on it. A table's cells are lists
// of their own.
func Behind(content []any) []string { return BehindOn(content, "", nil) }

// BehindOn is Behind for a list set on base, a table cell's own fill (CellFill; "" for none), with
// the PDF's backdrops (decision 113): a span no fill rule of the list covers is on the fill the PDF
// paints behind its text (Backdrops.Of), else on the cell's fill.
func BehindOn(content []any, base string, backdrops Backdrops) []string {
	result := make([]string, len(content))
	fill, width, pen := "", 0.0, 0.0
	for index, value := range content {
		span := rec.AsMap(value)
		advance := advanceOf(span)
		result[index] = base
		if backdrop := backdrops.Of(span); backdrop != "" {
			result[index] = backdrop
		}
		if fill != "" && pen+float64(advance/2) <= width {
			result[index] = fill
		}
		if color := FillOf(span); color != "" {
			fill, width, pen = color, sp(span, "width_sp"), 0
			continue
		}
		pen += advance
	}
	return result
}

// emSP is the size of text without a recorded one (10 pt).
const emSP = 655360

// advanceOf is how far a span moves along its line, in scaled points: a rule's width and margins,
// an image's width, half an em for each character of a text, 0 for anything else.
func advanceOf(span rec.Map) float64 {
	switch span["type"] {
	case "rule":
		return sp(span, "width_sp") + sp(span, "margin_left_sp") + sp(span, "margin_right_sp")
	case "image":
		return sp(span, "width_sp")
	}
	text, ok := span["text"].(string)
	if !ok {
		return 0
	}
	size := float64(emSP)
	if style, styled := span["style"].(rec.Map); styled {
		if value, sized := rec.Number(style["size_sp"]); sized && value > 0 {
			size = value
		}
	}
	return float64(utf8.RuneCountInString(text)) * size / 2
}

// CellFill is the colour colortbl paints behind a rich table cell (\cellcolor, \rowcolor,
// \columncolor, \rowcolors: the grid cell's fill), "" for none.
func CellFill(cell rec.Map) string {
	fill, _ := cell["fill"].(string)
	if _, valid := Parse(fill); !valid {
		return ""
	}
	return fill
}

// eachList calls visit with a content list and with the content of every cell of its rich
// tables, nested tables too, as inline.Deep reads them, each with the cell fill it is set on
// (base): the cell's own, else that of the cell around its table ("" on the page).
func eachList(content []any, visit func(list []any, base string), depth int, base string) {
	if depth > 64 {
		return
	}
	visit(content, base)
	for _, value := range content {
		span := rec.AsMap(value)
		grid, _ := span["grid"].(rec.Map)
		if span["type"] != "table_grid" || grid["schema_version"] != tablemodel.Version {
			continue
		}
		for _, value := range rec.FetchList(grid, "cells") {
			cell := rec.AsMap(value)
			inner, _ := cell["content"].([]any)
			fill := CellFill(cell)
			if fill == "" {
				fill = base
			}
			eachList(inner, visit, depth+1, fill)
		}
	}
}

// FillKind is the background kind of text on fill.
func FillKind(fill string) string { return fillPrefix + fill }

// fillOfKind is the fill a kind names, if it names one.
func fillOfKind(kind string) (string, bool) {
	return strings.CutPrefix(kind, fillPrefix)
}

// PageKinds are kinds without the fills: the theme's own backgrounds.
func PageKinds(kinds []string) []string {
	result := []string{}
	for _, kind := range kinds {
		if _, isFill := fillOfKind(kind); !isFill {
			result = append(result, kind)
		}
	}
	return result
}

// FillsOf are the fills among kinds, sorted.
func FillsOf(kinds []string) []string {
	result := []string{}
	for _, kind := range kinds {
		if fill, isFill := fillOfKind(kind); isFill {
			result = append(result, fill)
		}
	}
	sort.Strings(result)
	return result
}

// LightensOn reports whether text on fill reads by growing lighter: white contrasts with the fill
// more than black. Otherwise it darkens. One of the two always reaches 4.5:1.
func LightensOn(fill RGB) bool {
	return Ratio(RGB{1, 1, 1}, fill) > Ratio(RGB{}, fill)
}

// The dark theme's counterpart of a light fill has OKLCH lightness darkFillBase plus darkFillSpan
// times the fill's distance from white: a near-white listing background becomes a box just above
// the dark page (#1b1d22, lightness 0.22), a mid-tone box a darker one of its hue.
const (
	darkFillBase = 0.24
	darkFillSpan = 0.6
)

// DarkFill is the fill as the dark theme paints it (decision 099): a fill on which text reads by
// growing lighter is dark already and stays; a light one becomes a dark box of its hue, so the
// dark theme's light inks read on it and on the page around it alike, wherever the browser
// wraps the text.
func DarkFill(fill RGB) RGB {
	if LightensOn(fill) {
		return fill
	}
	lightness, chroma, hue := fill.oklch()
	return fromOKLCH(darkFillBase+float64((1-lightness)*darkFillSpan), chroma, hue).quantized()
}

// OnDarkFill is ink as the dark theme shows it on fill: lightened, by the smallest OKLCH lightness
// change, until it reads on the fill's counterpart (DarkFill) and on the dark page.
func OnDarkFill(ink, fill RGB) RGB {
	page, _ := Parse(Dark.Backgrounds["page"])
	return Adjust(ink, []RGB{DarkFill(fill), page}, TextRatio, true)
}

// Canvas is the colour the PDF fills every page with before its text (decision 107: beamer's
// background canvas, \pagecolor), as the text paint proof records it, or "" for white paper. Text
// outside an author's fill is on the canvas, in every structure: the page paints it behind the
// text in both themes, and inks are shown against it.
func Canvas(document rec.Map) string {
	paint, _ := document["text_paint"].(rec.Map)
	canvas, _ := paint["canvas"].(string)
	if _, valid := Parse(canvas); !valid {
		return ""
	}
	return canvas
}

// Fills are the colours of the document's fills (FillOf), table cell fills (CellFill), its
// canvas (Canvas) and the PDF's backdrops behind text (decision 113), sorted: the page paints each in both themes and gives the PDF's own text
// (black) on it an ink.
func Fills(document rec.Map) []string {
	found := []string{}
	if canvas := Canvas(document); canvas != "" {
		found = append(found, canvas)
	}
	for _, fill := range DocumentBackdrops(document).fills() {
		if !slices.Contains(found, fill) {
			found = append(found, fill)
		}
	}
	for _, value := range rec.FetchMap(document, "nodes") {
		content, _ := rec.AsMap(value)["content"].([]any)
		eachList(content, func(list []any, base string) {
			if base != "" && !slices.Contains(found, base) {
				found = append(found, base)
			}
			for _, span := range list {
				if fill := FillOf(rec.AsMap(span)); fill != "" && !slices.Contains(found, fill) {
					found = append(found, fill)
				}
			}
		}, 0, "")
	}
	sort.Strings(found)
	return found
}
