package accessibility

import (
	"regexp"
	"strconv"

	"github.com/sarielhp/relatex/internal/gaplocate"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
)

// placed adds where an item lies in the author's source (decision 184): the file and line of the
// caption of the float that holds it (or of the first figure inside a table), the float's kind, its
// caption's number and label, and for a table its position among the float's tables. Only what is
// known is added.
func placed(item rec.Map, place gaplocate.Place) rec.Map {
	if place.File != "" {
		item["file"], item["line"] = place.File, int64(place.Line)
	}
	if place.Float != "" {
		item["float"] = place.Float
	}
	if place.Number != "" {
		item["number"] = place.Number
	}
	if place.Label != "" {
		item["label"] = place.Label
	}
	if place.Index > 0 {
		item["table"], item["of"] = int64(place.Index), int64(place.Of)
	}
	if place.Caption != "" {
		item["caption"] = clip(place.Caption, 80)
	}
	return item
}

// The structural checks that read the page as written (decision 088): its formulas' alttext and
// its heading levels, and the tables' header cells by the rule the page follows.

var (
	mathElement = regexp.MustCompile(`<math\b[^>]*>`)
	mathAltText = regexp.MustCompile(`\balttext=(?:"[^"]+"|'[^']+')`)
	mathID      = regexp.MustCompile(`\bid=(?:"([^"]*)"|'([^']*)')`)
	// headingOpener: a heading element, or a run-in title's span with role heading (decision 088 amendment 1).
	headingOpener = regexp.MustCompile(`<h([1-6])\b[^>]*>|<span class="runin-heading" role="heading" aria-level="([1-6])">`)
)

// formulaItems: the page's <math> elements without a text alternative (alttext).
func formulaItems(page []byte) []any {
	return unmarkedItems(page, mathElement, mathAltText)
}

// unmarkedItems: the tags opener finds in the page that marked does not match, each with its id.
func unmarkedItems(page []byte, opener, marked *regexp.Regexp) []any {
	items := []any{}
	for _, tag := range opener.FindAll(page, -1) {
		if marked.Match(tag) {
			continue
		}
		id := ""
		if match := mathID.FindSubmatch(tag); match != nil {
			id = string(match[1]) + string(match[2])
		}
		items = append(items, rec.Map{"id": id})
	}
	return items
}

// headingItems: a heading of the page more than one level below the heading before it. The first
// heading may have any level: a page without a title starts at h2.
func headingItems(page []byte) []any {
	items := []any{}
	previous := int64(0)
	for _, match := range headingOpener.FindAllSubmatch(page, -1) {
		level, _ := strconv.ParseInt(string(match[1])+string(match[2]), 10, 64)
		if previous > 0 && level > previous+1 {
			id := ""
			if found := mathID.FindSubmatch(match[0]); found != nil {
				id = string(found[1]) + string(found[2])
			}
			items = append(items, rec.Map{"id": id, "level": level, "previous": previous})
		}
		previous = level
	}
	return items
}

// tableItems: data tables without header cells (column or row headers, decision 131); a layout
// table needs none. A table of one row or one column has no header to relate its data to and is
// presumed a layout (decision 111, as the lint judges it). headers counts the tables with header
// cells by how they are known (tablelayout.HeaderSourceOf) and the presumed layouts: the row's
// provenance.
func tableItems(tables []rec.Map, nodes rec.Map, places *gaplocate.Locator) (items []any, headers rec.Map) {
	items, headers = []any{}, rec.Map{}
	count := func(key string) {
		previous, _ := headers[key].(int64)
		headers[key] = previous + 1
	}
	for _, span := range tables {
		grid := rec.FetchMap(span, "grid")
		if grid["layout"] == true {
			continue
		}
		if source := tablelayout.HeaderSourceOf(grid, nodes); source != "" {
			count(source)
			continue
		}
		if tablelayout.PresumedLayout(grid) {
			count("layout_presumed")
			continue
		}
		cells, _ := grid["cells"].([]any)
		items = append(items, placed(rec.Map{"id": span["id"], "cells": int64(len(cells))}, places.Table(rec.AsString(span["id"]))))
	}
	return items, headers
}
