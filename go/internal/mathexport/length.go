package mathexport

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

var lengthNumber = regexp.MustCompile(`\A([+-]?(?:\d+(?:\.\d*)?|\.\d+))(em|ex|px|pt|pc|in|cm|mm|mu|%|width|height|depth)?\z`)

var namedSpaces = map[string]int{"veryverythinmathspace": 1, "verythinmathspace": 2, "thinmathspace": 3,
	"mediummathspace": 4, "thickmathspace": 5, "verythickmathspace": 6, "veryverythickmathspace": 7}

// Length is MathLength: a MathML dimension as a CSS length. Relative mpadded metrics are
// retained as explicit operands rather than approximated by guessed font dimensions.
type Length struct {
	Value      float64
	Unit       string
	Adjustment bool
}

// errArgument is Ruby's ArgumentError (Float("1.")): it is no Relatex::Failure and is not
// rescued where Failures are.
var errArgument = errors.New("invalid value for Float()")

// NewLength is MathLength.new(source, padded:).
func NewLength(source string, padded bool) Length {
	source = rec.Strip(source)
	sign := 1.0
	if strings.HasPrefix(source, "negative") {
		sign = -1
	}
	if named, ok := namedSpaces[strings.TrimPrefix(source, "negative")]; ok {
		source = RubyFloat(sign*float64(named)/18.0) + "em"
	}
	match := lengthNumber.FindStringSubmatch(source)
	if match == nil {
		rec.Fail("Unsupported mathematical length: %s", source)
	}
	if strings.HasSuffix(match[1], ".") {
		panic(fmt.Errorf("%w: %q", errArgument, match[1]))
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		panic(fmt.Errorf("%w: %q", errArgument, match[1]))
	}
	if math.IsInf(value, 0) || math.IsNaN(value) {
		rec.Fail("Nonfinite mathematical length")
	}
	unit := match[2]
	if unit == "" && value == 0 {
		unit = "pt"
	}
	if unit == "" {
		rec.Fail("Nonzero mathematical length needs a unit")
	}
	return Length{Value: value, Unit: unit, Adjustment: padded && (strings.HasPrefix(source, "+") || strings.HasPrefix(source, "-"))}
}

var (
	texPoints = 72.27
	bigPoints = 72.0
)

// TeX is MathLength#tex.
func (l Length) TeX() string {
	amount, target := l.Value, l.Unit
	switch l.Unit {
	case "pt":
		amount, target = l.Value*texPoints/bigPoints, "pt"
	case "px":
		amount, target = l.Value*0.75*texPoints/bigPoints, "pt"
	case "pc":
		amount, target = l.Value*12*texPoints/bigPoints, "pt"
	case "em", "ex", "in", "cm", "mm", "mu":
	default:
		rec.Fail("Native TeX projection cannot preserve relative %s geometry", l.Unit)
	}
	return FormatG(amount, 8) + target
}

// Typst is MathLength#typst.
func (l Length) Typst() string {
	unit, err := canonjson.Marshal(l.Unit)
	if err != nil {
		rec.Fail("%v", err)
	}
	return fmt.Sprintf("(%s, %s, %t)", FormatG(l.Value, 10), unit, l.Adjustment)
}

// FormatG is C's printf %.Ng, correctly rounded (rec.FormatG).
func FormatG(value float64, precision int) string { return rec.FormatG(value, precision) }

// RubyFloat is Ruby's Float#to_s.
func RubyFloat(value float64) string {
	text, err := canonjson.Marshal(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return string(text)
}

// Quote is JSON.generate of a string.
func Quote(value string) string { return quote(value) }

// RubyValue is Ruby's to_s (string interpolation) of a JSON value.
func RubyValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	case canonjson.Integer:
		return string(typed)
	}
	text, err := canonjson.Marshal(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return string(text)
}
