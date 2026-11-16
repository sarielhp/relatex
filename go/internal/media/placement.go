// Package media is the Go port of Relatex::MediaPlacement and the graphics evidence that
// binds retained image slots to native resources (GraphicEvidence, GraphicEvidenceV2).
package media

import (
	"math"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// placement follows the executed graphicx operations in source top-left bp. The matrix is
// kept as JSON values: before any operation it holds the Integer zeros Ruby writes as 0.
type placement struct {
	source, use   rec.Map
	matrix        []any
	width, height float64
	depth         float64
	naturalWidth  float64
	naturalHeight float64
	viewport      []float64
}

// Placement is MediaPlacement.new(source, use, operations).data.
func Placement(source, use rec.Map, operations []any) rec.Map {
	p := &placement{source: source, use: use, width: geometry.SPToBP(float(rec.Fetch(use, "width"))),
		height: geometry.SPToBP(float(rec.Fetch(use, "height")))}
	p.establishViewport()
	for _, operation := range operations {
		p.transform(rec.AsMap(operation))
	}
	data := rec.Map{"schema_version": mediamodel.PlacementVersion, "page": rec.Fetch(source, "page"), "pagebox": rec.Fetch(source, "pagebox"),
		"source_box_bp": []any{int64(0), int64(0), p.naturalWidth, p.naturalHeight}, "viewport_bp": geometry.Floats(p.viewport),
		"clip": rec.Fetch(use, "clip"), "matrix": p.matrix,
		"width_sp": geometry.BPToSP(p.width), "height_sp": geometry.BPToSP(p.height + p.depth), "depth_sp": geometry.BPToSP(p.depth),
		"operations": operations}
	data["id"] = digest(data)
	mediamodel.ValidatePlacement(data)
	return data
}

func float(value any) float64 {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("expected a number, got %T", value)
	}
	return number
}

func digest(value any) string {
	text, err := canonjson.Digest(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return text
}

func (p *placement) establishViewport() {
	p.naturalWidth = geometry.SPToBP(float(rec.Fetch(p.source, "natural_width")))
	p.naturalHeight = geometry.SPToBP(float(rec.Fetch(p.source, "natural_height")))
	box := geometry.Box(rec.FetchList(p.use, "viewport"))
	llx, lly, urx, ury := box[0], box[1], box[2], box[3]
	p.viewport = []float64{llx, p.naturalHeight - ury, urx, p.naturalHeight - lly}
	width := urx - llx
	height := ury - lly
	if !(width > 0) || !(height > 0) {
		rec.Fail("Empty graphic viewport")
	}
	sx := p.width / width
	sy := p.height / height
	p.matrix = []any{sx, int64(0), int64(0), sy, -llx * sx, -p.viewport[1] * sy}
}

func (p *placement) transform(operation rec.Map) {
	var matrix []any
	switch rec.Fetch(operation, "kind") {
	case "rotate":
		values := geometry.Numbers([]any{rec.Fetch(operation, "cos"), rec.Fetch(operation, "sin")}, 2)
		cosine, sine := values[0], values[1]
		if math.Abs(float64(cosine*cosine)+float64(sine*sine)-1) >= 0.001 {
			rec.Fail("Invalid native rotation")
		}
		matrix = []any{cosine, -sine, sine, cosine, int64(0), int64(0)}
	case "scale":
		values := geometry.Numbers([]any{rec.Fetch(operation, "x"), rec.Fetch(operation, "y")}, 2)
		matrix = []any{values[0], int64(0), int64(0), values[1], int64(0), int64(0)}
	default:
		rec.Fail("Unknown native graphic transformation")
	}
	own := geometry.Numbers(matrix, 6)
	bounds := geometry.TransformedBox(own, []float64{0, 0, p.width, p.height + p.depth})
	moved := geometry.Compose([]float64{1, 0, 0, 1, -bounds[0], -bounds[1]}, own)
	p.matrix = geometry.Floats(geometry.Compose(moved, geometry.Numbers(p.matrix, 6)))
	p.width = bounds[2] - bounds[0]
	total := bounds[3] - bounds[1]
	if rec.Fetch(operation, "kind") == "rotate" {
		p.depth = geometry.SPToBP(float(rec.Fetch(operation, "depth")))
	} else {
		p.depth = p.scaledDepth(operation)
	}
	p.height = total - p.depth
	if rec.Fetch(operation, "kind") != "rotate" {
		return
	}
	p.width = geometry.SPToBP(float(rec.Fetch(operation, "width")))
	p.height = geometry.SPToBP(float(rec.Fetch(operation, "height")))
}

func (p *placement) scaledDepth(operation rec.Map) float64 {
	factor := float(rec.Fetch(operation, "y"))
	if factor < 0 {
		return p.height * -factor
	}
	return p.depth * factor
}
