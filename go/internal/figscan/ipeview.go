package figscan

import (
	"fmt"
	"strings"
)

// ipeView is the Ipe page and view a PDF page shows.
type ipeView struct {
	page          *element
	pageNumber    int
	view          *element
	viewNumber    int
	viewsOnPage   int
	totalPdfPages int
}

// findView maps a PDF page number to its Ipe page and view as ipelib's PdfWriter::createPages
// does (ipepdfwriter.cpp): every view of every page in order, a page without views as one view
// showing every layer, and no PDF page for a page with a layer named NOPDF. An export with
// ipetoipe -markedview leaves out pages marked="no" and keeps only a page's views marked="yes"
// (its last view when none is); pdfPages (0 when unknown) tells that export from a full one.
func findView(root *element, pdfPage, pdfPages int) (ipeView, bool) {
	places := pdfViews(root, false)
	if pdfPages > 0 && pdfPages != len(places) {
		if marked := pdfViews(root, true); len(marked) == pdfPages {
			places = marked
		}
	}
	if pdfPage < 1 || pdfPage > len(places) {
		return ipeView{totalPdfPages: len(places)}, false
	}
	place := places[pdfPage-1]
	place.totalPdfPages = len(places)
	return place, true
}

// pdfViews lists the (page, view) pairs a PDF export writes, in page order.
func pdfViews(root *element, markedOnly bool) []ipeView {
	var places []ipeView
	number := 0
	for _, page := range children(root, "page") {
		number++
		if hasLayer(page, "NOPDF") || markedOnly && page.attr("marked") == "no" {
			continue
		}
		views := children(page, "view")
		if len(views) == 0 {
			views = []*element{nil}
		}
		for _, v := range exportedViews(views, markedOnly) {
			places = append(places, ipeView{page: page, pageNumber: number, view: views[v], viewNumber: v + 1,
				viewsOnPage: len(views)})
		}
	}
	return places
}

// exportedViews are the indices of the views an export writes.
func exportedViews(views []*element, markedOnly bool) []int {
	var shown []int
	for v, view := range views {
		if !markedOnly || view != nil && view.attr("marked") == "yes" {
			shown = append(shown, v)
		}
	}
	if len(shown) == 0 {
		shown = []int{len(views) - 1}
	}
	return shown
}

func hasLayer(page *element, name string) bool {
	for _, layer := range children(page, "layer") {
		if layer.attr("name") == name {
			return true
		}
	}
	return false
}

func describeView(place ipeView) string {
	if place.viewsOnPage <= 1 {
		return fmt.Sprintf("Ipe page %d", place.pageNumber)
	}
	return fmt.Sprintf("Ipe page %d, view %d of %d", place.pageNumber, place.viewNumber, place.viewsOnPage)
}

func children(e *element, name string) []*element {
	var found []*element
	for i := range e.Children {
		if e.Children[i].XMLName.Local == name {
			found = append(found, &e.Children[i])
		}
	}
	return found
}

// visibleLayers are the layers the view shows; every layer of the page without a view.
func visibleLayers(place ipeView) map[string]bool {
	visible := map[string]bool{}
	if place.view != nil {
		for _, name := range strings.Fields(place.view.attr("layers")) {
			visible[name] = true
		}
		return visible
	}
	return pageLayers(place.page)
}

// shownLayers are the names of the layers the view shows, in the order the page lists them; a page
// without layers shows ipelib's default layer alpha.
func shownLayers(place ipeView) []string {
	visible := visibleLayers(place)
	shown := []string{}
	for _, layer := range children(place.page, "layer") {
		if name := layer.attr("name"); visible[name] {
			shown = append(shown, name)
		}
	}
	if first := firstLayer(place.page); len(shown) == 0 && visible[first] {
		shown = append(shown, first)
	}
	return shown
}

// pageLayers are the page's layers; a page without any has ipelib's default layer alpha.
func pageLayers(page *element) map[string]bool {
	layers := map[string]bool{}
	for _, layer := range children(page, "layer") {
		layers[layer.attr("name")] = true
	}
	layers[firstLayer(page)] = true
	return layers
}

// firstLayer is the layer of a page's objects before any names one: its first layer.
func firstLayer(page *element) string {
	if layers := children(page, "layer"); len(layers) > 0 {
		return layers[0].attr("name")
	}
	return "alpha"
}

// viewColorMap is a view's colour mapping (<map kind="color" from to>, Ipe 7.2.25): the first
// mapping of a name wins, as in AttributeMap::map.
func viewColorMap(view *element) map[string]string {
	mapping := map[string]string{}
	if view == nil {
		return mapping
	}
	for _, m := range children(view, "map") {
		if _, seen := mapping[m.attr("from")]; m.attr("kind") == "color" && !seen {
			mapping[m.attr("from")] = m.attr("to")
		}
	}
	return mapping
}

// readSymbols are the symbol names the document's stylesheets define.
func readSymbols(root *element) map[string]bool {
	symbols := map[string]bool{}
	for _, sheet := range children(root, "ipestyle") {
		for _, symbol := range children(sheet, "symbol") {
			symbols[symbol.attr("name")] = true
		}
	}
	return symbols
}
