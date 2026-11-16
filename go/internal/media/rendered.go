package media

import (
	"fmt"
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
)

var resolvedFeatures = []string{"tikz", "xy", "picture", "pgf-clip"}

type pictureKey struct {
	context any
	page    any
}

// Rendered is RenderedGraphicEvidence: a graphic without an inspectable source (a TikZ
// picture) is read as the region of the reference PDF that TeX shipped for it. Shipout
// records each of its boxes' positions; the union per page is the region later cropped.
type Rendered struct {
	boxes    map[pictureKey][]int64
	rendered map[any]any
	count    int
}

// NewRendered is RenderedGraphicEvidence.new(records).
func NewRendered(records []rec.Map) *Rendered {
	r := &Rendered{boxes: map[pictureKey][]int64{}, rendered: map[any]any{}}
	for _, record := range records {
		if record["kind"] != "diagnostic" || record["code"] != "graphic_position" {
			continue
		}
		name := pictureKey{rec.Fetch(record, "context"), rec.Fetch(record, "page")}
		r.boxes[name] = union(r.boxes[name], positionBox(record))
	}
	return r
}

// Span is the span for the first retained box of a picture; inside reports that the
// picture is already rendered (its further boxes are inside), and nil, false that no
// region was shipped for it on the page.
func (r *Rendered) Span(item rec.Map, page any) (span rec.Map, inside bool) {
	context := rec.Fetch(item, "graphic")
	if _, ok := r.rendered[context]; ok {
		return nil, true
	}
	region, ok := r.boxes[pictureKey{context, page}]
	if !ok {
		return nil, false
	}
	r.rendered[context] = page
	r.count++
	return rec.Map{"type": "rendered_graphic", "id": fmt.Sprintf("r%d", r.count), "graphic": context, "page": page,
		"region_sp": []any{region[0], region[1], region[2], region[3]}, "depth_sp": rec.Fetch(item, "depth")}, false
}

// Resolved reports whether the source limitation is resolved by the picture's own
// rendering being in the reading.
func (r *Rendered) Resolved(diagnostic rec.Map) bool {
	if rec.Fetch(diagnostic, "code") != "unsupported_graphic_source" || !slices.Contains(resolvedFeatures, asString(diagnostic["feature"])) {
		return false
	}
	_, ok := r.rendered[diagnostic["context"]]
	return ok
}

// positionBox is [left, bottom, right, top] in sp from the page's lower left corner.
func positionBox(record rec.Map) []int64 {
	x, y := rec.FetchInt(record, "x_sp"), rec.FetchInt(record, "y_sp")
	width, height, depth := rec.FetchInt(record, "width_sp"), rec.FetchInt(record, "height_sp"), rec.FetchInt(record, "depth_sp")
	top := y
	if rec.Fetch(record, "box") == "hlist" {
		top = y + height
	}
	return []int64{x, top - height - depth, x + width, top}
}

func union(first, second []int64) []int64 {
	if first == nil {
		return second
	}
	return []int64{min(first[0], second[0]), min(first[1], second[1]), max(first[2], second[2]), max(first[3], second[3])}
}
