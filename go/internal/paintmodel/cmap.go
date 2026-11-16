package paintmodel

import (
	"encoding/base64"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// The ToUnicode CMaps of PDFFontEncoding: their bfchar and bfrange blocks, the slots they map
// and the inherited or CID maps the proof rejects.
var (
	cmapHeader = regexp.MustCompile(`(\d+)\s+begin(bfchar|bfrange)`)
	cmapPair   = regexp.MustCompile(`<([0-9a-fA-F]+)>\s*<([0-9a-fA-F]+)>`)
	cmapTriple = regexp.MustCompile(`<([0-9a-fA-F]+)>\s*<([0-9a-fA-F]+)>\s*<([0-9a-fA-F]+)>`)
	// ObjectRef is a qpdf object reference ("obj:12 0 R").
	ObjectRef = regexp.MustCompile(`\Aobj:\d+ \d+ R\z`)
	slotText  = regexp.MustCompile(`\A(?:0|[1-9]\d{0,2})\z`)
	inherited = regexp.MustCompile(`(?i)usecmap|begincidchar|begincidrange`)
)

// validateMaps is PDFFontEncoding.validate_maps!.
func validateMaps(value any) {
	maps, ok := value.(rec.Map)
	if !ok {
		rec.Fail("Invalid PDF font encoding inventory")
	}
	for name, item := range maps {
		mapping, isMap := item.(rec.Map)
		if name == "" || FontName(name) != name || !isMap || len(mapping) == 0 {
			rec.Fail("Invalid PDF font encoding face")
		}
		for slot, value := range mapping {
			text, isString := value.(string)
			valid := slotText.MatchString(slot) && rec.ToI(slot) <= 255 && isString && utf8.ValidString(text) && text != ""
			if !valid {
				rec.Fail("Invalid PDF font slot encoding")
			}
		}
	}
}

// parseCMap is PDFFontEncoding.parse_cmap: a Type1 font's ToUnicode CMap as slot to text.
func parseCMap(data []byte) rec.Map {
	if len(data) > 1_048_576 || !ascii(data) {
		rec.Fail("PDF CMap is too large or not ASCII")
	}
	text := string(data)
	if inherited.MatchString(text) {
		rec.Fail("Inherited or CID CMaps are unsupported")
	}
	result := rec.Map{}
	blocks := cmapBlocks(text)
	if len(blocks) == 0 {
		rec.Fail("Missing or unsupported PDF Unicode CMap")
	}
	for _, block := range blocks {
		pattern := cmapTriple
		if block.kind == "bfchar" {
			pattern = cmapPair
		}
		entries := pattern.FindAllStringSubmatch(block.body, -1)
		consumed := rec.Strip(pattern.ReplaceAllString(block.body, ""))
		count, err := strconv.ParseInt(block.count, 10, 64)
		if consumed != "" || err != nil || int64(len(entries)) != count {
			rec.Fail("Unsupported PDF Unicode CMap block")
		}
		for _, entry := range entries {
			addMapping(result, block.kind, entry[1:])
		}
	}
	return result
}

func ascii(data []byte) bool {
	for _, character := range data {
		if character >= 0x80 {
			return false
		}
	}
	return true
}

type cmapBlock struct{ count, kind, body string }

func rubySpace(character byte) bool {
	return character == ' ' || character == '\t' || character == '\n' || character == '\v' || character == '\f' || character == '\r'
}

// cmapBlocks is text.scan(/(\d+)\s+begin(bfchar|bfrange)\s+(.*?)\s+end\2/m), without the
// backreference RE2 lacks.
func cmapBlocks(text string) []cmapBlock {
	result := []cmapBlock{}
	at := 0
	for at < len(text) {
		match := cmapHeader.FindStringSubmatchIndex(text[at:])
		if match == nil {
			break
		}
		start := at + match[0]
		block, end, ok := cmapBody(text, at+match[1], text[at+match[2]:at+match[3]], text[at+match[4]:at+match[5]])
		if !ok {
			at = start + 1
			continue
		}
		result = append(result, block)
		at = end
	}
	return result
}

// cmapBody reads \s+(.*?)\s+endKIND after the header ending at from: the body ends at the
// first whitespace run followed by endKIND; with a single whitespace run the body is empty.
func cmapBody(text string, from int, count, kind string) (cmapBlock, int, bool) {
	after := from
	for after < len(text) && rubySpace(text[after]) {
		after++
	}
	if after == from {
		return cmapBlock{}, 0, false
	}
	closing := "end" + kind
	for q := after; q < len(text); q++ {
		if !rubySpace(text[q]) {
			continue
		}
		run := q
		for run < len(text) && rubySpace(text[run]) {
			run++
		}
		if strings.HasPrefix(text[run:], closing) {
			return cmapBlock{count: count, kind: kind, body: text[after:q]}, run + len(closing), true
		}
		q = run - 1
	}
	if after-from >= 2 && strings.HasPrefix(text[after:], closing) {
		return cmapBlock{count: count, kind: kind, body: ""}, after + len(closing), true
	}
	return cmapBlock{}, 0, false
}

func addMapping(result rec.Map, kind string, entry []string) {
	first := hexInteger(entry[0])
	last := first
	if kind != "bfchar" {
		last = hexInteger(entry[1])
	}
	target := entry[len(entry)-1]
	if first.Sign() < 0 || first.Cmp(big.NewInt(255)) > 0 || last.Cmp(first) < 0 || last.Cmp(big.NewInt(255)) > 0 {
		rec.Fail("Unsupported PDF Type1 slot range")
	}
	if len(target) == 0 || len(target)%4 != 0 {
		rec.Fail("Invalid PDF Unicode target")
	}
	base := hexInteger(target)
	for slot := first.Int64(); slot <= last.Int64(); slot++ {
		code := new(big.Int).Add(base, big.NewInt(slot-first.Int64())).Text(16)
		if len(code) < len(target) {
			code = strings.Repeat("0", len(target)-len(code)) + code
		}
		text := utf16Text(code)
		key := strconv.FormatInt(slot, 10)
		if previous, ok := result[key]; ok && previous != text {
			rec.Fail("Conflicting PDF Unicode slot")
		}
		result[key] = text
	}
}

func hexInteger(text string) *big.Int {
	value, ok := new(big.Int).SetString(text, 16)
	if !ok {
		rec.Fail("Invalid PDF Unicode slot: invalid value for Integer(): %q", text)
	}
	return value
}

// utf16Text is [code].pack('H*') read as UTF-16BE and encoded as UTF-8.
func utf16Text(code string) string {
	if len(code)%2 == 1 {
		code += "0"
	}
	data := make([]byte, len(code)/2)
	for index := range data {
		value, _ := strconv.ParseUint(code[2*index:2*index+2], 16, 8)
		data[index] = byte(value)
	}
	if len(data)%2 == 1 {
		rec.Fail("Invalid PDF Unicode slot: incomplete \"\\x%02X\" on UTF-16BE", data[len(data)-1])
	}
	units := make([]uint16, len(data)/2)
	for index := range units {
		units[index] = uint16(data[2*index])<<8 | uint16(data[2*index+1])
	}
	for index := 0; index < len(units); index++ {
		unit := units[index]
		if utf16.IsSurrogate(rune(unit)) {
			if unit >= 0xDC00 || index+1 >= len(units) || units[index+1] < 0xDC00 || units[index+1] > 0xDFFF {
				rec.Fail("Invalid PDF Unicode slot: invalid surrogate in UTF-16BE")
			}
			index++
		}
	}
	return string(utf16.Decode(units))
}

func decodeBase64(text string) []byte {
	data, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil || strings.ContainsAny(text, "\r\n") {
		rec.RaiseArgument("invalid base64")
	}
	return data
}

// rescueArgumentOnly is rescue ArgumentError: an argumentError becomes a Failure with
// prefix; a lookup failure (Ruby's KeyError) continues.
func rescueArgumentOnly(prefix string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(rec.ArgumentError); ok {
		panic(rec.Failure{Message: prefix + failure.Message})
	}
	panic(recovered)
}
