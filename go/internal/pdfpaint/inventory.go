package pdfpaint

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// inventory is PDFPaintInventory: scalar MuPDF paint atoms. A table chooses its own region
// and checks every atom there; unrelated page graphics do not grant permission to ignore
// table ink.
type inventory struct {
	page   int64
	glyphs []rec.Map
	rules  []rec.Map
	other  []rec.Map
}

type walkState struct {
	clip       int
	restricted bool
}

func newInventory(data []byte, page int64) (result *inventory) {
	defer rescueArgument("Invalid PDF paint inventory: ")
	if nativetrace.Declared(data) {
		rec.Fail("PDF paint XML declarations forbidden")
	}
	pages := parseTrace(data).Descendants("page")
	if len(pages) != 1 || rec.ToI(pages[0].Attrs["number"]) != page {
		rec.Fail("Unexpected PDF paint page")
	}
	result = &inventory{page: page, glyphs: []rec.Map{}, rules: []rec.Map{}, other: []rec.Map{}}
	result.walk(pages[0], &walkState{})
	return result
}

// value is a required attribute; a missing one is an invalid inventory.
func value(element *node, name string) string {
	text, ok := element.Attrs[name]
	if !ok {
		rec.RaiseArgument("%s lacks %s", element.Name, name)
	}
	return text
}

func (v *inventory) walk(element *node, state *walkState) {
	for _, child := range element.Children {
		name := child.Name
		if strings.HasPrefix(name, "clip_") {
			state.clip++
			continue
		}
		if name == "pop_clip" {
			state.clip--
			if state.clip < 0 {
				rec.Fail("Unbalanced PDF paint clip stack")
			}
			continue
		}
		v.visit(child, state)
	}
}

func (v *inventory) visit(child *node, state *walkState) {
	restricted := *state
	restricted.restricted = true
	switch child.Name {
	case "fill_text":
		v.text(child, state)
	case "stroke_text", "ignore_text":
		v.text(child, &restricted)
	case "stroke_path", "fill_path":
		v.path(child, state)
	case "fill_image", "fill_image_mask":
		v.image(child)
	case "fill_shade":
		rec.Fail("Unbounded shading in table paint profile")
	case "group":
		safe := child.Attrs["alpha"] == "1" && child.Attrs["blendmode"] == "Normal" && child.Attrs["knockout"] == "0"
		group := *state
		group.restricted = state.restricted || !safe
		v.walk(child, &group)
	case "mask", "tile":
		v.walk(child, &restricted)
	default:
		if len(child.Children) > 0 {
			v.walk(child, state)
		}
	}
}

func matrix(element *node) []float64 {
	values := floats(value(element, "transform"))
	return geometry.Numbers(geometry.Floats(values), 6)
}

func black(space string, color []float64) bool {
	return (space == "DeviceGray" && floatsEqual(color, []float64{0})) ||
		(space == "DeviceRGB" && floatsEqual(color, []float64{0, 0, 0}))
}

func (v *inventory) admissible(element *node, state *walkState) bool {
	color := floats(element.Attrs["color"])
	alpha, ok := element.Attrs["alpha"]
	if !ok {
		alpha = "1"
	}
	return black(element.Attrs["colorspace"], color) && alpha == "1" && state.clip == 0 && !state.restricted
}

func (v *inventory) text(element *node, state *walkState) {
	transform := matrix(element)
	safe := v.admissible(element, state)
	for _, span := range element.Named("span") {
		size := floats(value(span, "trm"))
		finite := len(size) == 4
		for _, number := range size {
			finite = finite && !math.IsInf(number, 0) && !math.IsNaN(number)
		}
		if !finite {
			rec.Fail("Invalid PDF font matrix")
		}
		var previous rec.Map
		for _, glyph := range span.Named("g") {
			unicode := value(glyph, "unicode")
			if _, anchored := glyph.Attrs["glyph"]; !anchored {
				if previous == nil {
					rec.Fail("Unanchored PDF Unicode continuation")
				}
				previous["text"] = previous["text"].(string) + unicode
				continue
			}
			x, y := rubyFloat(value(glyph, "x")), rubyFloat(value(glyph, "y"))
			px, py := geometry.Point(transform, x, y)
			scale := math.Hypot(size[0], size[1])
			previous = rec.Map{"id": fmt.Sprintf("g%d", len(v.glyphs)), "text": unicode, "baseline": []any{px, py},
				"width": rubyFloat(value(glyph, "adv")) * scale, "font": paintmodel.FontName(value(span, "font")),
				"size": math.Hypot(size[2], size[3]),
				"safe": safe && identityPrefix(transform) && size[1] == 0 && size[2] == 0}
			v.glyphs = append(v.glyphs, previous)
		}
	}
}

type point [2]float64

func (v *inventory) path(element *node, state *walkState) {
	transform := matrix(element)
	points := []point{}
	for _, command := range element.Children {
		if command.Name != "moveto" && command.Name != "lineto" {
			continue
		}
		x, y := geometry.Point(transform, rubyFloat(value(command, "x")), rubyFloat(value(command, "y")))
		points = append(points, point{x, y})
	}
	if len(points) == 0 {
		v.unknownPath(element, transform)
		return
	}
	rectangle := bounds(points)
	var valid bool
	if element.Name == "stroke_path" {
		valid = strokeRectangle(element, points, rectangle)
	} else {
		valid = filledRectangle(element, points)
	}
	if !valid {
		v.unknownPath(element, transform)
		return
	}
	v.rules = append(v.rules, rec.Map{"id": fmt.Sprintf("r%d", len(v.rules)), "rect": geometry.Floats(rectangle),
		"safe": v.admissible(element, state)})
}

func bounds(points []point) []float64 {
	// Ruby's minmax: the first of equal values (0.0 before -0.0), unlike math.Min.
	result := []float64{points[0][0], points[0][1], points[0][0], points[0][1]}
	for _, p := range points[1:] {
		for axis := range 2 {
			if p[axis] < result[axis] {
				result[axis] = p[axis]
			}
			if p[axis] > result[axis+2] {
				result[axis+2] = p[axis]
			}
		}
	}
	return result
}

// strokeRectangle expands the supplied rectangle in place by the stroke's width.
func strokeRectangle(element *node, points []point, rectangle []float64) bool {
	if len(points) != 2 || !slices.Equal(element.Names(), []string{"moveto", "lineto"}) || element.Attrs["linecap"] != "0,0,0" {
		return false
	}
	if rectangle[0] != rectangle[2] && rectangle[1] != rectangle[3] {
		return false
	}
	if !identityPrefix(matrix(element)) {
		return false
	}
	width := rubyFloat(value(element, "linewidth"))
	if !(width > 0) || math.IsInf(width, 0) {
		return false
	}
	if rectangle[1] == rectangle[3] {
		rectangle[1] -= float64(width / 2)
		rectangle[3] += float64(width / 2)
	} else {
		rectangle[0] -= float64(width / 2)
		rectangle[2] += float64(width / 2)
	}
	return true
}

func filledRectangle(element *node, points []point) bool {
	for _, name := range element.Names() {
		if name != "moveto" && name != "lineto" && name != "closepath" {
			return false
		}
	}
	if len(points) != 4 && len(points) != 5 {
		return false
	}
	closed := points
	if points[len(points)-1] != points[0] {
		closed = append(slices.Clone(points), points[0])
	}
	for index := 0; index+1 < len(closed); index++ {
		a, b := closed[index], closed[index+1]
		if a[0] != b[0] && a[1] != b[1] {
			return false
		}
	}
	unique := []point{}
	for _, p := range points {
		if !slices.Contains(unique, p) {
			unique = append(unique, p)
		}
	}
	return len(unique) == 4
}

func (v *inventory) unknownPath(element *node, transform []float64) {
	points := []point{}
	for _, command := range element.Children {
		for _, pair := range [][2]string{{"x", "y"}, {"x1", "y1"}, {"x2", "y2"}, {"x3", "y3"}} {
			x, hasX := command.Attrs[pair[0]]
			y, hasY := command.Attrs[pair[1]]
			if hasX && hasY {
				px, py := geometry.Point(transform, rubyFloat(x), rubyFloat(y))
				points = append(points, point{px, py})
			}
		}
	}
	if len(points) == 0 {
		rec.Fail("PDF paint path has no bounded geometry")
	}
	v.other = append(v.other, rec.Map{"rect": geometry.Floats(bounds(points)), "kind": element.Name})
}

func (v *inventory) image(element *node) {
	box := geometry.TransformedBox(matrix(element), []float64{0, 0, 1, 1})
	v.other = append(v.other, rec.Map{"rect": geometry.Floats(box), "kind": element.Name})
}
