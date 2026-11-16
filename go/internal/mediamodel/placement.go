// Package mediamodel is the document model's media (decision 102): the media-1 asset descriptor
// document.json holds for an image (MediaAsset: its original and per-target representations,
// each verified against its bytes), the legacy PNG descriptor (PNGImage), the SVG safety check
// (PDFLayout.validate_svg!), the placement of an asset on the page (MediaPlacement) and the file
// extension of each media type. The core (assets, media) builds them; every reader of the model
// reads them here.
package mediamodel

import (
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
	"math"
	"slices"
)

// Extensions is MediaAsset::EXTENSIONS: the media types an export can carry.
var Extensions = map[string]string{"image/png": "png", "image/jpeg": "jpg", "application/pdf": "pdf", "image/svg+xml": "svg"}

// PlacementVersion is MediaPlacement::VERSION.
const PlacementVersion = "media-placement-1"

var placementFields = []string{"schema_version", "page", "pagebox", "source_box_bp", "viewport_bp", "clip", "matrix",
	"width_sp", "height_sp", "depth_sp", "operations", "id"}

var operationFields = map[string][]string{
	"rotate": {"kind", "cos", "sin", "origin_x", "origin_y", "width", "height", "depth", "input_width", "input_height", "input_depth"},
	"scale":  {"kind", "x", "y", "input_width", "input_height", "input_depth"},
}

// ValidatePlacement is MediaPlacement.validate!.
func ValidatePlacement(data rec.Map) {
	if !rec.SameKeys(data, placementFields) {
		rec.Fail("Unexpected media placement fields")
	}
	if rec.Fetch(data, "schema_version") != PlacementVersion {
		rec.Fail("Unknown media placement version")
	}
	withoutID := rec.Map{}
	for key, value := range data {
		if key != "id" {
			withoutID[key] = value
		}
	}
	if rec.Fetch(data, "id") != digest(withoutID) {
		rec.Fail("Media placement identity differs")
	}
	if !rec.Positive(rec.Fetch(data, "page")) {
		rec.Fail("Invalid selected PDF page")
	}
	if !slices.Contains([]any{"mediabox", "cropbox", "bleedbox", "trimbox", "artbox"}, rec.Fetch(data, "pagebox")) {
		rec.Fail("Unknown PDF page box")
	}
	if clip := rec.Fetch(data, "clip"); clip != true && clip != false {
		rec.Fail("Invalid media clipping flag")
	}
	geometry.Inverse(geometry.Numbers(rec.FetchList(data, "matrix"), 6))
	geometry.Box(rec.FetchList(data, "source_box_bp"))
	geometry.Box(rec.FetchList(data, "viewport_bp"))
	for _, key := range []string{"width_sp", "height_sp"} {
		value, ok := rec.Fetch(data, key).(int64)
		if !ok || value < 1 || value > 1_073_741_823 {
			rec.Fail("Invalid media canvas dimensions")
		}
	}
	if !rec.IsInt(rec.Fetch(data, "depth_sp")) {
		rec.Fail("Invalid media baseline")
	}
	validateOperations(rec.Fetch(data, "operations"))
}

func validateOperations(value any) {
	operations, ok := value.([]any)
	if !ok || len(operations) > 32 {
		rec.Fail("Excessive or invalid media operations")
	}
	for _, item := range operations {
		operation := rec.AsMap(item)
		fields, known := operationFields[rec.FetchString(operation, "kind")]
		if !known {
			rec.Fail("Unknown media operation")
		}
		if !rec.SameKeys(operation, fields) {
			rec.Fail("Unexpected native media operation fields")
		}
		for _, key := range fields[1:] {
			number, isNumber := rec.Number(operation[key])
			if !isNumber || math.IsInf(number, 0) || math.IsNaN(number) || math.Abs(number) > 1_073_741_823 {
				rec.Fail("Invalid native media operation coordinate")
			}
		}
	}
}

func digest(value any) string {
	text, err := canonjson.Digest(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return text
}
