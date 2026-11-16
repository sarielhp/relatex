package pdfpaint

import (
	"math"
	"strings"
)

// The page's canvas (decision 107): a page whose first painting operation fills the whole page in
// one opaque colour (beamer's background canvas, \pagecolor) shows its text on that colour, not
// on white paper. The text paint proof records it when every page has the same one.

// canvasTolerance is how far, in PDF points, a canvas's edges may lie inside the page's.
const canvasTolerance = 0.5

// canvasPoints are the corners a canvas's rectangle may list: moveto and three or four linetos.
const (
	canvasMinPoints = 4
	canvasMaxPoints = 5
)

const (
	canvasDone  = iota // the page's first operation was read
	canvasFirst        // the page's first operation is still to come
	canvasPath         // reading the page's first operation, a fill
)

// canvasReader follows a trace's pages and records each page's canvas, "" for none.
type canvasReader struct {
	pages  map[int64]string
	page   int64
	media  []float64
	state  int
	depth  int
	color  string
	matrix []float64
	points [][2]float64
	curved bool
}

// visit reads one trace element; open are its enclosing elements.
func (r *canvasReader) visit(name string, attrs map[string]string, open []*node) {
	if r.state == canvasPath && len(open) >= r.depth {
		r.addPoint(name, attrs)
		return
	}
	if r.state == canvasPath {
		r.finish()
	}
	switch {
	case name == "page":
		r.start(attrs)
	case r.state != canvasFirst || name == "set_default_colorspaces":
	case name == "fill_path" && len(open) > 0 && open[len(open)-1].Name == "page":
		r.state = canvasDone
		color, ok := canvasColor(attrs)
		if ok {
			r.state, r.depth, r.color, r.matrix, r.points, r.curved = canvasPath, len(open)+1, color, floats(attrs["transform"]), nil, false
		}
	default:
		r.state = canvasDone
	}
}

func (r *canvasReader) start(attrs map[string]string) {
	if r.pages == nil {
		r.pages = map[int64]string{}
	}
	r.page, r.media, r.state = int64(rubyFloat(attrs["number"])), floats(attrs["mediabox"]), canvasFirst
	r.pages[r.page] = ""
}

// addPoint adds a corner of the fill's path; a curve makes it no rectangle.
func (r *canvasReader) addPoint(name string, attrs map[string]string) {
	switch name {
	case "moveto", "lineto":
		r.points = append(r.points, [2]float64{rubyFloat(attrs["x"]), rubyFloat(attrs["y"])})
	case "closepath":
	default:
		r.curved = true
	}
}

// finish decides the page's canvas from the fill read: its colour when its path is a rectangle
// covering the page.
func (r *canvasReader) finish() {
	if r.state == canvasPath && r.covers() {
		r.pages[r.page] = r.color
	}
	r.state = canvasDone
}

// covers: the fill's corners, through its transform, are the corners of a rectangle at least as
// large as the page's media box.
func (r *canvasReader) covers() bool {
	if r.curved || len(r.points) < canvasMinPoints || len(r.points) > canvasMaxPoints || len(r.matrix) != 6 || len(r.media) != 4 {
		return false
	}
	low, high := [2]float64{math.Inf(1), math.Inf(1)}, [2]float64{math.Inf(-1), math.Inf(-1)}
	corners := make([][2]float64, len(r.points))
	for index, point := range r.points {
		corners[index] = placed(r.matrix, point)
		for axis := range 2 {
			low[axis], high[axis] = math.Min(low[axis], corners[index][axis]), math.Max(high[axis], corners[index][axis])
		}
	}
	for _, point := range corners {
		for axis := range 2 {
			if !near(point[axis], low[axis]) && !near(point[axis], high[axis]) {
				return false
			}
		}
	}
	return low[0] <= r.media[0]+canvasTolerance && low[1] <= r.media[1]+canvasTolerance &&
		high[0] >= r.media[2]-canvasTolerance && high[1] >= r.media[3]-canvasTolerance
}

func near(a, b float64) bool { return math.Abs(a-b) <= canvasTolerance }

// canvasColor is an opaque fill's colour as #rrggbb, black included.
func canvasColor(attrs map[string]string) (string, bool) {
	if alpha, ok := attrs["alpha"]; ok && strings.TrimSpace(alpha) != "1" {
		return "", false
	}
	hex, ok := InkHex(attrs["colorspace"], strings.Fields(attrs["color"]))
	if !ok {
		return "", false
	}
	if hex == "" {
		hex = "#000000"
	}
	return hex, true
}

// canvasOf is the canvas every one of pages has, "" when one has none, they differ, or it is white
// (the paper's own colour).
func canvasOf(pages map[int64]string, count int) string {
	if len(pages) != count || count == 0 {
		return ""
	}
	first := pages[1]
	for _, color := range pages {
		if color != first {
			return ""
		}
	}
	if first == "#ffffff" {
		return ""
	}
	return first
}
