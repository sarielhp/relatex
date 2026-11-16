package xmltree

import "bytes"

// replacement is U+FFFD, the character a reader cannot name.
var replacement = []byte("�")

// Characters is NativeTrace.characters (the native trace, figure and page SVGs): mutool writes what a PDF's ToUnicode maps a glyph to,
// also code points XML cannot hold. A lone UTF-16 surrogate half comes as UTF-8 (third cohort
// 802), a control character as a character reference (&#x15; in a trace, &#x0015; in an SVG's
// data-text; fifth cohort 1457) or raw. Each reads as U+FFFD; data without one is returned as is.
func Characters(data []byte) []byte {
	var out []byte
	last := 0
	for index := 0; index < len(data); {
		size := unreadableAt(data, index)
		if size == 0 {
			index++
			continue
		}
		if out == nil {
			out = make([]byte, 0, len(data))
		}
		out = append(append(out, data[last:index]...), replacement...)
		index += size
		last = index
	}
	if out == nil {
		return data
	}
	return append(out, data[last:]...)
}

// unreadableAt is the length of the unreadable character at index, or 0.
func unreadableAt(data []byte, index int) int {
	switch value := data[index]; {
	case value == '&':
		return illegalReference(data[index:])
	case value < 0x20:
		if value == '\t' || value == '\n' || value == '\r' {
			return 0
		}
		return 1
	case value == 0xED:
		// A surrogate's UTF-8 form: 0xED then 0xA0-0xBF, then a continuation byte.
		if index+2 < len(data) && data[index+1]&0xE0 == 0xA0 && data[index+2]&0xC0 == 0x80 {
			return 3
		}
	}
	return 0
}

// illegalReference is the length of a character reference at the start of text that names a
// code point XML 1.0 does not allow, or 0 (also for no reference).
func illegalReference(text []byte) int {
	if !bytes.HasPrefix(text, []byte("&#")) {
		return 0
	}
	end := bytes.IndexByte(text[:min(len(text), 16)], ';')
	if end < 0 {
		return 0
	}
	digits, base := text[2:end], int64(10)
	if len(digits) > 0 && (digits[0] == 'x' || digits[0] == 'X') {
		digits, base = digits[1:], 16
	}
	value, ok := referenceValue(digits, base)
	if !ok || legalCharacter(value) {
		return 0
	}
	return end + 1
}

// referenceValue reads a reference's at most 12 digits, a large value as beyond Unicode.
func referenceValue(digits []byte, base int64) (int64, bool) {
	if len(digits) == 0 || len(digits) > 12 {
		return 0, false
	}
	value := int64(0)
	for _, digit := range digits {
		number := digitValue(digit)
		if number < 0 || number >= base {
			return 0, false
		}
		value = min(value*base+number, 0x110000)
	}
	return value, true
}

func digitValue(digit byte) int64 {
	switch {
	case digit >= '0' && digit <= '9':
		return int64(digit - '0')
	case digit >= 'a' && digit <= 'f':
		return int64(digit-'a') + 10
	case digit >= 'A' && digit <= 'F':
		return int64(digit-'A') + 10
	}
	return -1
}

// legalCharacter is XML 1.0's Char production.
func legalCharacter(value int64) bool {
	return value == 0x9 || value == 0xA || value == 0xD || (value >= 0x20 && value <= 0xD7FF) ||
		(value >= 0xE000 && value <= 0xFFFD) || (value >= 0x10000 && value <= 0x10FFFF)
}
