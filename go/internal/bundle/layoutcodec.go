package bundle

import (
	"bytes"
	"maps"
	"runtime"
	"sync"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/svgoutline"
)

// The bundle stores a layout without the glyph fields its page SVGs already hold (decision
// 085). A measured glyph's outline binding is its paint, its origin binding and a copy of one
// SVG use (use, path, text, matrix, bounds, status), and its ink quad is that use's bounds at
// the glyph's baseline: the stored binding keeps the use index, the paint and the origin, and
// the ink quad is left out. A field is left out only when the SVG gives it back exactly, so
// reading restores the layout byte for byte and every check runs on the whole record. On a
// 303-page book (Lebl's Basic Analysis) the copies were 158 MB of a 562 MB layout.

// expandLayout restores what the bundle left out (compactGlyph, shareLayoutSVG); a layout stored whole is returned as read.
func expandLayout(layout any) any {
	return mapLayoutPages(unshareLayoutSVG(layout), expandGlyph)
}

type glyphCodec func(glyph rec.Map, uses []rec.Map) rec.Map

// mapLayoutPages applies codec to every glyph of every page with an SVG, pages in parallel.
func mapLayoutPages(layout any, codec glyphCodec) any {
	source, ok := layout.(rec.Map)
	if !ok {
		return layout
	}
	pages, ok := source["pages"].([]any)
	if !ok {
		return layout
	}
	result := maps.Clone(source)
	mapped := make([]any, len(pages))
	failures := make([]any, len(pages))
	work := make(chan int)
	var group sync.WaitGroup
	for range max(1, runtime.GOMAXPROCS(0)) {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range work {
				mapped[index], failures[index] = mapPage(pages[index], codec)
			}
		}()
	}
	for index := range pages {
		work <- index
	}
	close(work)
	group.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
	result["pages"] = mapped
	return result
}

func mapPage(value any, codec glyphCodec) (mapped any, failure any) {
	defer func() {
		if recovered := recover(); recovered != nil {
			failure = recovered
		}
	}()
	page, ok := value.(rec.Map)
	if !ok {
		return value, nil
	}
	glyphs, hasGlyphs := page["glyphs"].([]any)
	text, hasSVG := page["svg"].(string)
	if !hasGlyphs || !hasSVG {
		return value, nil
	}
	uses := svgoutline.For(decodeSVG(text)).Uses
	result := maps.Clone(page)
	coded := make([]any, len(glyphs))
	for index, glyph := range glyphs {
		coded[index] = glyph
		if record, isMap := glyph.(rec.Map); isMap {
			coded[index] = codec(record, uses)
		}
	}
	result["glyphs"] = coded
	return result, nil
}

// compactGlyph leaves out of a glyph (and of its Unicode components) the binding fields its
// SVG use holds and an ink quad equal to the use's bounds at the baseline.
func compactGlyph(glyph rec.Map, uses []rec.Map) rec.Map {
	binding, bound := glyph["outline_binding"].(rec.Map)
	baseline, placed := glyph["baseline"].([]any)
	if !bound || !placed {
		return glyph
	}
	result := maps.Clone(glyph)
	if rec.Equal(glyph["ink_quad"], svgoutline.Quad(binding["bounds"], baseline)) {
		delete(result, "ink_quad")
	}
	_, painted := binding["paint"]
	_, origin := binding["origin_binding"]
	if native := bindingUse(binding, uses); painted && origin && native != nil && rec.Equal(withoutPaint(binding), native) {
		result["outline_binding"] = rec.Map{"use": binding["use"], "paint": binding["paint"], "origin_binding": binding["origin_binding"]}
	}
	codeComponents(result, uses, compactGlyph)
	// Equal values can still serialize differently (an integer for a float): a glyph is stored
	// compact only when reading gives back its exact bytes.
	if !bytes.Equal(marshal(expandGlyph(result, uses)), marshal(glyph)) {
		return glyph
	}
	return result
}

// expandGlyph restores the fields compactGlyph left out.
func expandGlyph(glyph rec.Map, uses []rec.Map) rec.Map {
	binding, bound := glyph["outline_binding"].(rec.Map)
	baseline, placed := glyph["baseline"].([]any)
	if !bound || !placed {
		return glyph
	}
	result := maps.Clone(glyph)
	if _, whole := binding["status"]; !whole {
		if native := bindingUse(binding, uses); native != nil {
			restored := maps.Clone(native)
			restored["paint"] = binding["paint"]
			restored["origin_binding"] = binding["origin_binding"]
			binding = restored
			result["outline_binding"] = restored
		}
	}
	if _, stored := result["ink_quad"]; !stored {
		result["ink_quad"] = svgoutline.Quad(binding["bounds"], baseline)
	}
	codeComponents(result, uses, expandGlyph)
	return result
}

func codeComponents(glyph rec.Map, uses []rec.Map, codec glyphCodec) {
	parts, ok := glyph["unicode_components"].([]any)
	if !ok {
		return
	}
	coded := make([]any, len(parts))
	for index, part := range parts {
		coded[index] = part
		if record, isMap := part.(rec.Map); isMap {
			coded[index] = codec(record, uses)
		}
	}
	glyph["unicode_components"] = coded
}

// bindingUse is the SVG use a painted binding names, or nil.
func bindingUse(binding rec.Map, uses []rec.Map) rec.Map {
	if binding["status"] == "synthetic_space" {
		return nil
	}
	index, ok := binding["use"].(int64)
	if !ok || index < 0 || index >= int64(len(uses)) {
		return nil
	}
	return uses[index]
}

func withoutPaint(binding rec.Map) rec.Map {
	return without(binding, "paint", "origin_binding")
}
