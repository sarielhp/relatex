package svgoutline

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// tolerance is PDFUnicodeGlyphs::TOLERANCE.
const tolerance = 0.001

// element is one SVG element: its local name, its attributes by qualified name, its parent.
type element struct {
	name     string
	attrs    map[string]string
	parent   *element
	children []*element
	root     bool
	chain    []float64
	// clips are the clipPaths a nested outline group applies to this piece (targets.go).
	clips []*element
}

// Outlines is SVGGlyphOutlines: the glyph uses of a page SVG, each with its outline path,
// accumulated matrix and measured bounds.
type Outlines struct {
	Uses []rec.Map
	// signatures are the uses' own attributes, by use index: two opaque uses with equal ones at one
	// accumulated matrix paint the same ink (repaint.go).
	signatures []string
	// strokes are, by use index, the half line width (points) of a use that only strokes its outline,
	// or -1; shapes are their signatures without colors (overpaint.go).
	strokes []float64
	shapes  []string
	// fills are, by use index, the use's fill as written (inherited, black without one), lower case.
	fills []string
	// geometries are, by path id, a plain path target's outline (repaint.go).
	geometries map[string]string
	// filled is, by use index, the filled use a stroke companion strokes, or -1 (overpaint.go).
	filled []int
	// page is the root's viewBox (x0, y0, x1, y1), or nil without one (offpage.go).
	page []float64
}

var (
	memo     = map[[32]byte]*Outlines{}
	memoKeys [][32]byte
	memoLock sync.Mutex
)

// For is SVGGlyphOutlines.for: one reading per page SVG in this process.
func For(svg []byte) *Outlines {
	key := sha256.Sum256(svg)
	memoLock.Lock()
	cached, ok := memo[key]
	memoLock.Unlock()
	if ok {
		return cached
	}
	outlines := parseOutlines(svg)
	memoLock.Lock()
	defer memoLock.Unlock()
	for len(memoKeys) >= 64 {
		delete(memo, memoKeys[0])
		memoKeys = memoKeys[1:]
	}
	memo[key] = outlines
	memoKeys = append(memoKeys, key)
	return outlines
}

// Close is SVGGlyphOutlines.close?.
func Close(left, right []any) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		a, aok := rec.Number(left[index])
		b, bok := rec.Number(right[index])
		if !aok || !bok {
			rec.Fail("undefined method '-' for %T", left[index])
		}
		if !(abs(a-b) <= tolerance) {
			return false
		}
	}
	return true
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

// Quad is SVGGlyphOutlines.quad: a box as a quad, or the baseline point twice without one.
func Quad(box, baseline any) []any {
	var values []any
	if list, ok := box.([]any); ok {
		values = list
	} else if box == nil || box == false {
		point := rec.AsList(baseline)
		values = append(append([]any{}, point...), point...)
	} else {
		values = rec.AsList(box)
	}
	at := func(index int) any {
		if index < len(values) {
			return values[index]
		}
		return nil
	}
	x0, y0, x1, y1 := at(0), at(1), at(2), at(3)
	return []any{x0, y0, x1, y0, x0, y1, x1, y1}
}

// parseOutlines reads the SVG (already validated by PDFLayout.validate_svg!'s rules) into
// its elements, then measures each text use.
func parseOutlines(svg []byte) *Outlines {
	mediamodel.ValidateSVG(svg)
	elements, uses := readElements(svg)
	ids := map[string]*element{}
	for _, item := range elements {
		id, ok := item.attrs["id"]
		if !ok {
			continue
		}
		if _, seen := ids[id]; seen {
			rec.Fail("Duplicate SVG outline identity")
		}
		ids[id] = item
	}
	paths := map[string]boundsMeasure{}
	result := &Outlines{Uses: make([]rec.Map, 0, len(uses)), signatures: make([]string, 0, len(uses)),
		strokes: make([]float64, 0, len(uses)), shapes: make([]string, 0, len(uses)), geometries: map[string]string{},
		page: pageBox(elements)}
	for index, use := range uses {
		data := useData(use, index, ids, paths)
		if id := rec.FetchString(data, "path"); ids[id].name == "path" {
			result.geometries[id] = ids[id].attrs["fill-rule"] + "\x00" + ids[id].attrs["d"]
		} else if ids[id].name == "g" {
			result.geometries[id] = groupGeometry(ids[id])
		}
		result.Uses = append(result.Uses, data)
		result.signatures = append(result.signatures, signature(use))
		result.fills = append(result.fills, fillOf(use))
		result.strokes = append(result.strokes, strokeCompanion(use, rec.FetchList(data, "matrix")))
		result.shapes = append(result.shapes, shape(use))
	}
	result.filled = result.filledUses()
	return result
}

// groupGeometry is what a glyph group (a Type3 glyph, already checked by outlinePieces) draws: its pieces' paths,
// stencil bitmaps, placements and fills, without the ids that name the group and its masks. Two groups of
// one geometry draw the same ink (the hatch boxes two table fonts of one figure share, papers6/0899).
func groupGeometry(group *element) string {
	var out strings.Builder
	for _, piece := range outlinePieces(group) {
		if piece.name == "path" {
			out.WriteString("p\x00" + piece.attrs["transform"] + "\x00" + piece.attrs["fill-rule"] + "\x00" +
				piece.attrs["fill"] + "\x00" + piece.attrs["d"] + "\x01")
			continue
		}
		picture := stencilImage(stencilMask(piece, group), piece)
		out.WriteString("r\x00" + piece.attrs["transform"] + "\x00" + piece.attrs["width"] + "\x00" + piece.attrs["height"] +
			"\x00" + picture.attrs["xlink:href"] + "\x01")
	}
	return out.String()
}

// fillOf is the fill a use paints with: its own, else the nearest ancestor's, else the SVG default black.
func fillOf(use *element) string {
	for node := use; node != nil; node = node.parent {
		if value, present := node.attrs["fill"]; present {
			return strings.ToLower(value)
		}
	}
	return "#000000"
}

// filledAs narrows several uses at one placement to the ones filled in the paint's own color (the white
// box and the black pattern of one Type3 hatch glyph pair drawn at one origin, papers6/0899); when the
// color tells none or all apart, matches stay as they are.
func (o *Outlines) filledAs(matches []rec.Map, paint rec.Map) []rec.Map {
	color, known := paint["fill"].(string)
	if !known {
		return matches
	}
	kept := []rec.Map{}
	for _, entry := range matches {
		use := int(rec.FetchInt(entry, "use"))
		if use < len(o.fills) && o.fills[use] == color {
			kept = append(kept, entry)
		}
	}
	if len(kept) == 0 || len(kept) == len(matches) {
		return matches
	}
	return kept
}

func readElements(svg []byte) (all, uses []*element) {
	decoder := xml.NewDecoder(bytes.NewReader(xmltree.Characters(svg)))
	stack := []*element{}
	for {
		token, err := decoder.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			rec.Fail("Invalid native trace XML: %s", err.Error())
		}
		switch typed := token.(type) {
		case xml.StartElement:
			item := &element{name: typed.Name.Local, attrs: map[string]string{}}
			for _, attribute := range typed.Attr {
				item.attrs[qualified(attribute.Name)] = attribute.Value
			}
			if len(stack) > 0 {
				item.parent = stack[len(stack)-1]
				item.parent.children = append(item.parent.children, item)
			} else {
				item.root = true
			}
			stack = append(stack, item)
			all = append(all, item)
			if _, text := item.attrs["data-text"]; text && item.name == "use" {
				uses = append(uses, item)
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	return all, uses
}

func qualified(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

func useData(use *element, index int, ids map[string]*element, paths map[string]boundsMeasure) rec.Map {
	href, ok := use.attrs["xlink:href"]
	if !ok {
		href, ok = use.attrs["href"]
	}
	if !ok || !strings.HasPrefix(href, "#") {
		rec.Fail("Glyph outline must reference a local path")
	}
	id := strings.TrimPrefix(href, "#")
	target, found := ids[id]
	if !found {
		rec.Fail("Missing SVG glyph outline")
	}
	pieces := outlinePieces(target)
	for _, key := range []string{"x", "y", "width", "height"} {
		if _, offset := use.attrs[key]; offset {
			rec.Fail("Unsupported SVG glyph offset")
		}
	}
	matrix := geometry.Compose(chain(use.parent), parseTransform(use.attrs["transform"]))
	var bounds []float64
	for index, piece := range pieces {
		key := id + "\x00" + strconv.Itoa(index)
		measure, ok := paths[key]
		if !ok {
			measure = pieceMeasure(piece)
			paths[key] = measure
		}
		bounds = unionBox(bounds, clipBounds(measure.bounds(pieceMatrix(matrix, piece)), matrix, piece))
	}
	status := "measured"
	var boundsValue any
	if bounds == nil {
		status = "empty_outline"
	} else {
		boundsValue = geometry.Floats(bounds)
	}
	return rec.Map{"use": int64(index), "path": id, "text": use.attrs["data-text"], "matrix": geometry.Floats(matrix),
		"bounds": boundsValue, "status": status}
}

// chain is the transform of an element and its ancestors up to the root.
func chain(node *element) []float64 {
	if node == nil {
		rec.Fail("Detached SVG glyph outline")
	}
	if node.chain != nil {
		return node.chain
	}
	if node.name == "svg" {
		if _, transformed := node.attrs["transform"]; transformed || !node.root {
			rec.Fail("Unsupported SVG root transform")
		}
		node.chain = geometry.Identity
		return node.chain
	}
	node.chain = geometry.Compose(chain(node.parent), parseTransform(node.attrs["transform"]))
	return node.chain
}
