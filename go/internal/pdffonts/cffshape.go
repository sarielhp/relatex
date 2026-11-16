package pdffonts

import (
	"encoding/binary"
	"errors"
	"strconv"
)

// The outlines of a bare CFF CID program (a figure's FontFile3 subset), read as glyph shapes
// (outline.go) so that a glyph can be matched with the same glyph of an installed font. Only the
// parts a match needs: charset, CharStrings, the Private dictionaries' widths and subroutines,
// FontMatrix and Type 2 path operators (no flex, no seac).

var errCharstring = errors.New("cff: unsupported charstring")

// cffOutlines reads the glyphs of a CID-keyed CFF program.
type cffOutlines struct {
	charStrings [][]byte
	cidGlyph    map[int]int // CID -> glyph index
	global      [][]byte
	selectFD    func(gid int) int
	privates    []cffPrivate
	scale       float64 // the Top DICT FontMatrix x scale, 0 when it has none
}

// cffPrivate is one font dictionary's widths, subroutines and FontMatrix scale.
type cffPrivate struct {
	defaultWidth, nominalWidth float64
	local                      [][]byte
	scale                      float64 // its FontMatrix's x scale, 0 when it has none
}

// parseCFFOutlines reads a CID-keyed CFF program; an error for another kind.
func parseCFFOutlines(data []byte) (*cffOutlines, error) {
	r := &cffReader{data: data}
	if len(data) < 4 {
		return nil, errNotCFF
	}
	_, at, err := r.index(int(data[2])) // Name INDEX
	if err != nil {
		return nil, err
	}
	tops, at, err := r.index(at)
	if err != nil || len(tops) != 1 {
		return nil, errNotCFF
	}
	_, at, err = r.index(at) // String INDEX
	if err != nil {
		return nil, err
	}
	global, _, err := r.index(at)
	if err != nil {
		return nil, err
	}
	top := cffDict(tops[0])
	font := &cffOutlines{global: global}
	if err := font.readTop(r, top); err != nil {
		return nil, err
	}
	return font, nil
}

// readTop reads CharStrings, charset, FDArray and FDSelect from the Top DICT.
func (o *cffOutlines) readTop(r *cffReader, top map[int][]float64) error {
	if len(top[1230]) == 0 || len(top[17]) == 0 {
		return errNotCFF // not a CIDFont, or no glyphs
	}
	var err error
	if o.charStrings, _, err = r.index(int(top[17][0])); err != nil {
		return err
	}
	o.cidGlyph = cffCharset(r.data, int(first0(top[15])), len(o.charStrings))
	if len(top[1236]) == 0 {
		return errNotCFF
	}
	dicts, _, err := r.index(int(top[1236][0]))
	if err != nil {
		return err
	}
	for _, dict := range dicts {
		private, err := readPrivate(r, cffDict(dict))
		if err != nil {
			return err
		}
		o.privates = append(o.privates, private)
	}
	o.selectFD = fdSelector(r.data, int(first0(top[1237])))
	o.scale = fontScale(top[1207])
	return nil
}

func first0(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

// fontScale is the x scale of a FontMatrix operand list, 0 for none.
func fontScale(matrix []float64) float64 {
	if len(matrix) == 6 {
		return matrix[0]
	}
	return 0
}

// readPrivate reads a font dictionary's Private DICT.
func readPrivate(r *cffReader, fd map[int][]float64) (cffPrivate, error) {
	private := cffPrivate{scale: fontScale(fd[1207])}
	span := fd[18]
	if len(span) != 2 {
		return private, nil
	}
	size, offset := int(span[0]), int(span[1])
	if size < 0 || offset < 0 || offset+size > len(r.data) {
		return private, errNotCFF
	}
	dict := cffDict(r.data[offset : offset+size])
	private.defaultWidth, private.nominalWidth = first0(dict[20]), first0(dict[21])
	if len(dict[19]) > 0 {
		local, _, err := r.index(offset + int(dict[19][0]))
		if err != nil {
			return private, err
		}
		private.local = local
	}
	return private, nil
}

// cffCharset maps each CID to its glyph index (glyph 0 is CID 0).
func cffCharset(data []byte, at, count int) map[int]int {
	cids := map[int]int{0: 0}
	if at < 3 || at >= len(data) {
		return cids
	}
	format, pos := data[at], at+1
	for gid := 1; gid < count; {
		switch {
		case format == 0 && pos+2 <= len(data):
			cids[int(binary.BigEndian.Uint16(data[pos:]))] = gid
			gid, pos = gid+1, pos+2
		case format > 0 && format < 3 && pos+3 <= len(data):
			start, left := int(binary.BigEndian.Uint16(data[pos:])), int(data[pos+2])
			pos += 3
			if format == 2 && pos < len(data) {
				left, pos = left<<8|int(data[pos]), pos+1
			}
			for i := 0; i <= left && gid < count; i++ {
				cids[start+i] = gid
				gid++
			}
		default:
			return cids
		}
	}
	return cids
}

// fdSelector returns the font dictionary of a glyph (format 0 and 3 of FDSelect).
func fdSelector(data []byte, at int) func(int) int {
	if at < 3 || at >= len(data) {
		return func(int) int { return 0 }
	}
	switch data[at] {
	case 0:
		return func(gid int) int {
			if at+1+gid < len(data) {
				return int(data[at+1+gid])
			}
			return 0
		}
	case 3:
		return func(gid int) int { return rangedFD(data, at, gid) }
	}
	return func(int) int { return 0 }
}

// rangedFD reads FDSelect format 3: the font dictionary of the range holding gid.
func rangedFD(data []byte, at, gid int) int {
	if at+3 > len(data) {
		return 0
	}
	ranges, chosen := int(binary.BigEndian.Uint16(data[at+1:])), 0
	for i := 0; i < ranges && at+3+3*i+3 <= len(data); i++ {
		entry := data[at+3+3*i:]
		if int(binary.BigEndian.Uint16(entry)) > gid {
			break
		}
		chosen = int(entry[2])
	}
	return chosen
}

// shape is a glyph's shape; false when its charstring uses what this reader skips.
func (o *cffOutlines) shape(gid int) (glyphShape, bool) {
	if gid < 0 || gid >= len(o.charStrings) {
		return glyphShape{}, false
	}
	fd := min(o.selectFD(gid), len(o.privates)-1)
	if fd < 0 {
		return glyphShape{}, false
	}
	private := o.privates[fd]
	scale := matrixScale(o.scale, private.scale)
	run := &charstring{private: private, global: o.global, width: private.defaultWidth}
	if err := run.exec(o.charStrings[gid], 0); err != nil {
		return glyphShape{}, false
	}
	run.close()
	shape := glyphShape{width: run.width * scale}
	for _, contour := range run.contours {
		anchors := make([]point, len(contour))
		for i, p := range contour {
			anchors[i] = point{p.x * scale, p.y * scale}
		}
		shape.contours = append(shape.contours, anchors)
	}
	return shape, true
}

// cffDict reads a DICT: operands as numbers by operator (a two-byte operator 12 n is 1200+n).
func cffDict(dict []byte) map[int][]float64 {
	ops, operands := map[int][]float64{}, []float64{}
	for at := 0; at < len(dict); {
		b0 := int(dict[at])
		switch {
		case b0 == 12 && at+1 < len(dict):
			ops[1200+int(dict[at+1])], operands, at = operands, nil, at+2
		case b0 <= 21:
			ops[b0], operands, at = operands, nil, at+1
		case b0 == 30:
			value, next := cffReal(dict, at+1)
			operands, at = append(operands, value), next
		default:
			value, next := cffInteger(dict, at)
			operands, at = append(operands, float64(value)), next
		}
	}
	return ops
}

// cffReal reads a real operand's nibbles.
func cffReal(dict []byte, at int) (float64, int) {
	text := ""
	for ; at < len(dict); at++ {
		for _, nibble := range []byte{dict[at] >> 4, dict[at] & 15} {
			switch {
			case nibble <= 9:
				text += string(rune('0' + nibble))
			case nibble == 10:
				text += "."
			case nibble == 11:
				text += "E"
			case nibble == 12:
				text += "E-"
			case nibble == 14:
				text += "-"
			case nibble == 15:
				value, _ := strconv.ParseFloat(text, 64) // a malformed real reads 0
				return value, at + 1
			}
		}
	}
	return 0, at
}

// matrixScale is the font units to em scale of the two FontMatrix scales (0: none given): their
// product, 0.001 when neither is given.
func matrixScale(top, fd float64) float64 {
	switch {
	case top == 0 && fd == 0:
		return 0.001
	case top == 0:
		return fd
	case fd == 0:
		return top
	}
	return top * fd
}
