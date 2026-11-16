package svgoutline

import (
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
)

// A glyph use's target is the outline it draws. MuPDF writes a glyph of an ordinary font as one
// untransformed path in em units. A Type 3 glyph (a font a plotting program embeds in its figure,
// slides/0016) is its own drawing: a group of paths in the glyph space, each with the font matrix
// as its transform, or an empty group for a glyph that draws nothing. Only filled paths and
// stencilled bitmaps (stencil.go) are outlines: a group holding anything else (a stroke, another
// group, a translucent fill) stays unsupported, since its ink is not the pieces' fill. A colored
// Type 3 glyph (one that sets its own colors, d0: the twemoji snowflake and fire of
// papers4/1123) fills its paths in opaque colors of its own; a fill color changes the ink's
// color, not its extent, so a path may name one, but not "none" or a paint server.

// pieceAttributes are the attributes an outline path of a group may carry.
var pieceAttributes = map[string]bool{"d": true, "transform": true, "fill-rule": true, "fill": true}

// solidFill is an opaque fill color as MuPDF's SVG writes one.
var solidFill = regexp.MustCompile(`\A#[0-9a-f]{6}\z`)

// filledPiece: a path filled in the use's color or in a solid color of its own.
func filledPiece(piece *element) bool {
	fill, colored := piece.attrs["fill"]
	return !colored || solidFill.MatchString(fill)
}

// outlinePieces is the paths and stencilled rects a use's target draws; a group's masks are
// drawn only through the rects that name them. A nested group is a clip: it carries clip-path
// naming a clipPath sibling, or it is a plain wrapper (papers6/0798's Type 3 glyph); each piece
// it holds is cut by the clip rect (clipBounds).
func outlinePieces(target *element) []*element {
	if target.name == "path" {
		return []*element{target}
	}
	if target.name != "g" || len(target.attrs) != 1 {
		rec.Fail("Unsupported SVG glyph outline target")
	}
	return groupPieces(target, nil, nil)
}

// groupPieces collects a group's pieces; clips are the clipPaths of its ancestors.
func groupPieces(group *element, clips []*element, pieces []*element) []*element {
	for _, child := range group.children {
		switch child.name {
		case "mask", "clipPath":
			continue
		case "g":
			inner := append(clips[:len(clips):len(clips)], groupClip(child, group)...)
			pieces = groupPieces(child, inner, pieces)
			continue
		case "rect":
			stencilImage(stencilMask(child, group), child)
		case "path":
			if len(child.children) > 0 || !onlyAttributes(child, pieceAttributes) || !filledPiece(child) {
				rec.Fail("Unsupported SVG glyph outline target")
			}
		default:
			rec.Fail("Unsupported SVG glyph outline target")
		}
		child.clips = clips
		pieces = append(pieces, child)
	}
	return pieces
}

// groupClip is the clip a nested group applies: the sibling clipPath its clip-path names (none
// for a plain wrapper). Any other attribute is unsupported.
func groupClip(nested, parent *element) []*element {
	for name := range nested.attrs {
		if name != "clip-path" {
			rec.Fail("Unsupported SVG glyph outline target")
		}
	}
	reference, clipped := nested.attrs["clip-path"]
	if !clipped {
		if !slices.ContainsFunc(nested.children, func(child *element) bool { return child.name == "clipPath" }) {
			rec.Fail("Unsupported SVG glyph outline target")
		}
		return nil
	}
	id := strings.TrimSuffix(strings.TrimPrefix(reference, "url(#"), ")")
	for _, sibling := range parent.children {
		if sibling.name == "clipPath" && sibling.attrs["id"] == id {
			return []*element{sibling}
		}
	}
	rec.Fail("Unsupported SVG glyph outline target")
	return nil
}

// clipBounds cuts box by the rect of each clip the piece sits in; nil when nothing is left.
func clipBounds(box []float64, matrix []float64, piece *element) []float64 {
	for _, clip := range piece.clips {
		if box == nil {
			return nil
		}
		if len(clip.children) != 1 || clip.children[0].name != "path" ||
			!onlyAttributes(clip.children[0], pieceAttributes) {
			rec.Fail("Unsupported SVG glyph outline target")
		}
		path := clip.children[0]
		cut := newPathBounds(path.attrs["d"]).bounds(pieceMatrix(matrix, path))
		if cut == nil {
			return nil
		}
		box = []float64{math.Max(box[0], cut[0]), math.Max(box[1], cut[1]), math.Min(box[2], cut[2]),
			math.Min(box[3], cut[3])}
		if box[0] > box[2] || box[1] > box[3] {
			return nil
		}
	}
	return box
}

// pieceMeasure measures one piece of a use's target.
func pieceMeasure(piece *element) boundsMeasure {
	if piece.name == "rect" {
		return newStencilBounds(piece, piece.parent)
	}
	return newPathBounds(piece.attrs["d"])
}

// pieceMatrix is the use's matrix composed with the piece's own transform.
func pieceMatrix(matrix []float64, piece *element) []float64 {
	transform, ok := piece.attrs["transform"]
	if !ok {
		return matrix
	}
	return geometry.Compose(matrix, parseTransform(transform))
}

// unionBox is the box covering both; nil (an empty outline) covers nothing.
func unionBox(left, right []float64) []float64 {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return []float64{math.Min(left[0], right[0]), math.Min(left[1], right[1]), math.Max(left[2], right[2]),
		math.Max(left[3], right[3])}
}
