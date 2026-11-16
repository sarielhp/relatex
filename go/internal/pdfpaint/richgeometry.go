package pdfpaint

import (
	"math"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// richGeometry is PDFRichTableGeometry with the luatex-horizontal-tj-1 position model (the
// only one the capture uses): a single translation binds all table fragments on a page to
// independent PDF glyphs and rules. Repeated or identical neighboring tables cannot borrow
// ink.
type richGeometry struct {
	fragments []rec.Map
	inventory *inventory
	byText    *omap.Map[[2]any, []rec.Map]
	glyphs    []rec.Map
	rules     []rec.Map
}

func newRichGeometry(fragments []rec.Map, paint *inventory, encodings, names rec.Map) *richGeometry {
	g := &richGeometry{fragments: fragments, inventory: paint, byText: omap.New[[2]any, []rec.Map]()}
	for _, glyph := range paint.glyphs {
		key := [2]any{glyph["text"], glyph["font"]}
		list, _ := g.byText.Get(key)
		g.byText.Set(key, append(list, glyph))
	}
	for _, entry := range fragments {
		fragment := rec.FetchMap(entry, "fragment")
		for _, glyph := range rec.FetchList(fragment, "glyphs") {
			native := paintmodel.NativeGlyph(rec.AsMap(glyph), encodings, names)
			native["table"] = rec.Fetch(entry, "id")
			g.glyphs = append(g.glyphs, native)
		}
	}
	for _, entry := range fragments {
		for _, rule := range rec.FetchList(rec.FetchMap(entry, "fragment"), "rules") {
			rect := paintmodel.NativeRect(rec.AsMap(rule))
			g.rules = append(g.rules, rec.Map{"table": rec.Fetch(entry, "id"), "rect": rect})
		}
	}
	return g
}

func (g *richGeometry) paintFor(glyph rec.Map) []rec.Map {
	list, _ := g.byText.Get([2]any{glyph["text"], glyph["font"]})
	return list
}

func (g *richGeometry) verify() rec.Map {
	if len(g.glyphs) == 0 && len(g.rules) == 0 {
		rec.Fail("Native table fragments contain no visible paint")
	}
	var found rec.Map
	matches := 0
	for _, origin := range g.origins() {
		if result, ok := g.tryMatch(origin); ok {
			found = result
			matches++
		}
	}
	if matches != 1 {
		rec.Fail("PDF table paint has missing or ambiguous native geometry")
	}
	return found
}

// tryMatch is match(origin) rescue Failure nil.
func (g *richGeometry) tryMatch(origin [2]float64) (result rec.Map, ok bool) {
	defer rescueFailure(&ok)
	return g.match(origin), true
}

// rescueFailure turns a rec.Failure panic into ok = false.
func rescueFailure(ok *bool) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if _, isFailure := recovered.(rec.Failure); !isFailure {
		panic(recovered)
	}
	*ok = false
}

func (g *richGeometry) origins() [][2]float64 {
	result := [][2]float64{}
	add := func(origin [2]float64) {
		for _, seen := range result {
			if seen == origin {
				return
			}
		}
		result = append(result, origin)
	}
	if len(g.glyphs) > 0 {
		anchor := g.glyphs[0]
		for _, glyph := range g.glyphs[1:] {
			if len(g.paintFor(glyph)) < len(g.paintFor(anchor)) {
				anchor = glyph
			}
		}
		base := paintmodel.Pair(anchor, "baseline")
		for _, glyph := range g.paintFor(anchor) {
			if paintmodel.PrecisionIdentity(anchor, glyph) {
				seen := paintmodel.Pair(glyph, "baseline")
				add([2]float64{seen[0] - base[0], seen[1] - base[1]})
			}
		}
		return result
	}
	anchor := g.rules[0]["rect"].([]float64)
	for _, rule := range g.inventory.rules {
		rect := floatList(rule["rect"])
		if sameExtent(anchor, rect) {
			add([2]float64{rect[0] - anchor[0], rect[1] - anchor[1]})
		}
	}
	return result
}

func floatList(value any) []float64 {
	if list, ok := value.([]float64); ok {
		return list
	}
	return geometry.Numbers(rec.AsList(value), len(rec.AsList(value)))
}

func sameExtent(first, second []float64) bool {
	return math.Abs((first[2]-first[0])-(second[2]-second[0])) <= paintmodel.RichToleranceBP &&
		math.Abs((first[3]-first[1])-(second[3]-second[1])) <= paintmodel.RichToleranceBP
}

type binding struct {
	native   int
	pdf      rec.Map
	raw      []float64
	residual []float64
}

func (g *richGeometry) match(origin [2]float64) rec.Map {
	usedGlyphs, usedRules := map[any]bool{}, map[any]bool{}
	glyphBindings, ruleBindings := map[any][]binding{}, map[any][]binding{}
	for _, glyph := range g.glyphs {
		g.matchGlyph(glyph, origin, usedGlyphs, glyphBindings)
	}
	for _, rule := range g.rules {
		g.matchRule(rule, origin, usedRules, ruleBindings)
	}
	g.verifyExclusiveCoverage(origin, usedGlyphs, usedRules)
	return g.precisionGeometry(origin, glyphBindings, ruleBindings)
}

func (g *richGeometry) matchGlyph(glyph rec.Map, origin [2]float64, used map[any]bool, bindings map[any][]binding) {
	var matched rec.Map
	count := 0
	for _, observed := range g.paintFor(glyph) {
		if !used[observed["id"]] && paintmodel.PrecisionIdentity(glyph, observed) && paintmodel.PrecisionMatch(glyph, observed, origin) &&
			math.Abs(paintmodel.Number(glyph, "width")-paintmodel.Number(observed, "width")) <= paintmodel.RichAdvanceTolerance {
			matched = observed
			count++
		}
	}
	if count != 1 {
		rec.Fail("Native table glyph has no unique PDF binding")
	}
	if matched["safe"] != true {
		rec.Fail("Unsupported table text paint or clipping")
	}
	used[matched["id"]] = true
	raw, residual := paintmodel.PrecisionErrors(glyph, matched, origin)
	table := glyph["table"]
	bindings[table] = append(bindings[table], binding{native: len(bindings[table]), pdf: matched, raw: raw, residual: residual})
}

func within(first, second []float64, tolerance float64) bool {
	for index := range first {
		if math.Abs(first[index]-second[index]) > tolerance {
			return false
		}
	}
	return true
}

func (g *richGeometry) matchRule(rule rec.Map, origin [2]float64, used map[any]bool, bindings map[any][]binding) {
	target := paintmodel.Translated(rule["rect"].([]float64), origin)
	matches := []rec.Map{}
	for _, observed := range g.inventory.rules {
		if !used[observed["id"]] && within(target, floatList(observed["rect"]), paintmodel.RuleToleranceBP) {
			matches = append(matches, observed)
		}
	}
	if len(matches) != 1 && !g.coincidentRules(rule, target, matches) {
		rec.Fail("Native table rule has no unique PDF binding")
	}
	matched := matches[0]
	if matched["safe"] != true {
		rec.Fail("Unsupported table rule paint or clipping")
	}
	used[matched["id"]] = true
	rect := floatList(matched["rect"])
	errors := make([]float64, len(target))
	for index := range target {
		errors[index] = math.Abs(target[index] - rect[index])
	}
	table := rule["table"]
	bindings[table] = append(bindings[table], binding{native: len(bindings[table]), pdf: matched, raw: errors, residual: errors})
}

// coincidentRules: longtable deliberately paints a pair of coincident rules. Their ordered
// occurrences can bind only when the entire equal-paint group has the same native table
// owner and exact cardinality.
func (g *richGeometry) coincidentRules(rule rec.Map, target []float64, remaining []rec.Map) bool {
	if len(remaining) <= 1 {
		return false
	}
	rect := rule["rect"].([]float64)
	native := 0
	for _, candidate := range g.rules {
		if !within(candidate["rect"].([]float64), rect, paintmodel.RichToleranceBP) {
			continue
		}
		native++
		if candidate["table"] != rule["table"] || !floatsEqual(candidate["rect"].([]float64), rect) {
			return false
		}
	}
	if native <= 1 {
		return false
	}
	observed := []rec.Map{}
	for _, candidate := range g.inventory.rules {
		if within(target, floatList(candidate["rect"]), paintmodel.RuleToleranceBP) {
			observed = append(observed, candidate)
		}
	}
	if len(observed) != native {
		return false
	}
	for _, candidate := range observed[1:] {
		if !floatsEqual(floatList(candidate["rect"]), floatList(observed[0]["rect"])) {
			return false
		}
	}
	return true
}

func (g *richGeometry) precisionGeometry(origin [2]float64, glyphs, rules map[any][]binding) rec.Map {
	result := rec.Map{}
	for _, entry := range g.fragments {
		id := rec.Fetch(entry, "id")
		all := append(append([]binding{}, glyphs[id]...), rules[id]...)
		raw, residual := []float64{}, []float64{}
		for _, item := range all {
			raw, residual = append(raw, item.raw...), append(residual, item.residual...)
		}
		result[rec.AsString(id)] = rec.Map{"glyphs": int64(len(glyphs[id])), "rules": int64(len(rules[id])),
			"origin_bp": []any{origin[0], origin[1]}, "position_model": paintmodel.PositionModel,
			"maximum_error_bp": paintmodel.Maximum(raw), "maximum_residual_bp": paintmodel.Maximum(residual),
			"glyph_bindings": bindingList(glyphs[id]), "rule_bindings": bindingList(rules[id])}
	}
	return result
}

func bindingList(items []binding) []any {
	result := make([]any, len(items))
	for index, item := range items {
		result[index] = rec.Map{"native": int64(item.native), "pdf": item.pdf}
	}
	return result
}

func (g *richGeometry) regions(origin [2]float64) [][]float64 {
	result := [][]float64{}
	for _, entry := range g.fragments {
		for _, value := range rec.FetchList(rec.FetchMap(entry, "fragment"), "rows") {
			row := rec.AsMap(value)
			start := geometry.Numbers(rec.FetchList(row, "origin_sp"), 2)
			height := paintmodel.Number(row, "height") + paintmodel.Number(row, "depth")
			for _, item := range rec.FetchList(row, "cells") {
				cell := rec.AsMap(item)
				left := start[0] + paintmodel.Number(cell, "left")
				box := geometry.Box([]any{left, start[1], left + paintmodel.Number(cell, "width"), start[1] + height})
				for index := range box {
					box[index] = geometry.SPToBP(box[index])
				}
				result = append(result, paintmodel.Translated(box, origin))
			}
		}
	}
	return result
}

func inside(p [2]float64, rect []float64) bool {
	return p[0] >= rect[0]-paintmodel.RichToleranceBP && p[0] <= rect[2]+paintmodel.RichToleranceBP &&
		p[1] >= rect[1]-paintmodel.RichToleranceBP && p[1] <= rect[3]+paintmodel.RichToleranceBP
}

func intersects(first, second []float64) bool {
	return first[0] < second[2]-paintmodel.RichToleranceBP && first[2] > second[0]+paintmodel.RichToleranceBP &&
		first[1] < second[3]-paintmodel.RichToleranceBP && first[3] > second[1]+paintmodel.RichToleranceBP
}

func (g *richGeometry) verifyExclusiveCoverage(origin [2]float64, glyphs, rules map[any]bool) {
	bounds := g.regions(origin)
	hit := func(test func([]float64) bool) bool {
		for _, rect := range bounds {
			if test(rect) {
				return true
			}
		}
		return false
	}
	for _, glyph := range g.inventory.glyphs {
		if !glyphs[glyph["id"]] && hit(func(rect []float64) bool { return inside(paintmodel.Pair(glyph, "baseline"), rect) }) {
			rec.Fail("Unmodeled PDF text in table cells")
		}
	}
	for _, rule := range g.inventory.rules {
		if !rules[rule["id"]] && hit(func(rect []float64) bool { return intersects(floatList(rule["rect"]), rect) }) {
			rec.Fail("Unmodeled PDF rule in table cells")
		}
	}
	for _, paint := range g.inventory.other {
		if hit(func(rect []float64) bool { return intersects(floatList(paint["rect"]), rect) }) {
			rec.Fail("Unsupported PDF image/path in table cells")
		}
	}
}
