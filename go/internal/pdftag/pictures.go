package pdftag

import "math"

// Drawn pictures (decision 105, amendment of 2026-11-02): a TikZ, pgfplots, picture-mode, xy-pic or
// PSTricks picture is painted in the page's own content, not as an image object, so no XObject
// placement can be its Figure's. The core records the box each one was cropped from
// (glyphowner.Picture); every path, shading, image and glyph painted inside that box is the
// Figure's content. A picture nothing is painted in stays unplaced (images_placed).

// pictureMargin is how far, in bp, a path may reach beyond its picture's box: TikZ bounds a picture
// by its path coordinates, not by the line width or every arrow tip.
const pictureMargin = 2

// bounds is a box in device space: lower-left and upper-right corners.
type bounds [4]float64

// emptyBounds holds no point; adding one makes it that point.
var emptyBounds = bounds{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}

// add extends the box by a point.
func (b *bounds) add(x, y float64) {
	b[0], b[1], b[2], b[3] = min(b[0], x), min(b[1], y), max(b[2], x), max(b[3], y)
}

// empty reports whether the box holds no point.
func (b bounds) empty() bool { return b[0] > b[2] || b[1] > b[3] }

// within reports whether the box lies inside another, grown by margin on every side.
func (b bounds) within(outer bounds, margin float64) bool {
	return !b.empty() && b[0] >= outer[0]-margin && b[1] >= outer[1]-margin &&
		b[2] <= outer[2]+margin && b[3] <= outer[3]+margin
}

// intersect is the box both boxes cover (empty when they are apart).
func (b bounds) intersect(other bounds) bounds {
	return bounds{max(b[0], other[0]), max(b[1], other[1]), min(b[2], other[2]), min(b[3], other[3])}
}

// apply is the point (x, y) through the matrix.
func (m matrix) apply(x, y float64) (float64, float64) {
	return float64(x*m[0]) + float64(y*m[2]) + m[4], float64(x*m[1]) + float64(y*m[3]) + m[5]
}

// rectangle is the device box of a rectangle of user space through the matrix.
func (m matrix) rectangle(x0, y0, x1, y1 float64) bounds {
	box := emptyBounds
	for _, corner := range [][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		box.add(m.apply(corner[0], corner[1]))
	}
	return box
}

// pathOperands are how many numbers each path construction operator takes, as points.
var pathOperands = map[string]int{"m": 2, "l": 2, "c": 6, "v": 4, "y": 4}

// extendPath adds a path construction operator's points to the current path's box; a clipping
// operator makes the path a clip when it is painted.
func (in *interpreter) extendPath(op operation) {
	switch op.operator {
	case "W", "W*":
		in.clipping = true
		return
	case "re":
		values, err := numbers(op, 4)
		if err == nil {
			box := in.state.ctm.rectangle(values[0], values[1], values[0]+values[2], values[1]+values[3])
			in.path.add(box[0], box[1])
			in.path.add(box[2], box[3])
		}
		return
	}
	count := pathOperands[op.operator]
	if count == 0 {
		return
	}
	values, err := numbers(op, count)
	if err != nil {
		return
	}
	for i := 0; i+1 < count; i += 2 {
		in.path.add(in.state.ctm.apply(values[i], values[i+1]))
	}
}

// endPath is the painted path's device box; a clipping path narrows the clip.
func (in *interpreter) endPath() (bounds, bool) {
	box := in.path
	in.path = emptyBounds
	if in.clipping && !box.empty() {
		if in.state.clipped {
			in.state.clip = in.state.clip.intersect(box)
		} else {
			in.state.clip, in.state.clipped = box, true
		}
	}
	in.clipping = false
	return box, !box.empty()
}

// objectBox is the device box of a Do, sh or inline image: an image's unit square, a form's
// bounding box, a shading's clip (a shading without a clip fills the page: no box).
func (in *interpreter) objectBox(entry *item, dict map[string]any) {
	switch {
	case entry.op.operator == "sh":
		entry.box, entry.boxed = in.state.clip, in.state.clipped
	case entry.xobject == "Form":
		at := in.state.ctm
		if form := in.file.array(dict["/Matrix"]); len(form) == 6 {
			var m matrix
			for i := range m {
				m[i] = numberOf(form[i])
			}
			at = m.times(at)
		}
		if box := in.file.array(dict["/BBox"]); len(box) == 4 {
			entry.box = at.rectangle(numberOf(box[0]), numberOf(box[1]), numberOf(box[2]), numberOf(box[3]))
			entry.boxed = true
		}
	default:
		entry.box, entry.boxed = in.state.ctm.rectangle(0, 0, 1, 1), true
	}
}

// picture is a drawn picture's Figure leaf and its box in device space.
type picture struct {
	leaf string
	box  bounds
}

// pagePictures are the drawn pictures of a page whose leaves are Figures, their boxes turned from
// the layout's frame (top-left, bp) into device space by the page's box.
func (t *tagger) pagePictures(number int, frame pageFrame) []picture {
	if t.owners == nil {
		return nil
	}
	var pictures []picture
	for _, drawn := range t.owners.Pictures {
		if drawn.Page != number || t.roleOf(drawn.Node) != "Figure" {
			continue
		}
		box := drawn.Box
		left, top := frame.user(box[0], box[1])
		right, bottom := frame.user(box[2], box[3])
		pictures = append(pictures, picture{leaf: drawn.Node,
			box: bounds{min(left, right), min(top, bottom), max(left, right), max(top, bottom)}})
	}
	return pictures
}

// drawnLeaves are the image leaves the core bounded as drawn pictures: no image object is theirs.
func (t *tagger) drawnLeaves() map[string]bool {
	leaves := map[string]bool{}
	if t.owners != nil {
		for _, drawn := range t.owners.Pictures {
			leaves[drawn.Node] = true
		}
	}
	return leaves
}

// pictureOf is the leaf of the picture a box lies in, "" for none.
func pictureOf(pictures []picture, box bounds, margin float64) string {
	for _, p := range pictures {
		if box.within(p.box, margin) {
			return p.leaf
		}
	}
	return ""
}

// glyphPicture is the picture a glyph's body lies in, by the layout's position: the glyph belongs to
// the picture when its horizontal midpoint (origin plus half its advance) does, not when its origin
// does. TeX sets the text after an inline picture with its origin exactly on the box's right edge
// (papers 0191: the "," after the picture at x=166.08, the box's right edge, and the ")" at 74.1),
// and a glyph that starts where the picture ends is the text's. A glyph TeX set outside every
// literal-drawn box, which the owners give a leaf other than the picture's host, stays its owner's:
// a caption pulled up into the picture's box by a negative \vspace (papers 0097's Figure 2, whose
// TikZ box reaches below the caption's first baseline) is no part of the picture, and its first line
// read back as the Figure's.
func (t *tagger) glyphPicture(page *prepared, pictures []picture, ordinal int) string {
	if len(pictures) == 0 || page.failure != "" || ordinal >= len(page.mapper.mapping) {
		return ""
	}
	index := page.mapper.mapping[ordinal]
	painted := t.painted[page.number]
	if index < 0 || index >= len(painted) || painted[index].unknown {
		return ""
	}
	frame := page.mapper.frame
	glyph := painted[index]
	x, y := frame.user(glyph.baseline[0]+float64(glyph.advance/2), glyph.baseline[1])
	leaf := pictureOf(pictures, bounds{x, y, x, y}, 0)
	if owner, owned := t.placedOwners[glyphRef{page.number, index}]; owned && owner != t.builder.hosts[leaf] {
		return ""
	}
	return leaf
}
