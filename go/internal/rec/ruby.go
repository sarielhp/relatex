package rec

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/canonjson"
)

// ToS is Ruby's to_s (string interpolation) of a JSON value: nil is empty, a string itself,
// an array or object its inspect.
func ToS(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any, Map:
		return Inspect(value)
	}
	return scalar(value)
}

// Inspect is Ruby 3.3's inspect of a JSON value: {"a"=>1, "b"=>[nil, true]}.
func Inspect(value any) string {
	switch typed := value.(type) {
	case nil:
		return "nil"
	case string:
		return InspectString(typed)
	case []any:
		parts := make([]string, len(typed))
		for index, item := range typed {
			parts[index] = Inspect(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case Map:
		// A parsed document's keys are sorted (stage files are written canonically).
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for index, key := range keys {
			parts[index] = InspectString(key) + "=>" + Inspect(typed[key])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return scalar(value)
}

func scalar(value any) string {
	switch typed := value.(type) {
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case canonjson.Integer:
		return string(typed)
	}
	text, err := canonjson.Marshal(value)
	if err != nil {
		Fail("%v", err)
	}
	return string(text)
}

// InspectString is Ruby's String#inspect for a UTF-8 string.
func InspectString(text string) string { return `"` + escaped(text, false) + `"` }

// DumpString is Ruby's String#dump: non-ASCII characters as \u escapes.
func DumpString(text string) string { return `"` + escaped(text, true) + `"` }

func escaped(text string, dump bool) string {
	var builder strings.Builder
	for index, character := range text {
		switch {
		case character == utf8.RuneError && !strings.HasPrefix(text[index:], "\uFFFD"):
			fmt.Fprintf(&builder, `\x%02X`, text[index]) // an invalid byte
		case character == '"' || character == '\\':
			builder.WriteByte('\\')
			builder.WriteRune(character)
		case character == '#' && index+1 < len(text) && strings.IndexByte("{$@", text[index+1]) >= 0:
			builder.WriteString(`\#`)
		case character < 0x20 || character == 0x7f:
			builder.WriteString(controlEscape(character, dump))
		case character >= 0x80 && (dump || !printable(character)):
			builder.WriteString(unicodeEscape(character))
		default:
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

// controlEscape is how inspect writes a control character of a UTF-8 string (\u0000) and how
// dump writes it (\x00); both use the named escapes.
func controlEscape(character rune, dump bool) string {
	names := map[rune]string{'\n': `\n`, '\t': `\t`, '\r': `\r`, '\f': `\f`, '\v': `\v`, '\b': `\b`, '\a': `\a`, 0x1b: `\e`}
	if name, ok := names[character]; ok {
		return name
	}
	if dump {
		return fmt.Sprintf(`\x%02X`, character)
	}
	return fmt.Sprintf(`\u%04X`, character)
}

func unicodeEscape(character rune) string {
	if character > 0xFFFF {
		return fmt.Sprintf(`\u{%X}`, character)
	}
	return fmt.Sprintf(`\u%04X`, character)
}

// Node is nodes.fetch(id) on a document's nodes, failing as Ruby's KeyError does.
func Node(nodes Map, id any) Map {
	key := ToS(id)
	node, ok := nodes[key].(Map)
	if !ok {
		panic(Failure{Message: "key not found: " + InspectString(key), Lookup: true})
	}
	return node
}

// WithoutBlankLines is gsub(/\n[ \t]*(?=\n)/) with nothing: whitespace-only lines removed.
func WithoutBlankLines(text string) string {
	var builder strings.Builder
	for index := 0; index < len(text); {
		if text[index] == '\n' {
			next := index + 1
			for next < len(text) && (text[next] == ' ' || text[next] == '\t') {
				next++
			}
			if next < len(text) && text[next] == '\n' {
				index = next
				continue
			}
		}
		builder.WriteByte(text[index])
		index++
	}
	return builder.String()
}

// printable is Onigmo's print class as String#inspect uses it on UTF-8: everything assigned
// except the line and paragraph separators (control characters are escaped before).
func printable(character rune) bool {
	if unicode.Is(unicode.Zl, character) || unicode.Is(unicode.Zp, character) {
		return false
	}
	return unicode.In(character, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Zs, unicode.Cf, unicode.Co)
}

// Round is Ruby's Float#round(digits) for 0 < digits <= 14 (round half away from zero, at
// the decimal digits a float holds), as rb_flo_round_by_double computes it.
func Round(number float64, digits int) float64 {
	if number == 0 || math.IsInf(number, 0) || math.IsNaN(number) {
		return number
	}
	_, binexp := math.Frexp(number)
	if roundOverflow(digits, binexp) {
		return number
	}
	if roundUnderflow(digits, binexp) {
		return 0
	}
	if digits > 14 {
		panic(fmt.Errorf("round(%d) by rational is not ported", digits))
	}
	scale := math.Pow(10, float64(digits))
	return roundHalfUp(number, scale) / scale
}

func roundOverflow(digits, binexp int) bool {
	const floatDig = 17 // DBL_DIG + 2
	limit := binexp/3 - 1
	if binexp > 0 {
		limit = binexp / 4
	}
	return digits >= floatDig-limit
}

func roundUnderflow(digits, binexp int) bool {
	limit := binexp / 4
	if binexp > 0 {
		limit = binexp/3 + 1
	}
	return digits < -limit
}

func roundHalfUp(x, s float64) float64 {
	f := math.Round(x * s)
	if s == 1.0 {
		return f
	}
	if x > 0 {
		if (f+0.5)/s <= x {
			f++
		}
		return f
	}
	if (f-0.5)/s >= x {
		f--
	}
	return f
}

// FormatG is C's printf %.Ng, correctly rounded, as Relatex::DecimalFormat.g prints a float.
func FormatG(value float64, precision int) string {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return map[bool]string{true: "Inf", false: "NaN"}[math.IsInf(value, 0)]
	}
	if precision == 0 {
		precision = 1
	}
	scientific := strconv.FormatFloat(value, 'e', precision-1, 64)
	mantissa, exponentText, _ := strings.Cut(scientific, "e")
	exponent, _ := strconv.Atoi(exponentText)
	if exponent < -4 || exponent >= precision {
		mantissa = trimZeros(mantissa)
		sign := "+"
		if exponent < 0 {
			sign, exponent = "-", -exponent
		}
		return fmt.Sprintf("%se%s%02d", mantissa, sign, exponent)
	}
	return trimZeros(strconv.FormatFloat(value, 'f', precision-1-exponent, 64))
}

func trimZeros(text string) string {
	if !strings.Contains(text, ".") {
		return text
	}
	return strings.TrimSuffix(strings.TrimRight(text, "0"), ".")
}

// Extname is Ruby's File.extname: a leading dot alone is no extension.
func Extname(path string) string {
	base := strings.TrimLeft(filepath.Base(path), ".")
	if index := strings.LastIndexByte(base, '.'); index > 0 && index < len(base)-1 {
		return base[index:]
	}
	return ""
}
