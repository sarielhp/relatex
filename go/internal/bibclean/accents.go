package bibclean

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/texaccents"
)

// combiningAccents maps LaTeX accent commands (without leading backslash) to Unicode combining marks.
var combiningAccents = texaccents.Combining

// specialLetters maps argument-free LaTeX letter commands (without leading backslash) to Unicode.
var specialLetters = map[string]string{
	"aa": "å",
	"AA": "Å",
	"ae": "æ",
	"AE": "Æ",
	"oe": "œ",
	"OE": "Œ",
	"o":  "ø",
	"O":  "Ø",
	"l":  "ł",
	"L":  "Ł",
	"ss": "ß",
	"SS": "ẞ",
	"dh": "ð",
	"DH": "Ð",
	"dj": "đ",
	"DJ": "Đ",
	"ng": "ŋ",
	"NG": "Ŋ",
	"th": "þ",
	"TH": "Þ",
	"i":  "ı",
	"j":  "ȷ",
}

// textSymbols maps argument-free LaTeX text symbol commands (without leading backslash) to plain text.
var textSymbols = map[string]string{
	"textendash":        "–",
	"textemdash":        "—",
	"textquotedblleft":  "“",
	"textquotedblright": "”",
	"textquoteleft":     "‘",
	"textquoteright":    "’",
	"dots":              "…",
	"ldots":             "…",
	"copyright":         "©",
	"P":                 "¶",
	"S":                 "§",
	"dag":               "†",
	"ddag":              "‡",
	"slash":             "/",
}

// applyCombiningAccent attaches a Unicode combining mark to the first rune (or base) of text and returns NFC.
func applyCombiningAccent(mark, base string) string {
	switch base {
	case "ı":
		base = "i"
	case "ȷ":
		base = "j"
	}
	if base == "" {
		return ""
	}
	_, size := utf8.DecodeRuneInString(base)
	composed := base[:size] + mark + base[size:]
	return norm.NFC.String(composed)
}

func isSpecialCharCommand(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := combiningAccents[name]; ok {
		return true
	}
	if _, ok := specialLetters[name]; ok {
		return true
	}
	if _, ok := textSymbols[name]; ok {
		return true
	}
	if name == "textdollar" || name == "textquotesingle" || name == "nobreakspace" {
		return true
	}
	return len(name) == 1 && strings.ContainsRune("&%$#_{}", rune(name[0]))
}
