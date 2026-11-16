package pdffonts

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
)

// Type1 is what a Type 1 font program says of itself: its name, its built-in encoding, its
// glyphs' advance widths and the metrics a font descriptor needs.
type Type1 struct {
	Name        string
	Encoding    map[int]string     // the built-in encoding; StandardEncoding's names when it says so
	Widths      map[string]float64 // glyph name -> advance width in text space units (1000 per em)
	BBox        [4]float64
	ItalicAngle float64
	FixedPitch  bool
	StemV       float64
	// The program as a PDF FontFile stream holds it: clear text, encrypted binary, trailer.
	Clear, Binary, Trailer []byte
}

var (
	errNotType1   = errors.New("not a Type 1 font program")
	clearName     = regexp.MustCompile(`/FontName\s*/(\S+)`)
	clearBBox     = regexp.MustCompile(`/FontBBox\s*[\[{]\s*(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)`)
	clearAngle    = regexp.MustCompile(`/ItalicAngle\s+(-?[\d.]+)`)
	clearFixed    = regexp.MustCompile(`/isFixedPitch\s+true`)
	clearMatrix   = regexp.MustCompile(`/FontMatrix\s*[\[{]\s*(-?[\d.eE-]+)`)
	clearSlot     = regexp.MustCompile(`dup\s+(\d+)\s*/([^\s/\[\]{}()<>]+)\s+put`)
	privateLenIV  = regexp.MustCompile(`/lenIV\s+(\d+)`)
	privateStemV  = regexp.MustCompile(`/StdVW\s*\[\s*([\d.]+)`)
	charStringsAt = regexp.MustCompile(`/CharStrings\s+\d+`)
)

// ParseType1 reads a Type 1 program: a PFB file, a PFA file, or a PDF FontFile stream's data
// (clear text, binary, trailer; lengths gives Length1 and Length2 when known, else 0).
func ParseType1(data []byte, length1, length2 int) (*Type1, error) {
	plain, encrypted, trailer, err := splitProgram(data, length1, length2)
	if err != nil {
		return nil, err
	}
	font := &Type1{Clear: plain, Binary: encrypted, Trailer: trailer, Encoding: programEncoding(plain)}
	if match := clearName.FindSubmatch(plain); match != nil {
		font.Name = string(match[1])
	}
	if match := clearBBox.FindSubmatch(plain); match != nil {
		for index := range font.BBox {
			font.BBox[index], _ = strconv.ParseFloat(string(match[index+1]), 64)
		}
	}
	if match := clearAngle.FindSubmatch(plain); match != nil {
		font.ItalicAngle, _ = strconv.ParseFloat(string(match[1]), 64)
	}
	font.FixedPitch = clearFixed.Match(plain)
	scale := 1.0
	if match := clearMatrix.FindSubmatch(plain); match != nil {
		if value, err := strconv.ParseFloat(string(match[1]), 64); err == nil && value > 0 {
			scale = value * 1000
		}
	}
	private := decrypt(binaryOf(encrypted), 55665, 4)
	if match := privateStemV.FindSubmatch(private); match != nil {
		font.StemV, _ = strconv.ParseFloat(string(match[1]), 64)
	}
	font.Widths = charStringWidths(private, scale)
	return font, nil
}

// programEncoding is the clear text's encoding: its "dup CODE /NAME put" entries, or
// StandardEncoding's names when it names that.
func programEncoding(plain []byte) map[int]string {
	if bytes.Contains(plain, []byte("/Encoding StandardEncoding")) {
		return standardEncoding()
	}
	codes := map[int]string{}
	for _, match := range clearSlot.FindAllSubmatch(plain, -1) {
		if code, err := strconv.Atoi(string(match[1])); err == nil && code < 256 {
			codes[code] = string(match[2])
		}
	}
	return codes
}

// splitProgram cuts a program into its clear text, its encrypted part and its trailer.
func splitProgram(data []byte, length1, length2 int) (plain, encrypted, trailer []byte, err error) {
	if len(data) > 6 && data[0] == 0x80 {
		return splitPFB(data)
	}
	if length1 <= 0 || length1 > len(data) {
		at := bytes.Index(data, []byte("eexec"))
		if at < 0 {
			return nil, nil, nil, errNotType1
		}
		// One end of line follows eexec; the encrypted bytes may begin with any byte.
		length1 = at + len("eexec")
		if bytes.HasPrefix(data[length1:], []byte("\r\n")) {
			length1 += 2
		} else if length1 < len(data) && (data[length1] == '\r' || data[length1] == '\n') {
			length1++
		}
	}
	end := trailerStart(data, length1)
	if length2 > 0 && length1+length2 <= len(data) {
		end = length1 + length2
	}
	return data[:length1], data[length1:end], data[end:], nil
}

// trailerStart is where a program's trailer begins: the lines of zeros before its cleartomark
// (512 of them), or the end of the data without one.
func trailerStart(data []byte, from int) int {
	mark := bytes.LastIndex(data, []byte("cleartomark"))
	if mark < from {
		return len(data)
	}
	start := mark
	for start > from {
		line := bytes.LastIndexAny(data[from:start-1], "\r\n") + from + 1
		zeros := bytes.Trim(data[line:start], "\r\n")
		if len(zeros) == 0 || len(bytes.Trim(zeros, "0")) != 0 {
			break
		}
		start = line
	}
	return start
}

// splitPFB reads a PFB file's segments: ASCII (1), binary (2), end (3).
func splitPFB(data []byte) (plain, encrypted, trailer []byte, err error) {
	segment := 0
	for at := 0; at+2 <= len(data) && data[at] == 0x80 && data[at+1] != 3; segment++ {
		if at+6 > len(data) {
			return nil, nil, nil, errNotType1
		}
		size := int(binary.LittleEndian.Uint32(data[at+2 : at+6]))
		if at+6+size > len(data) {
			return nil, nil, nil, errNotType1
		}
		part := data[at+6 : at+6+size]
		switch {
		case segment == 0:
			plain = part
		case data[at+1] == 2:
			encrypted = append(encrypted, part...)
		default:
			trailer = append(trailer, part...)
		}
		at += 6 + size
	}
	if plain == nil || encrypted == nil {
		return nil, nil, nil, errNotType1
	}
	return plain, encrypted, trailer, nil
}

// binaryOf is the encrypted part as bytes: hexadecimal text (PFA) is decoded.
func binaryOf(encrypted []byte) []byte {
	if trimmed := bytes.TrimLeft(encrypted, " \t\r\n"); len(trimmed) < 4 || !isHexDigits(trimmed[:4]) {
		return encrypted
	}
	digits := make([]byte, 0, len(encrypted))
	for _, c := range encrypted {
		if isHexDigits([]byte{c}) {
			digits = append(digits, c)
		}
	}
	decoded := make([]byte, len(digits)/2)
	n, _ := hex.Decode(decoded, digits[:len(decoded)*2])
	return decoded[:n]
}

func isHexDigits(data []byte) bool {
	for _, c := range data {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// decrypt is Type 1 decryption with key r, the first skip bytes dropped.
func decrypt(cipher []byte, r uint16, skip int) []byte {
	plain := make([]byte, len(cipher))
	for index, c := range cipher {
		plain[index] = c ^ byte(r>>8)
		r = (uint16(c)+r)*52845 + 22719
	}
	if len(plain) < skip {
		return nil
	}
	return plain[skip:]
}

// charStringWidths reads every glyph's advance width from its charstring's hsbw or sbw.
func charStringWidths(private []byte, scale float64) map[string]float64 {
	widths := map[string]float64{}
	lenIV := 4
	if match := privateLenIV.FindSubmatch(private); match != nil {
		lenIV, _ = strconv.Atoi(string(match[1]))
	}
	start := charStringsAt.FindIndex(private)
	if start == nil {
		return widths
	}
	reader := &charStringReader{data: private, at: start[1]}
	for {
		name, glyph, ok := reader.next()
		if !ok {
			return widths
		}
		if width, found := glyphWidth(decrypt(glyph, 4330, lenIV)); found {
			widths[name] = width * scale
		}
	}
}

// charStringReader walks the CharStrings dictionary: /name LENGTH RD <bytes> ND.
type charStringReader struct {
	data []byte
	at   int
}

func (r *charStringReader) next() (string, []byte, bool) {
	slash := bytes.IndexByte(r.data[r.at:], '/')
	end := bytes.Index(r.data[r.at:], []byte("end"))
	if slash < 0 || (end >= 0 && end < slash) {
		return "", nil, false
	}
	r.at += slash + 1
	fields := r.fields(2)
	if len(fields) < 2 {
		return "", nil, false
	}
	length, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", nil, false
	}
	token := r.fields(1)
	if len(token) == 0 || r.at+1+length > len(r.data) {
		return "", nil, false
	}
	glyph := r.data[r.at+1 : r.at+1+length]
	r.at += 1 + length
	return fields[0], glyph, true
}

// fields reads count whitespace-separated words from the reader's position.
func (r *charStringReader) fields(count int) []string {
	var words []string
	for len(words) < count && r.at < len(r.data) {
		for r.at < len(r.data) && isSpace(r.data[r.at]) {
			r.at++
		}
		start := r.at
		for r.at < len(r.data) && !isSpace(r.data[r.at]) {
			r.at++
		}
		if start == r.at {
			break
		}
		words = append(words, string(r.data[start:r.at]))
	}
	return words
}

func isSpace(c byte) bool { return c == ' ' || c == '\n' || c == '\r' || c == '\t' }

// glyphWidth runs a charstring until its hsbw or sbw: the advance width.
func glyphWidth(code []byte) (float64, bool) {
	var stack []float64
	for at := 0; at < len(code); {
		v := int(code[at])
		at++
		switch {
		case v >= 32 && v <= 246:
			stack = append(stack, float64(v-139))
		case v >= 247 && v <= 254 && at < len(code):
			w := int(code[at])
			at++
			if v <= 250 {
				stack = append(stack, float64((v-247)*256+w+108))
			} else {
				stack = append(stack, float64(-(v-251)*256-w-108))
			}
		case v == 255 && at+4 <= len(code):
			stack = append(stack, float64(int32(binary.BigEndian.Uint32(code[at:at+4]))))
			at += 4
		case v == 13 && len(stack) >= 2: // hsbw: sbx wx
			return stack[1], true
		case v == 12 && at < len(code):
			width, done, ok := escape(code[at], stack)
			at++
			if done || !ok {
				return width, ok
			}
			stack = stack[:len(stack)-1]
			stack[len(stack)-1] = width
		default:
			return 0, false
		}
	}
	return 0, false
}

// escape runs a two-byte operator before the side bearing: sbw (done, the width) or div (its
// quotient replaces the last two operands).
func escape(operator byte, stack []float64) (value float64, done, ok bool) {
	switch {
	case operator == 7 && len(stack) >= 4: // sbw: sbx sby wx wy
		return stack[2], true, true
	case operator == 12 && len(stack) >= 2 && stack[len(stack)-1] != 0: // div
		return stack[len(stack)-2] / stack[len(stack)-1], false, true
	}
	return 0, true, false
}
