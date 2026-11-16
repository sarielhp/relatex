package figdesc

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Complexity is how much a figure's drawing holds (decision 110): the glyphs of its text labels
// and the marks it draws (paths, shapes, placed symbols), counted in its displayed SVG outside
// definitions. A raster figure is not measured.
type Complexity struct {
	Glyphs, Marks int
	Measured      bool
}

// Thresholds of a complex figure, set conservatively on the stores' figures (2026-11-02): a plot
// with labelled axes, ticks and a legend, or a diagram with many labelled parts, holds both many
// glyphs and many marks; a small diagram with a few labels does not.
const (
	complexGlyphs = 40
	complexMarks  = 100
)

// Complex tells whether the figure is likely too rich for a short description alone.
func (c Complexity) Complex() bool {
	return c.Measured && c.Glyphs >= complexGlyphs && c.Marks >= complexMarks
}

func (c Complexity) String() string {
	return fmt.Sprintf("%d text glyphs, %d drawn marks", c.Glyphs, c.Marks)
}

// Complexity measures the figure's displayed picture, once.
func (f *Figure) Complexity() Complexity {
	if f.complexity == nil {
		measured := Complexity{}
		if f.document != nil && f.span != nil {
			if data, mime := f.displayed(); mime == "image/svg+xml" {
				measured = MeasureSVG(string(data))
			}
		}
		f.complexity = &measured
	}
	return *f.complexity
}

var (
	svgTag = regexp.MustCompile(`<(/?)([A-Za-z][\w:.-]*)([^>]*?)(/?)>([^<]*)`)
	// glyphReference is a use of a glyph outline: the converters name them glyph…, font… or g….
	glyphReference = regexp.MustCompile(`(?i)href\s*=\s*["']#(?:glyph|font|g[\d_-])`)
)

// hiddenElements hold definitions, which draw only where they are used.
var (
	hiddenElements = []string{"defs", "symbol", "clipPath", "mask", "pattern", "marker"}
	markElements   = []string{"path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "image"}
)

// MeasureSVG counts an SVG's text glyphs (glyph uses and the characters of text elements) and its
// marks (shapes and other uses) outside definitions.
func MeasureSVG(svg string) Complexity {
	measured := Complexity{Measured: true}
	hidden, inText := 0, 0
	for _, match := range svgTag.FindAllStringSubmatch(svg, -1) {
		closing, name, attributes, empty, after := match[1] != "", match[2], match[3], match[4] != "", match[5]
		if slices.Contains(hiddenElements, name) {
			hidden += nesting(closing, empty)
			continue
		}
		if hidden > 0 {
			continue
		}
		if name == "text" {
			inText += nesting(closing, empty)
		}
		switch {
		case closing:
		case name == "use" && glyphReference.MatchString(attributes):
			measured.Glyphs++
		case name == "use" || slices.Contains(markElements, name):
			measured.Marks++
		}
		if inText > 0 && !closing {
			measured.Glyphs += len([]rune(strings.TrimSpace(after)))
		}
	}
	return measured
}

// nesting is how a tag changes the depth of its element: +1 opening, -1 closing, 0 empty.
func nesting(closing, empty bool) int {
	switch {
	case closing:
		return -1
	case empty:
		return 0
	}
	return 1
}
