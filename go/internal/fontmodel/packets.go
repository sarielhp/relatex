package fontmodel

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// A virtual font's property list as the font bindings record it (SourceFontBinding): each
// CHARACTER packet, the MAP operations inside it and the one SETCHAR a plain packet holds.
var (
	// CharacterHead is the head of a CHARACTER packet: its slot as a property-list number.
	CharacterHead = regexp.MustCompile(`\A\(CHARACTER ([CODH]) ([^\s()]+)`)
	mapOperations = regexp.MustCompile(`(?s)\(MAP\s+(.*?)\)\s*\)\s*\z`)
	setChar       = regexp.MustCompile(`(?s)\A(?:\(SELECTFONT D 0\)\s*)?\(SETCHAR ([CODH]) ([^\s()]+)\)\s*\z`)
)

// VPLNumber is SourceFontBinding.vpl_number: a property-list number (C char, O octal, D
// decimal, H hex) that must be a slot.
func VPLNumber(kind, token string) int64 {
	value := int64(-1)
	var err error
	switch kind {
	case "C":
		if runes := []rune(token); len(runes) == 1 {
			value = int64(runes[0])
		}
	case "O":
		value, err = rubyInteger(token, 8)
	case "D":
		value, err = rubyInteger(token, 10)
	case "H":
		value, err = rubyInteger(token, 16)
	}
	if err != nil {
		rec.Fail("Malformed virtual font slot")
	}
	if value < 0 || value > 255 {
		rec.Fail("Unsupported virtual font slot")
	}
	return value
}

// rubyInteger is Integer(token, base): an optional sign, digits with single underscores
// between them.
func rubyInteger(token string, base int) (int64, error) {
	text := strings.TrimSpace(token)
	if strings.Contains(text, "__") || strings.HasPrefix(strings.TrimLeft(text, "+-"), "_") || strings.HasSuffix(text, "_") {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(strings.ReplaceAll(text, "_", ""), base, 64)
}

// PacketSlots is SourceFontBinding.packet_slots: the base-font slot each virtual packet
// sets, for packets that only select font 0 and set one character.
func PacketSlots(value any) rec.Map {
	packets, ok := value.(rec.Map)
	total := 0
	for _, packet := range packets {
		text, isString := packet.(string)
		ok = ok && isString
		total += len(text)
	}
	if !ok {
		rec.Fail("Invalid virtual font packets")
	}
	if total > 4_194_304 {
		rec.Fail("Virtual packets exceed size bound")
	}
	result := rec.Map{}
	for slot, item := range packets {
		packet := item.(string)
		header := CharacterHead.FindStringSubmatch(packet)
		if header == nil || strconv.FormatInt(VPLNumber(header[1], header[2]), 10) != slot ||
			strings.Count(packet, "(") != strings.Count(packet, ")") || strings.Count(packet, "(CHARACTER ") != 1 {
			rec.Fail("Virtual font packet identity differs")
		}
		operations := ""
		if match := mapOperations.FindStringSubmatch(packet); match != nil {
			operations = match[1]
		}
		match := setChar.FindStringSubmatch(operations)
		if match == nil {
			rec.Fail("Unsupported transformed or composite virtual glyph")
		}
		result[slot] = VPLNumber(match[1], match[2])
	}
	return result
}
