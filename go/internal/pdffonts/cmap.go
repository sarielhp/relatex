package pdffonts

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ToUnicode CMaps: read into code -> UTF-16BE hex, and written back as bfchar entries.

var (
	cmapDigits = strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "\f", "")
	cmapBlock  = regexp.MustCompile(`(?s)begin(bfchar|bfrange)(.*?)endbf(?:char|range)`)
	cmapHex    = regexp.MustCompile(`<([0-9A-Fa-f\s]*)>|\[|\]`)
	cmapSpace  = regexp.MustCompile(`(?s)begincodespacerange\s*<([0-9A-Fa-f]*)>`)
)

// ReadCMap reads a ToUnicode CMap: the codes it maps with their UTF-16 hex, and its code length
// in bytes (from its first code space range; 1 without one).
func ReadCMap(cmap string) (map[int]string, int) {
	mapped := map[int]string{}
	width := 1
	if match := cmapSpace.FindStringSubmatch(cmap); match != nil && len(match[1]) >= 4 {
		width = len(match[1]) / 2
	}
	for _, block := range cmapBlock.FindAllStringSubmatch(cmap, -1) {
		var tokens []string
		for _, match := range cmapHex.FindAllStringSubmatch(block[2], -1) {
			if match[0] == "[" || match[0] == "]" {
				tokens = append(tokens, match[0])
				continue
			}
			tokens = append(tokens, cmapDigits.Replace(match[1])) // a destination may split its units: <d835 dc5c>
		}
		if block[1] == "bfchar" {
			readChars(tokens, mapped)
		} else {
			readRanges(tokens, mapped)
		}
	}
	return mapped, width
}

func readChars(tokens []string, mapped map[int]string) {
	for i := 0; i+1 < len(tokens); i += 2 {
		if code, err := strconv.ParseUint(tokens[i], 16, 32); err == nil {
			mapped[int(code)] = strings.ToUpper(tokens[i+1])
		}
	}
}

// readRanges reads bfrange entries: <lo> <hi> <dst> (dst's last byte counting up) or
// <lo> <hi> [<d>...].
func readRanges(tokens []string, mapped map[int]string) {
	for i := 0; i+2 < len(tokens); {
		lo, errLo := strconv.ParseUint(tokens[i], 16, 32)
		hi, errHi := strconv.ParseUint(tokens[i+1], 16, 32)
		if errLo != nil || errHi != nil || hi < lo || hi-lo > 0xFFFF {
			return
		}
		if tokens[i+2] != "[" {
			rangeValues(lo, hi, tokens[i+2], mapped)
			i += 3
			continue
		}
		j := i + 3
		for code := lo; j < len(tokens) && tokens[j] != "]"; code, j = code+1, j+1 {
			mapped[int(code)] = strings.ToUpper(tokens[j])
		}
		i = j + 1
	}
}

// rangeValues maps lo..hi to dst counting up in its last 16-bit unit.
func rangeValues(lo, hi uint64, dst string, mapped map[int]string) {
	if len(dst) < 4 {
		return
	}
	prefix, last := dst[:len(dst)-4], dst[len(dst)-4:]
	base, err := strconv.ParseUint(last, 16, 32)
	if err != nil {
		return
	}
	for code := lo; code <= hi; code++ {
		mapped[int(code)] = strings.ToUpper(prefix) + fmt.Sprintf("%04X", (base+code-lo)&0xFFFF)
	}
}

// WriteCMap writes a ToUnicode CMap of codes width bytes long, as bfchar entries.
func WriteCMap(mapped map[int]string, width int) string {
	var out strings.Builder
	digits := 2 * max(width, 1)
	out.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
		"/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n1 begincodespacerange\n")
	fmt.Fprintf(&out, "<%s> <%s>\nendcodespacerange\n", strings.Repeat("0", digits), strings.Repeat("F", digits))
	codes := slices.Sorted(maps.Keys(mapped))
	for start := 0; start < len(codes); start += 100 {
		block := codes[start:min(start+100, len(codes))]
		fmt.Fprintf(&out, "%d beginbfchar\n", len(block))
		for _, code := range block {
			fmt.Fprintf(&out, "<%0*X> <%s>\n", digits, code, mapped[code])
		}
		out.WriteString("endbfchar\n")
	}
	out.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return out.String()
}

// Unusable reports whether a ToUnicode value is one PDF/UA-2 rejects or reads as unmapped: empty,
// U+0000, U+FEFF, U+FFFE, or a private-use character (8.4.5.8, 8.4.3).
func Unusable(value string) bool {
	if value == "" || len(value)%4 != 0 {
		return true
	}
	for at := 0; at < len(value); at += 4 {
		unit, err := strconv.ParseUint(value[at:at+4], 16, 16)
		if err != nil || unit == 0 || unit == 0xFEFF || unit == 0xFFFE || (unit >= 0xE000 && unit <= 0xF8FF) {
			return true
		}
		if unit >= 0xDB80 && unit <= 0xDBFF { // high surrogates of planes 15 and 16: private use
			return true
		}
	}
	return false
}
