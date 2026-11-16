package svgoutline

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
)

// A bitmap Type 3 glyph (a PK font that a TeX engine embeds when a font has no outline font,
// papers/0088 and 0101: the bbm "1") is an image mask. MuPDF writes it as a group holding a
// mask, whose one image is the glyph's bitmap, and a rect the mask stencils, the rect's
// transform mapping the bitmap's pixels to glyph space. Its ink is the bitmap's set pixels, so
// its bounds are the box of those pixels in glyph space: measured, not the rect's extent. Any
// other shape of mask, image or rect stays unsupported.

// boundsMeasure is a glyph outline piece's measurement: the bounds of its ink under a matrix.
type boundsMeasure interface {
	bounds(matrix []float64) []float64
}

// stencilBounds is a stencilled rect's ink box in its own space, nil for a bitmap without ink.
type stencilBounds struct {
	box []float64
}

func (s *stencilBounds) bounds(matrix []float64) []float64 {
	if s.box == nil {
		return nil
	}
	return geometry.TransformedBox(matrix, s.box)
}

var (
	maskAttributes  = map[string]bool{"id": true}
	imageAttributes = map[string]bool{"id": true, "width": true, "height": true, "xlink:href": true}
	rectAttributes  = map[string]bool{"x": true, "y": true, "width": true, "height": true, "transform": true, "mask": true}
)

// onlyAttributes: every attribute of the element is an allowed one.
func onlyAttributes(item *element, allowed map[string]bool) bool {
	for name := range item.attrs {
		if !allowed[name] {
			return false
		}
	}
	return true
}

// stencilMask is the mask a glyph group's stencilled rect names, which must be a sibling.
func stencilMask(rect *element, group *element) *element {
	reference := rect.attrs["mask"]
	if !strings.HasPrefix(reference, "url(#") || !strings.HasSuffix(reference, ")") {
		rec.Fail("Unsupported SVG glyph outline target")
	}
	id := strings.TrimSuffix(strings.TrimPrefix(reference, "url(#"), ")")
	for _, child := range group.children {
		if child.name == "mask" && child.attrs["id"] == id {
			return child
		}
	}
	rec.Fail("Unsupported SVG glyph outline target")
	return nil
}

// stencilImage is a glyph mask's one image, after checking the mask and the rect it stencils:
// the image lies exactly under the rect (both at the origin, of one size).
func stencilImage(mask, rect *element) *element {
	if !onlyAttributes(mask, maskAttributes) || len(mask.children) != 1 || !onlyAttributes(rect, rectAttributes) ||
		len(rect.children) > 0 {
		rec.Fail("Unsupported SVG glyph outline target")
	}
	picture := mask.children[0]
	if picture.name != "image" || len(picture.children) > 0 || !onlyAttributes(picture, imageAttributes) {
		rec.Fail("Unsupported SVG glyph outline target")
	}
	for _, key := range []string{"x", "y"} {
		if value, ok := rect.attrs[key]; ok && attributeNumber(value) != 0 {
			rec.Fail("Unsupported SVG glyph outline target")
		}
	}
	for _, key := range []string{"width", "height"} {
		if attributeNumber(rect.attrs[key]) != attributeNumber(picture.attrs[key]) {
			rec.Fail("Unsupported SVG glyph outline target")
		}
	}
	return picture
}

// attributeNumber is a length attribute's number; anything else is unsupported.
func attributeNumber(value string) float64 {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		rec.Fail("Unsupported SVG glyph outline target")
	}
	return number
}

// newStencilBounds measures a stencilled rect: the box of its bitmap's set pixels, scaled from
// pixels to the image's width and height.
func newStencilBounds(rect *element, group *element) *stencilBounds {
	picture := stencilImage(stencilMask(rect, group), rect)
	bitmap := decodeStencil(picture.attrs["xlink:href"])
	box := inkedPixels(bitmap)
	if box == nil {
		return &stencilBounds{}
	}
	size := bitmap.Bounds().Size()
	sx, sy := attributeNumber(picture.attrs["width"])/float64(size.X), attributeNumber(picture.attrs["height"])/float64(size.Y)
	return &stencilBounds{box: []float64{box[0] * sx, box[1] * sy, box[2] * sx, box[3] * sy}}
}

// decodeStencil reads a mask image's PNG data URI.
func decodeStencil(uri string) image.Image {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(uri, prefix) {
		rec.Fail("Unsupported SVG glyph outline target")
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(uri[len(prefix):]), ""))
	if err != nil {
		rec.Fail("Invalid SVG glyph stencil: %s", err.Error())
	}
	bitmap, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		rec.Fail("Invalid SVG glyph stencil: %s", err.Error())
	}
	return bitmap
}

// inkedPixels is the box, in pixels from the image's corner, of the pixels a luminance mask lets
// through (any nonzero premultiplied color); nil when none does.
func inkedPixels(bitmap image.Image) []float64 {
	area := bitmap.Bounds()
	var box []float64
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if r, g, b, _ := bitmap.At(x, y).RGBA(); r|g|b == 0 {
				continue
			}
			pixel := []float64{float64(x - area.Min.X), float64(y - area.Min.Y), float64(x - area.Min.X + 1), float64(y - area.Min.Y + 1)}
			box = unionBox(box, pixel)
		}
	}
	return box
}
