package paintmodel

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
)

// The text emission model of PDFTextPrecision: LuaTeX's horizontal TJ adjustments use a
// coarser raster than TeX sp. This models text emission only. Vertical positions and
// advances retain their existing bounds, rules have RuleToleranceBP; transformed, clipped
// or nonblack paint is still rejected.
const (
	PositionModel        = "luatex-horizontal-tj-1"
	coordinateTolerance  = 0.005
	RuleToleranceBP      = 0.01
	advanceToleranceBP   = 0.02
	RichToleranceBP      = 0.005
	RichAdvanceTolerance = 0.02
)

var paintID = regexp.MustCompile(`\A[gr]\d+\z`)

func spToBP(value any) float64 {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("expected a number, got %T", value)
	}
	return geometry.SPToBP(number)
}

func bpVector(values []any) []any {
	numbers := geometry.Numbers(values, len(values))
	result := make([]any, len(numbers))
	for index, number := range numbers {
		result[index] = geometry.SPToBP(number)
	}
	return result
}

// NativeGlyph is PDFTextPrecision.native_glyph: a native table glyph in bp, its text from
// the PDF font's slot encoding when it names an eight-bit slot.
func NativeGlyph(glyph rec.Map, encodings rec.Map, names rec.Map) rec.Map {
	text := rec.Fetch(glyph, "text")
	name := FontName(rec.FetchString(glyph, "font"))
	if slot, ok := glyph["slot"]; ok {
		number, isInt := slot.(int64)
		if !isInt || number < 0 || number > 255 {
			rec.Fail("Invalid native eight-bit font slot")
		}
		mapping, found := encodings[name].(rec.Map)
		if _, present := encodings[name]; !present || !found {
			rec.Fail("Missing PDF font encoding: %s", name)
		}
		value, present := mapping[strconv.FormatInt(number, 10)]
		if !present {
			rec.Fail("Missing PDF font slot: %s:%d", name, number)
		}
		text = value
	}
	if text == "[unmapped]" || text == "" {
		rec.Fail("Unmapped native table glyph")
	}
	font := any(name)
	if mapped, ok := names[name]; ok {
		font = mapped
	}
	return rec.Map{"text": text, "baseline": bpVector(geometry.Floats(geometry.Numbers(rec.FetchList(glyph, "baseline_sp"), 2))),
		"width": spToBP(rec.Fetch(glyph, "width_sp")), "size": spToBP(rec.Fetch(glyph, "size_sp")), "font": font}
}

// Number is a numeric field of a glyph or binding (m[key] - x in Ruby: a TypeError otherwise).
func Number(m rec.Map, key string) float64 {
	value, ok := rec.Number(rec.Fetch(m, key))
	if !ok {
		rec.Fail("undefined method '-' for %T", m[key])
	}
	return value
}

// Pair is a two-number vector field (a baseline or an origin).
func Pair(m rec.Map, key string) [2]float64 {
	values := rec.FetchList(m, key)
	if len(values) < 2 {
		rec.Fail("Invalid geometry vector")
	}
	a, okA := rec.Number(values[0])
	b, okB := rec.Number(values[1])
	if !okA || !okB {
		rec.Fail("Invalid geometry vector")
	}
	return [2]float64{a, b}
}

// PrecisionIdentity is whether two glyphs have the same text, font and size.
func PrecisionIdentity(expected, observed rec.Map) bool {
	return rec.Equal(expected["text"], observed["text"]) && rec.Equal(expected["font"], observed["font"]) &&
		math.Abs(Number(expected, "size")-Number(observed, "size")) <= coordinateTolerance
}

// PrecisionErrors is PDFTextPrecision.errors: the raw baseline error, and the residual once
// the backend's integer TJ raster (almost one 1/1000-em unit) is allowed for.
func PrecisionErrors(expected, observed rec.Map, origin [2]float64) (raw, residual []float64) {
	base := Pair(expected, "baseline")
	seen := Pair(observed, "baseline")
	target := [2]float64{base[0] + origin[0], base[1] + origin[1]}
	raw = []float64{math.Abs(target[0] - seen[0]), math.Abs(target[1] - seen[1])}
	quantum := Number(expected, "size") / 1000.0
	return raw, []float64{math.Max(raw[0]-quantum, 0.0), raw[1]}
}

// PrecisionMatch is PDFTextPrecision.match?: the same glyph within the advance and baseline
// tolerances once the TJ raster is allowed for.
func PrecisionMatch(expected, observed rec.Map, origin [2]float64) bool {
	if !PrecisionIdentity(expected, observed) || math.Abs(Number(expected, "width")-Number(observed, "width")) > advanceToleranceBP {
		return false
	}
	_, residual := PrecisionErrors(expected, observed, origin)
	return residual[0] <= coordinateTolerance && residual[1] <= coordinateTolerance
}

// validateFragment is PDFTextPrecision.validate_fragment!.
func validateFragment(native, observed rec.Map, encodings, names rec.Map) {
	if rec.Fetch(observed, "position_model") != PositionModel {
		rec.Fail("Unknown table text emission model")
	}
	originValues := geometry.Numbers(rec.FetchList(observed, "origin_bp"), 2)
	origin := [2]float64{originValues[0], originValues[1]}
	raw, residual := []float64{}, []float64{}
	glyphs := rec.FetchList(native, "glyphs")
	for index, binding := range bindings(glyphs, rec.FetchList(observed, "glyph_bindings"), "g") {
		expected := NativeGlyph(rec.AsMap(glyphs[index]), encodings, names)
		paint := rec.FetchMap(binding, "pdf")
		geometry.Numbers(rec.FetchList(paint, "baseline"), 2)
		geometry.Numbers([]any{rec.Fetch(paint, "width"), rec.Fetch(paint, "size")}, 2)
		if rec.Fetch(paint, "safe") != true || !PrecisionMatch(expected, paint, origin) {
			rec.Fail("Native/PDF text precision binding differs")
		}
		r, s := PrecisionErrors(expected, paint, origin)
		raw, residual = append(raw, r...), append(residual, s...)
	}
	rules := rec.FetchList(native, "rules")
	for index, binding := range bindings(rules, rec.FetchList(observed, "rule_bindings"), "r") {
		target := Translated(NativeRect(rec.AsMap(rules[index])), origin)
		paint := rec.FetchMap(binding, "pdf")
		rect := geometry.Numbers(rec.FetchList(paint, "rect"), 4)
		errors := make([]float64, 4)
		within := rec.Fetch(paint, "safe") == true
		for axis := range 4 {
			errors[axis] = math.Abs(target[axis] - rect[axis])
			within = within && errors[axis] <= RuleToleranceBP
		}
		if !within {
			rec.Fail("Native/PDF table rule binding differs")
		}
		raw, residual = append(raw, errors...), append(residual, errors...)
	}
	expected := []any{Maximum(raw), Maximum(residual)}
	actual := []any{observed["maximum_error_bp"], observed["maximum_residual_bp"]}
	for index := range actual {
		value, ok := rec.Number(actual[index])
		if !ok || math.IsInf(value, 0) || math.IsNaN(value) || !rec.Equal(actual[index], expected[index]) {
			rec.Fail("Table paint precision summary differs from bindings")
		}
	}
}

// NativeRect is a native rule's rect_sp in bp, [x0, y0, x1, y1].
func NativeRect(rule rec.Map) []float64 {
	values := geometry.Numbers(rec.FetchList(rule, "rect_sp"), 4)
	x, y := geometry.SPToBP(values[0]), geometry.SPToBP(values[1])
	width, height := geometry.SPToBP(values[2]), geometry.SPToBP(values[3])
	return []float64{x, y, x + width, y + height}
}

// Maximum is values.max || 0.0.
func Maximum(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

// bindings is PDFTextPrecision.bindings!: the observed bindings, one per native atom in
// order, with distinct well-formed paint ids.
func bindings(native, observed []any, prefix string) []rec.Map {
	result := make([]rec.Map, len(observed))
	ids := []string{}
	for index, value := range observed {
		binding := rec.AsMap(value)
		if !rec.Equal(rec.Fetch(binding, "native"), int64(index)) {
			rec.Fail("Native/PDF binding coverage differs")
		}
		result[index] = binding
	}
	if len(observed) != len(native) {
		rec.Fail("Native/PDF binding coverage differs")
	}
	for _, binding := range result {
		id, _ := rec.Fetch(rec.FetchMap(binding, "pdf"), "id").(string)
		for _, seen := range ids {
			if seen == id {
				rec.Fail("Duplicate or malformed table paint binding")
			}
		}
		if !paintID.MatchString(id) || !strings.HasPrefix(id, prefix) {
			rec.Fail("Duplicate or malformed table paint binding")
		}
		ids = append(ids, id)
	}
	return result
}

// Translated is rect [x0, y0, x1, y1] moved by origin.
func Translated(rect []float64, origin [2]float64) []float64 {
	result := make([]float64, len(rect))
	for index, value := range rect {
		result[index] = value + origin[index%2]
	}
	return result
}
