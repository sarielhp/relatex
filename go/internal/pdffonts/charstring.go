package pdffonts

import "encoding/binary"

// charstring runs one Type 2 charstring (Adobe Technical Note 5177) far enough to collect the
// anchor points of its contours and its advance width.
type charstring struct {
	private  cffPrivate
	global   [][]byte
	stack    []float64
	stems    int
	width    float64
	seen     bool // the width operand was read (or known absent)
	x, y     float64
	contours [][]point
	current  []point
	finished bool
}

// exec runs a charstring (or subroutine, depth > 0).
func (c *charstring) exec(code []byte, depth int) error {
	if depth > 10 {
		return errCharstring
	}
	for at := 0; at < len(code) && !c.finished; {
		b0 := int(code[at])
		if b0 >= 32 || b0 == 28 {
			value, next, ok := charNumber(code, at)
			if !ok {
				return errCharstring
			}
			c.stack, at = append(c.stack, value), next
			continue
		}
		at++
		if b0 == 11 {
			return nil
		}
		next, err := c.operator(code, at, b0, depth)
		if err != nil {
			return err
		}
		at = next
	}
	return nil
}

// charNumber reads a number operand (28, 32..255).
func charNumber(code []byte, at int) (float64, int, bool) {
	b0 := int(code[at])
	byteAt := func(i int) int {
		if at+i < len(code) {
			return int(code[at+i])
		}
		return 0
	}
	switch {
	case b0 == 28 && at+3 <= len(code):
		return float64(int16(byteAt(1)<<8 | byteAt(2))), at + 3, true
	case b0 >= 32 && b0 <= 246:
		return float64(b0 - 139), at + 1, true
	case b0 >= 247 && b0 <= 250 && at+2 <= len(code):
		return float64((b0-247)*256 + byteAt(1) + 108), at + 2, true
	case b0 >= 251 && b0 <= 254 && at+2 <= len(code):
		return float64(-(b0-251)*256 - byteAt(1) - 108), at + 2, true
	case b0 == 255 && at+5 <= len(code):
		return float64(int32(binary.BigEndian.Uint32(code[at+1:]))) / 65536, at + 5, true
	}
	return 0, at, false
}

// operator runs operator b0 (its code starts at at) and returns where the next one starts.
func (c *charstring) operator(code []byte, at, b0, depth int) (int, error) {
	switch b0 {
	case 1, 3, 18, 23: // stems
		c.takeWidth(len(c.stack)%2 == 1)
		c.stems += len(c.stack) / 2
		c.stack = c.stack[:0]
	case 19, 20: // hintmask, cntrmask: implicit vstems, then the mask bytes
		c.takeWidth(len(c.stack)%2 == 1)
		c.stems += len(c.stack) / 2
		c.stack = c.stack[:0]
		return min(at+(c.stems+7)/8, len(code)), nil
	case 21, 22, 4:
		c.moveto(b0)
	case 10, 29:
		return at, c.call(b0, depth)
	case 14:
		c.takeWidth(len(c.stack) == 1 || len(c.stack) == 5)
		c.finished = true
	case 12:
		return at + 1, errCharstring // flex, arithmetic: not read
	default:
		return at, c.path(b0)
	}
	return at, nil
}

// takeWidth reads the width operand when the operator has one more operand than it needs.
func (c *charstring) takeWidth(extra bool) {
	if c.seen {
		return
	}
	c.seen = true
	if extra && len(c.stack) > 0 {
		c.width = c.private.nominalWidth + c.stack[0]
		c.stack = c.stack[1:]
	}
}

// moveto starts a contour (rmoveto 21, hmoveto 22, vmoveto 4).
func (c *charstring) moveto(op int) {
	need := map[int]int{21: 2, 22: 1, 4: 1}[op]
	c.takeWidth(len(c.stack) > need)
	if len(c.stack) < need {
		c.stack = c.stack[:0]
		return
	}
	args := c.stack[len(c.stack)-need:]
	switch op {
	case 21:
		c.x, c.y = c.x+args[0], c.y+args[1]
	case 22:
		c.x += args[0]
	default:
		c.y += args[0]
	}
	c.close()
	c.current = []point{{c.x, c.y}}
	c.stack = c.stack[:0]
}

// close ends the open contour.
func (c *charstring) close() {
	if len(c.current) > 0 {
		c.contours = append(c.contours, c.current)
		c.current = nil
	}
}

// call runs a local (10) or global (29) subroutine.
func (c *charstring) call(op, depth int) error {
	if len(c.stack) == 0 {
		return errCharstring
	}
	subrs := c.private.local
	if op == 29 {
		subrs = c.global
	}
	index := int(c.stack[len(c.stack)-1]) + subrBias(len(subrs))
	c.stack = c.stack[:len(c.stack)-1]
	if index < 0 || index >= len(subrs) {
		return errCharstring
	}
	return c.exec(subrs[index], depth+1)
}

// subrBias is the bias added to a subroutine number.
func subrBias(count int) int {
	switch {
	case count < 1240:
		return 107
	case count < 33900:
		return 1131
	}
	return 32768
}

// lineTo adds an anchor at a relative step.
func (c *charstring) lineTo(dx, dy float64) {
	c.x, c.y = c.x+dx, c.y+dy
	c.current = append(c.current, point{c.x, c.y})
}

// path runs a path-drawing operator; an unknown one is an error.
func (c *charstring) path(op int) error {
	args := c.stack
	defer func() { c.stack = c.stack[:0] }()
	switch op {
	case 5: // rlineto
		for i := 0; i+2 <= len(args); i += 2 {
			c.lineTo(args[i], args[i+1])
		}
	case 6, 7: // hlineto, vlineto
		horizontal := op == 6
		for _, step := range args {
			if horizontal {
				c.lineTo(step, 0)
			} else {
				c.lineTo(0, step)
			}
			horizontal = !horizontal
		}
	case 8, 24, 25, 26, 27, 30, 31:
		return c.curves(op, args)
	default:
		return errCharstring
	}
	return nil
}

// curves runs the curve operators; only each curve's end point is an anchor.
func (c *charstring) curves(op int, args []float64) error {
	switch op {
	case 8: // rrcurveto
		c.relativeCurves(args)
	case 24: // rcurveline
		if len(args) < 2 {
			return errCharstring
		}
		c.relativeCurves(args[:len(args)-2])
		c.lineTo(args[len(args)-2], args[len(args)-1])
	case 25: // rlinecurve
		if len(args) < 6 {
			return errCharstring
		}
		for i := 0; i+2 <= len(args)-6; i += 2 {
			c.lineTo(args[i], args[i+1])
		}
		c.relativeCurves(args[len(args)-6:])
	case 26, 27: // vvcurveto, hhcurveto
		c.straightCurves(args, op == 27)
	default: // hvcurveto 31, vhcurveto 30
		c.alternatingCurves(args, op == 31)
	}
	return nil
}

func (c *charstring) relativeCurves(args []float64) {
	for i := 0; i+6 <= len(args); i += 6 {
		c.lineTo(args[i]+args[i+2]+args[i+4], args[i+1]+args[i+3]+args[i+5])
	}
}

// straightCurves is hhcurveto (horizontal) or vvcurveto: curves leaving and arriving straight.
func (c *charstring) straightCurves(args []float64, horizontal bool) {
	var lead float64
	if len(args)%4 == 1 {
		lead, args = args[0], args[1:]
	}
	for i := 0; i+4 <= len(args); i += 4 {
		if horizontal { // dxa dxb dyb dxc
			c.lineTo(args[i]+args[i+1]+args[i+3], args[i+2]+lead)
		} else { // dya dxb dyb dyc
			c.lineTo(args[i+1]+lead, args[i]+args[i+2]+args[i+3])
		}
		lead = 0
	}
}

// alternatingCurves is hvcurveto (start horizontal) or vhcurveto: curves that alternate; the last
// may have a fifth operand that sets the end tangent.
func (c *charstring) alternatingCurves(args []float64, horizontal bool) {
	for len(args) >= 4 {
		extra := 0.0
		if len(args) == 5 {
			extra = args[4]
		}
		if horizontal {
			c.lineTo(args[0]+args[1]+extra, args[2]+args[3])
		} else {
			c.lineTo(args[1]+args[3], args[0]+args[2]+extra)
		}
		horizontal = !horizontal
		args = args[min(4, len(args)):]
	}
}
