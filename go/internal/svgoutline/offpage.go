package svgoutline

import (
	"math"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// A glyph set outside the page (an overfull Beamer frame whose last lines run below the slide,
// slides/0013, 0015, 0017) is painted, so the paint trace and the structured text keep it, but
// MuPDF culls it from the page SVG: it has no outline there, and none is needed, since it adds no
// ink to the page. Such a glyph binds to its paint with the status outside_page and no bounds, but
// only when its whole metric box lies outside the page's viewBox and the SVG has no use of its
// text at its placement; a glyph with an outline always binds to it.

const outsidePage = "outside_page"

// pageBox is the root's viewBox as x0, y0, x1, y1, or nil.
func pageBox(elements []*element) []float64 {
	if len(elements) == 0 || elements[0].name != "svg" {
		return nil
	}
	fields := strings.Fields(strings.ReplaceAll(elements[0].attrs["viewBox"], ",", " "))
	if len(fields) != 4 {
		return nil
	}
	values := make([]float64, 4)
	for index, field := range fields {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil || isNotFinite(value) {
			return nil
		}
		values[index] = value
	}
	if values[2] <= 0 || values[3] <= 0 {
		return nil
	}
	return []float64{values[0], values[1], values[0] + values[2], values[1] + values[3]}
}

func isNotFinite(value float64) bool { return math.IsNaN(value) || math.IsInf(value, 0) }

// offPage: the glyph's metric box lies wholly outside the page.
func (o *Outlines) offPage(glyph rec.Map) bool {
	if o.page == nil {
		return false
	}
	quad := rec.FetchList(glyph, "metric_quad")
	if len(quad) != 8 {
		return false
	}
	xs, ys := make([]float64, 0, 4), make([]float64, 0, 4)
	for index, value := range quad {
		number, ok := rec.Number(value)
		if !ok || isNotFinite(number) {
			return false
		}
		if index%2 == 0 {
			xs = append(xs, number)
		} else {
			ys = append(ys, number)
		}
	}
	page := o.page
	return maxOf(xs) <= page[0] || minOf(xs) >= page[2] || maxOf(ys) <= page[1] || minOf(ys) >= page[3]
}

// placed: some use of the paint's first character lies at its placement.
func (o *Outlines) placed(paint rec.Map) bool {
	drawn, replaced := paint["drawn_text"]
	if !replaced {
		drawn = rec.Fetch(paint, "text")
	}
	first := firstOf(text(drawn))
	matrix := rec.FetchList(paint, "matrix")
	for _, use := range o.Uses {
		if use["text"] == first && Close(rec.FetchList(use, "matrix"), matrix) {
			return true
		}
	}
	return false
}

// offPageBinding is the binding of a glyph culled outside the page, or nil when it is not one.
func (b *binder) offPageBinding(glyph, paint rec.Map) rec.Map {
	if !b.svg.offPage(glyph) || b.svg.placed(paint) {
		return nil
	}
	return rec.Map{"status": outsidePage, "paint": paint, "bounds": nil}
}

// validateOffPage checks an outside_page binding as validatePainted checks a measured one, without
// its use.
func (v *validator) validateOffPage(glyph, binding rec.Map) {
	paint := rec.FetchMap(binding, "paint")
	want := rec.Map{"status": outsidePage, "paint": paint, "bounds": nil, "origin_binding": binding["origin_binding"]}
	if !rec.Equal(binding, want) || !v.svg.offPage(glyph) || v.svg.placed(paint) {
		rec.Fail("Invalid outside-page glyph binding")
	}
	v.validateUnplaced(glyph, paint, "fill_text", rec.Fetch(binding, "origin_binding"))
}

// validateUnplaced checks the paint of a glyph bound without an SVG use: its identity, its
// operation and its origin.
func (v *validator) validateUnplaced(glyph, paint rec.Map, operation string, origin any) {
	v.validatePaint(paint)
	glyphText := spaceText(text(rec.Fetch(glyph, "text")))
	matrix := rec.FetchList(paint, "matrix")
	if paintFont(rec.Fetch(paint, "font")) != glyphFont(rec.Fetch(glyph, "font")) || spaceText(text(rec.Fetch(paint, "text"))) != glyphText ||
		glyphText == "" || rec.Fetch(paint, "operation") != operation || !Close(matrix[len(matrix)-2:], rec.FetchList(paint, "baseline")) {
		rec.Fail("SVG outline and native glyph identities differ")
	}
	v.validateOrigin(glyph, paint, origin)
}

func firstOf(value string) string {
	for _, character := range value {
		return string(character)
	}
	return ""
}

func minOf(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		result = min(result, value)
	}
	return result
}

func maxOf(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		result = max(result, value)
	}
	return result
}
