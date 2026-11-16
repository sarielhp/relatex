package pdffonts

import (
	"encoding/binary"
	"math"
)

// point is a position in em units (the font's coordinates over its units per em).
type point struct{ x, y float64 }

// glyphShape is what identifies a glyph without naming it: its advance width and the anchor
// points of its contours (the on-curve points, a TrueType curve's implied ones included, a
// CFF curve's end points), all in em units so that programs of different units per em compare.
type glyphShape struct {
	width    float64
	contours [][]point
}

// empty reports a glyph that draws nothing (a space).
func (s glyphShape) empty() bool { return len(s.contours) == 0 }

// outlineFont reads the glyph outlines of a TrueType program (glyf, loca, hmtx).
type outlineFont struct {
	tables  map[string][]byte
	upem    float64
	long    bool // loca holds 32-bit offsets
	glyphs  int
	metrics int // numberOfHMetrics
}

// parseOutlines reads the tables an outline match needs; an error for a program without glyf.
func parseOutlines(data []byte) (*outlineFont, error) {
	if len(data) < 12 {
		return nil, errTrueType
	}
	font := &outlineFont{tables: tableDirectory(data)}
	head, maxp, hhea := font.tables["head"], font.tables["maxp"], font.tables["hhea"]
	if len(head) < 54 || len(maxp) < 6 || len(hhea) < 36 || font.tables["glyf"] == nil || font.tables["loca"] == nil {
		return nil, errTrueType
	}
	font.upem = float64(binary.BigEndian.Uint16(head[18:]))
	font.long = binary.BigEndian.Uint16(head[50:]) != 0
	font.glyphs = int(binary.BigEndian.Uint16(maxp[4:]))
	font.metrics = int(binary.BigEndian.Uint16(hhea[34:]))
	if font.upem == 0 {
		return nil, errTrueType
	}
	return font, nil
}

// advance is a glyph's advance width in em units.
func (o *outlineFont) advance(gid int) float64 {
	hmtx := o.tables["hmtx"]
	if o.metrics == 0 || len(hmtx) < 4*o.metrics {
		return 0
	}
	return float64(binary.BigEndian.Uint16(hmtx[4*min(gid, o.metrics-1):])) / o.upem
}

// glyphData is a glyph's glyf entry, nil for an empty one or one outside the table.
func (o *outlineFont) glyphData(gid int) []byte {
	loca, glyf := o.tables["loca"], o.tables["glyf"]
	size := 2
	if o.long {
		size = 4
	}
	if gid < 0 || gid >= o.glyphs || len(loca) < size*(gid+2) {
		return nil
	}
	offset := func(i int) int {
		if o.long {
			return int(binary.BigEndian.Uint32(loca[4*i:]))
		}
		return 2 * int(binary.BigEndian.Uint16(loca[2*i:]))
	}
	from, to := offset(gid), offset(gid+1)
	if from >= to || to > len(glyf) {
		return nil
	}
	return glyf[from:to]
}

// shape is a glyph's shape; false for a glyph the program lacks or one this reader cannot
// follow (a component placed by matching points).
func (o *outlineFont) shape(gid int) (glyphShape, bool) {
	if gid < 0 || gid >= o.glyphs {
		return glyphShape{}, false
	}
	contours, ok := o.contours(gid, 0)
	if !ok {
		return glyphShape{}, false
	}
	shape := glyphShape{width: o.advance(gid)}
	for _, contour := range contours {
		anchors := make([]point, 0, len(contour))
		for _, p := range anchorPoints(contour) {
			anchors = append(anchors, point{p.x / o.upem, p.y / o.upem})
		}
		shape.contours = append(shape.contours, anchors)
	}
	return shape, true
}

// ttPoint is a TrueType outline point.
type ttPoint struct {
	x, y float64
	on   bool
}

// contours reads a glyph's contours in font units, components resolved.
func (o *outlineFont) contours(gid, depth int) ([][]ttPoint, bool) {
	data := o.glyphData(gid)
	if data == nil {
		return nil, true
	}
	if len(data) < 10 {
		return nil, false
	}
	count := int(int16(binary.BigEndian.Uint16(data)))
	if count >= 0 {
		return simpleContours(data, count)
	}
	if depth > 4 {
		return nil, false
	}
	return o.composite(data[10:], depth)
}

// simpleContours reads a simple glyph's points and splits them into its contours.
func simpleContours(data []byte, count int) ([][]ttPoint, bool) {
	end := 10 + 2*count
	if len(data) < end+2 {
		return nil, false
	}
	last := -1
	if count > 0 {
		last = int(binary.BigEndian.Uint16(data[end-2:]))
	}
	instructions := end + 2 + int(binary.BigEndian.Uint16(data[end:]))
	if instructions > len(data) {
		return nil, false
	}
	points, ok := simplePoints(data[instructions:], last+1)
	if !ok {
		return nil, false
	}
	var contours [][]ttPoint
	start := 0
	for c := 0; c < count; c++ {
		stop := int(binary.BigEndian.Uint16(data[10+2*c:])) + 1
		if stop <= start || stop > len(points) {
			return nil, false
		}
		contours = append(contours, points[start:stop])
		start = stop
	}
	return contours, true
}

// simplePoints reads n points from a simple glyph's flags and coordinates.
func simplePoints(data []byte, n int) ([]ttPoint, bool) {
	flags := make([]byte, 0, n)
	at := 0
	for len(flags) < n {
		if at >= len(data) {
			return nil, false
		}
		flag := data[at]
		at++
		flags = append(flags, flag)
		if flag&8 == 0 {
			continue
		}
		if at >= len(data) {
			return nil, false
		}
		for repeat := int(data[at]); repeat > 0 && len(flags) < n; repeat-- {
			flags = append(flags, flag)
		}
		at++
	}
	points := make([]ttPoint, n)
	var ok bool
	for axis := 0; axis < 2; axis++ {
		var value float64
		for i, flag := range flags {
			var delta float64
			if delta, at, ok = coordinateDelta(data, at, flag, axis); !ok {
				return nil, false
			}
			value += delta
			if axis == 0 {
				points[i].x, points[i].on = value, flag&1 != 0
			} else {
				points[i].y = value
			}
		}
	}
	return points, true
}

// coordinateDelta reads one coordinate of a point (axis 0 x, 1 y) from its flag.
func coordinateDelta(data []byte, at int, flag byte, axis int) (float64, int, bool) {
	short, same := byte(2<<axis), byte(16<<axis)
	switch {
	case flag&short != 0:
		if at >= len(data) {
			return 0, at, false
		}
		delta := float64(data[at])
		if flag&same == 0 {
			delta = -delta
		}
		return delta, at + 1, true
	case flag&same != 0:
		return 0, at, true
	}
	if at+2 > len(data) {
		return 0, at, false
	}
	return float64(int16(binary.BigEndian.Uint16(data[at:]))), at + 2, true
}

// composite reads a composite glyph's components, each moved and scaled as it says.
func (o *outlineFont) composite(data []byte, depth int) ([][]ttPoint, bool) {
	var all [][]ttPoint
	for more := true; more; {
		if len(data) < 4 {
			return nil, false
		}
		flags, component := binary.BigEndian.Uint16(data), int(binary.BigEndian.Uint16(data[2:]))
		dx, dy, used, ok := componentOffset(data[4:], flags)
		if !ok {
			return nil, false
		}
		matrix, used2, ok := componentMatrix(data[4+used:], flags)
		if !ok {
			return nil, false
		}
		data = data[4+used+used2:]
		more = flags&0x20 != 0
		inner, ok := o.contours(component, depth+1)
		if !ok {
			return nil, false
		}
		for _, contour := range inner {
			moved := make([]ttPoint, len(contour))
			for i, p := range contour {
				moved[i] = ttPoint{float64(matrix[0]*p.x) + float64(matrix[2]*p.y) + dx,
					float64(matrix[1]*p.x) + float64(matrix[3]*p.y) + dy, p.on}
			}
			all = append(all, moved)
		}
	}
	return all, true
}

// componentOffset reads a component's two arguments; ok is false for point matching.
func componentOffset(data []byte, flags uint16) (dx, dy float64, used int, ok bool) {
	if flags&2 == 0 {
		return 0, 0, 0, false
	}
	if flags&1 != 0 {
		if len(data) < 4 {
			return 0, 0, 0, false
		}
		return float64(int16(binary.BigEndian.Uint16(data))), float64(int16(binary.BigEndian.Uint16(data[2:]))), 4, true
	}
	if len(data) < 2 {
		return 0, 0, 0, false
	}
	return float64(int8(data[0])), float64(int8(data[1])), 2, true
}

// componentMatrix reads a component's scale (a, b, c, d as x' = a x + c y, y' = b x + d y).
func componentMatrix(data []byte, flags uint16) ([4]float64, int, bool) {
	f2 := func(i int) float64 { return float64(int16(binary.BigEndian.Uint16(data[2*i:]))) / 16384 }
	switch {
	case flags&8 != 0 && len(data) >= 2:
		return [4]float64{f2(0), 0, 0, f2(0)}, 2, true
	case flags&0x40 != 0 && len(data) >= 4:
		return [4]float64{f2(0), 0, 0, f2(1)}, 4, true
	case flags&0x80 != 0 && len(data) >= 8:
		return [4]float64{f2(0), f2(1), f2(2), f2(3)}, 8, true
	case flags&(8|0x40|0x80) != 0:
		return [4]float64{}, 0, false
	}
	return [4]float64{1, 0, 0, 1}, 0, true
}

// anchorPoints are the on-curve points of a closed contour, with the implied on-curve point
// between two consecutive off-curve points added.
func anchorPoints(contour []ttPoint) []ttPoint {
	n := len(contour)
	if n == 0 {
		return nil
	}
	var anchors []ttPoint
	for i, p := range contour {
		if p.on {
			anchors = append(anchors, p)
			continue
		}
		next := contour[(i+1)%n]
		if !next.on {
			anchors = append(anchors, ttPoint{(p.x + next.x) / 2, (p.y + next.y) / 2, true})
		}
	}
	return anchors
}

// shapesAlike reports whether two shapes draw the same glyph: the same number of contours and
// every contour of one equals a distinct contour of the other (same anchors in the same cyclic
// order, either direction) within tolerance em.
func shapesAlike(a, b glyphShape, tolerance float64) bool {
	if len(a.contours) != len(b.contours) {
		return false
	}
	used := make([]bool, len(b.contours))
	for _, contour := range a.contours {
		found := false
		for j, other := range b.contours {
			if !used[j] && contoursAlike(contour, other, tolerance) {
				used[j], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// contoursAlike compares two closed point sequences up to rotation and direction.
func contoursAlike(a, b []point, tolerance float64) bool {
	a, b = dropClosing(a, tolerance), dropClosing(b, tolerance)
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for _, reversed := range []bool{false, true} {
		for shift := range b {
			if sequenceAlike(a, b, shift, reversed, tolerance) {
				return true
			}
		}
	}
	return false
}

// dropClosing removes a last point that repeats the first (a contour closed by a line).
func dropClosing(points []point, tolerance float64) []point {
	if n := len(points); n > 1 && near(points[0], points[n-1], tolerance) {
		return points[:n-1]
	}
	return points
}

// sequenceAlike compares a with b read from shift, forwards or backwards.
func sequenceAlike(a, b []point, shift int, reversed bool, tolerance float64) bool {
	n := len(a)
	for i, p := range a {
		j := (shift + i) % n
		if reversed {
			j = ((shift-i)%n + n) % n
		}
		if !near(p, b[j], tolerance) {
			return false
		}
	}
	return true
}

func near(p, q point, tolerance float64) bool {
	return math.Abs(p.x-q.x) <= tolerance && math.Abs(p.y-q.y) <= tolerance
}
