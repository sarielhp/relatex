// Package figscan reads what a vector figure draws into a small geometry model (decision 123,
// plans/figure-descriptions.md step 3): groups, paths, marks, labels and images, coordinates
// normalized to the figure's box (the longer side 0–100, the shorter in proportion, so circles stay
// circles; y up, rounded to 0.1). The readers fill one model: the
// embedded Ipe source now, a mutool trace fallback later. The writer turns it into a readable
// text form and a JSON form for a describer, both deterministic byte for byte.
package figscan

import "math"

// Version names the scanner's output; caches key on it.
const Version = "figscan-2"

// Sources of a figure's geometry.
const (
	SourceIpe   = "ipe"
	SourceTrace = "trace"
	// SourceBitmap is a figure that is one image covering the box.
	SourceBitmap = "bitmap"
	// SourceNone is a figure no reader understood.
	SourceNone = "none"
)

// Node kinds.
const (
	KindGroup = "group"
	KindPath  = "path"
	KindMark  = "mark"
	KindText  = "text"
	KindImage = "image"
)

// Point is a normalized position (the box's longer side spans 0–100, y up).
type Point [2]float64

// Box is a rectangle in PDF points: the figure's page box or view box.
type Box struct {
	X0, Y0, X1, Y1 float64
}

// Width is the box's width in points.
func (b Box) Width() float64 { return b.X1 - b.X0 }

// Height is the box's height in points.
func (b Box) Height() float64 { return b.Y1 - b.Y0 }

// scale is normalized units per point: the longer side spans 100.
func (b Box) scale() float64 { return 100 / max(b.Width(), b.Height()) }

// normalizer maps points (PDF and Ipe coordinates) to normalized ones.
func (b Box) normalizer() matrix {
	s := b.scale()
	return matrix{s, 0, 0, s, -b.X0 * s, -b.Y0 * s}
}

// Figure is one page of a figure file as the reader saw it.
type Figure struct {
	Source string `json:"source"`
	// Detail names the source's version or why nothing was read.
	Detail string `json:"detail,omitempty"`
	Page   int    `json:"page"`
	// View names the Ipe page and view the PDF page shows ("page 1 view 2 of 5").
	View string `json:"view,omitempty"`
	// Layers are the Ipe layers the view shows, in the page's order: they tell a view apart from
	// the others of its file (decision 133). Not in the JSON form, which stays figscan-2's.
	Layers []string `json:"-"`
	// Size is the box's width and height in points.
	Size [2]float64 `json:"size_pt"`
	// Extent is the box in normalized units: 100 along the longer side.
	Extent [2]float64 `json:"extent"`
	Nodes  []*Node    `json:"nodes"`
	// Shown and Total count the shape nodes kept by the size rule and all of them.
	Shown int `json:"shown"`
	Total int `json:"total"`
	// Labels lists every text node with its nearest primitives.
	Labels []Label `json:"labels"`
	// Cut counts the top-level nodes the size rule left out, by kind.
	Cut map[string]int `json:"cut,omitempty"`
	// Turned names the turn the paper prints the figure with ("90 degrees clockwise"), which the
	// geometry has been given (decision 123, amendment 3); "" when it is printed as drawn.
	Turned string `json:"turned,omitempty"`

	box Box
}

// Part is one subpath of a path: a polygon, polyline, ellipse, arc, spline or curve.
type Part struct {
	Shape  string  `json:"shape"`
	Points []Point `json:"points,omitempty"`
	Closed bool    `json:"closed,omitempty"`
	// Center, Radii and Angle (degrees, of the first axis) describe an ellipse or an arc's ellipse.
	Center *Point      `json:"center,omitempty"`
	Radii  *[2]float64 `json:"radii,omitempty"`
	Angle  float64     `json:"angle,omitempty"`
	// Through is an arc's middle point; its Points are its start and end.
	Through *Point `json:"through,omitempty"`
	// Original is the point count before simplification (0: the points are all of them).
	Original int `json:"original_points,omitempty"`
}

// Style is how a path, mark or text is painted.
type Style struct {
	Stroke string  `json:"stroke,omitempty"`
	Fill   string  `json:"fill,omitempty"`
	Pen    float64 `json:"pen,omitempty"`
	Dash   string  `json:"dash,omitempty"`
	Arrows string  `json:"arrows,omitempty"`
	// Opacity is the fill opacity (Ipe's opacity attribute), 0 when opaque.
	Opacity float64 `json:"fill_opacity,omitempty"`
}

// Node is one element of the figure's forest.
type Node struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Layer string `json:"layer,omitempty"`
	// Shape is a mark's shape (disk, square, cross …) or a collapsed run's member kind.
	Shape string `json:"shape,omitempty"`
	Parts []Part `json:"parts,omitempty"`
	Style
	// Point is a mark's or a text's position.
	Point *Point `json:"point,omitempty"`
	Size  string `json:"size,omitempty"`
	// Text is a label's source: LaTeX for Ipe; Math marks Ipe's math style ($…$ implied).
	Text     string `json:"text,omitempty"`
	TextType string `json:"text_type,omitempty"`
	Math     bool   `json:"math,omitempty"`
	// Printed is an Ipe label's text as the page prints it (from the trace); PrintedNote says
	// "source" when no printed text matched, "glyphs unmapped" when some glyph has no Unicode.
	Printed     string `json:"printed,omitempty"`
	PrintedNote string `json:"printed_note,omitempty"`
	// Case notes a traced label printed in small caps (its letters uppercased) and the traced text.
	Case string `json:"case,omitempty"`
	// Box is an image's or a group's extent [x0, y0, x1, y1].
	Box *[4]float64 `json:"box,omitempty"`
	// Count is the number of alike members a collapsed run stands for; Members their positions.
	Count   int     `json:"count,omitempty"`
	Members []Point `json:"members,omitempty"`
	// Repeats counts the identical copies drawn over each other (0 or 1: drawn once).
	Repeats  int     `json:"repeats,omitempty"`
	Children []*Node `json:"children,omitempty"`
	// Clip is a group's clipping path (Ipe's clip attribute).
	Clip []Part `json:"clip,omitempty"`
	// Cut counts the children the size rule left out, by kind.
	Cut map[string]int `json:"cut,omitempty"`

	// traced is a trace label's text as its glyphs carry it; smallCaps marks one with small caps.
	traced    string
	smallCaps bool
	// traceEnd is where a trace label's last glyph ends.
	traceEnd  *Point
	score     float64
	contrast  float64
	fillShade float64
}

// Label is a text node with the primitives nearest its position.
type Label struct {
	ID      string    `json:"id"`
	Text    string    `json:"text"`
	Math    bool      `json:"math,omitempty"`
	Printed string    `json:"printed,omitempty"`
	Note    string    `json:"printed_note,omitempty"`
	Point   Point     `json:"point"`
	Nearest []Nearest `json:"nearest"`
}

// Nearest is a primitive near a label and its distance in normalized units.
type Nearest struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Distance float64 `json:"distance"`
}

// round1 rounds to 0.1 and never yields negative zero.
func round1(value float64) float64 {
	rounded := math.Round(value*10) / 10
	if rounded == 0 {
		return 0
	}
	return rounded
}

func roundPoint(p Point) Point { return Point{round1(p[0]), round1(p[1])} }
