// Package rubynum is Ruby's arithmetic on JSON numbers: an int64 (Integer) or a float64 (Float).
// Integer arithmetic stays Integer (division floors), anything with a Float is a Float, as the
// Ruby reference computed the geometry the outputs print. The HTML writer and the table layout of
// the document model share it.
package rubynum

import (
	"fmt"
	"math"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Num normalizes a JSON number to int64 or float64.
func Num(value any) any {
	switch typed := value.(type) {
	case int64:
		return typed
	case float64:
		return typed
	case canonjson.Float:
		return float64(typed)
	case int:
		return int64(typed)
	}
	panic(fmt.Errorf("expected a number, got %T", value))
}

// Float is a number as a float64.
func Float(value any) float64 {
	number, ok := rec.Number(Num(value))
	if !ok {
		panic(fmt.Errorf("expected a number, got %T", value))
	}
	return number
}

func ints(a, b any) (int64, int64, bool) {
	x, okX := Num(a).(int64)
	y, okY := Num(b).(int64)
	return x, y, okX && okY
}

// Add is Ruby's +.
func Add(a, b any) any {
	if x, y, ok := ints(a, b); ok {
		return x + y
	}
	return Float(a) + Float(b)
}

// Sub is Ruby's -.
func Sub(a, b any) any {
	if x, y, ok := ints(a, b); ok {
		return x - y
	}
	return Float(a) - Float(b)
}

// Mul is Ruby's *.
func Mul(a, b any) any {
	if x, y, ok := ints(a, b); ok {
		return x * y
	}
	return Float(a) * Float(b)
}

// Div is Ruby's /: Integer division floors.
func Div(a, b any) any {
	if x, y, ok := ints(a, b); ok {
		if y == 0 {
			panic(fmt.Errorf("divided by 0"))
		}
		quotient := x / y
		if (x%y != 0) && ((x < 0) != (y < 0)) {
			quotient--
		}
		return quotient
	}
	return Float(a) / Float(b)
}

// Fdiv is Ruby's fdiv.
func Fdiv(a, b any) float64 { return Float(a) / Float(b) }

// Less is Ruby's <.
func Less(a, b any) bool {
	if x, y, ok := ints(a, b); ok {
		return x < y
	}
	return Float(a) < Float(b)
}

// Positive is Ruby's positive?.
func Positive(a any) bool { return Less(int64(0), a) }

// Minimum is [a, b].min: a on a tie.
func Minimum(a, b any) any {
	if Less(b, a) {
		return b
	}
	return a
}

// Maximum is [a, b].max: a on a tie.
func Maximum(a, b any) any {
	if Less(a, b) {
		return b
	}
	return a
}

// Abs is Ruby's abs.
func Abs(a any) any {
	if x, ok := Num(a).(int64); ok {
		if x < 0 {
			return -x
		}
		return x
	}
	return math.Abs(Float(a))
}

// Sum is Array#sum: Integers add exactly; from the first Float on, Kahan-Babuska
// compensated summation, as Ruby does.
func Sum(values []any) any {
	var total int64
	index := 0
	for ; index < len(values); index++ {
		value, ok := Num(values[index]).(int64)
		if !ok {
			break
		}
		total += value
	}
	if index == len(values) {
		return total
	}
	f, c := float64(total), 0.0
	for ; index < len(values); index++ {
		x := Float(values[index])
		if math.IsNaN(f) {
			continue
		}
		if math.IsNaN(x) {
			f = x
			continue
		}
		if math.IsInf(x, 0) {
			if math.IsInf(f, 0) && math.Signbit(x) != math.Signbit(f) {
				f = math.NaN()
			} else {
				f = x
			}
			continue
		}
		if math.IsInf(f, 0) {
			continue
		}
		t := f + x
		if math.Abs(f) >= math.Abs(x) {
			c += (f - t) + x
		} else {
			c += (x - t) + f
		}
		f = t
	}
	return f + c
}

// RoundDigits is Numeric#round(digits) for digits > 0: an Integer is unchanged; a Float is
// rounded half away from zero by Ruby's round_half_up.
func RoundDigits(value any, digits int) any {
	number, isFloat := Num(value).(float64)
	if !isFloat {
		return value
	}
	if number == 0 || math.IsInf(number, 0) || math.IsNaN(number) {
		return number
	}
	return rec.Round(number, digits)
}
