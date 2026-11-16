package authormath

import (
	"strings"
	"unicode/utf8"
)

// Scrub is Ruby's String#scrub on UTF-8 bytes: each maximal invalid subsequence (a lead byte
// and the continuation bytes that could still complete it) becomes one U+FFFD.
func Scrub(bytes []byte) string {
	if utf8.Valid(bytes) {
		return string(bytes)
	}
	var builder strings.Builder
	for index := 0; index < len(bytes); {
		character, width := utf8.DecodeRune(bytes[index:])
		if character != utf8.RuneError || width > 1 {
			builder.Write(bytes[index : index+width])
			index += width
			continue
		}
		builder.WriteRune(utf8.RuneError)
		index += invalidLength(bytes[index:])
	}
	return builder.String()
}

// invalidLength is the length of the maximal subpart at the start of bytes: a lead byte
// followed by the continuation bytes valid in their position, short of a whole character.
func invalidLength(bytes []byte) int {
	lead := bytes[0]
	need, low, high := 0, byte(0x80), byte(0xBF)
	switch {
	case lead >= 0xC2 && lead <= 0xDF:
		need = 1
	case lead == 0xE0:
		need, low = 2, 0xA0
	case lead == 0xED:
		need, high = 2, 0x9F
	case lead >= 0xE1 && lead <= 0xEF:
		need = 2
	case lead == 0xF0:
		need, low = 3, 0x90
	case lead == 0xF4:
		need, high = 3, 0x8F
	case lead >= 0xF1 && lead <= 0xF3:
		need = 3
	default:
		return 1
	}
	length := 1
	for length <= need && length < len(bytes) {
		next := bytes[length]
		if length > 1 {
			low, high = 0x80, 0xBF
		}
		if next < low || next > high {
			break
		}
		length++
	}
	return length
}
