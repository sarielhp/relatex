package tables

import (
	"fmt"
	"math"
	"slices"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// indexError is Ruby's IndexError (Array#fetch outside the array): unlike KeyError, the
// table evidence does not rescue it, so it is not a rec.Failure.
type indexError struct{ message string }

func (e indexError) Error() string { return e.message }

// fetchIndex is Array#fetch(i), raising IndexError.
func fetchIndex(list []any, index int64) any {
	if index < 0 {
		index += int64(len(list))
	}
	if index < 0 || index >= int64(len(list)) {
		panic(indexError{fmt.Sprintf("index %d outside of array bounds: %d...%d", index, -len(list), len(list))})
	}
	return list[index]
}

// rescue is Ruby's rescue Failure, KeyError, TypeError: fn's failure message, or "" when
// it completed. Any other panic (an IndexError) continues.
func rescue(fn func()) (message string, failed bool) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		failure, ok := recovered.(rec.Failure)
		if !ok {
			panic(recovered)
		}
		message, failed = failure.Message, true
	}()
	fn()
	return "", false
}

// key is a Ruby Hash key for JSON values: values that are eql? share it (1 and 1.0 do not).
func key(values ...any) string {
	bytes, err := canonjson.Marshal(values)
	if err != nil {
		rec.Fail("%v", err)
	}
	return string(bytes)
}

// missing raises Ruby's KeyError for a Hash#fetch of name.
func missing(name any) {
	panic(rec.Failure{Message: "key not found: " + tablemodel.Inspect(name), Lookup: true})
}

func truthy(value any) bool { return value != nil && value != false }

func isInt(value any) bool {
	_, ok := value.(int64)
	return ok
}

// number is a numeric value as a float64; anything else is Ruby's TypeError.
func number(value any) float64 {
	result, ok := rec.Number(value)
	if !ok {
		rec.Fail("%s can't be coerced into Integer", tablemodel.Inspect(value))
	}
	return result
}

// add, sub and the comparisons are Ruby's numeric operators: Integer with Integer stays an
// Integer, anything with a Float is a Float.
func add(left, right any) any {
	a, aInt := left.(int64)
	b, bInt := right.(int64)
	if aInt && bInt {
		return a + b
	}
	return number(left) + number(right)
}

func sub(left, right any) any {
	a, aInt := left.(int64)
	b, bInt := right.(int64)
	if aInt && bInt {
		return a - b
	}
	return number(left) - number(right)
}

func less(left, right any) bool { return number(left) < number(right) }

func positive(value any) bool { return number(value) > 0 }

func isZero(value any) bool { return number(value) == 0 }

// half is value / 2: floor division for an Integer, as Ruby divides.
func half(value any) any {
	if integer, ok := value.(int64); ok {
		return floorDiv(integer, 2)
	}
	return number(value) / 2
}

func floorDiv(value, divisor int64) int64 {
	quotient := value / divisor
	if (value%divisor != 0) && ((value < 0) != (divisor < 0)) {
		quotient--
	}
	return quotient
}

// maximum and minimum are Array#max and #min: the first of equal extremes; nil when empty.
func maximum(values []any) any {
	var result any
	for _, value := range values {
		if result == nil || number(value) > number(result) {
			result = value
		}
	}
	return result
}

func minimum(values []any) any {
	var result any
	for _, value := range values {
		if result == nil || number(value) < number(result) {
			result = value
		}
	}
	return result
}

// fdivRound is Integer#fdiv then Float#round: the nearest Integer, halves away from zero.
func fdivRound(value any, divisor int) int64 {
	return int64(math.Round(number(value) / float64(divisor)))
}

func fetchInt(m rec.Map, name string) int64 { return rec.AsInt(rec.Fetch(m, name)) }

func fetchMap(m rec.Map, name string) rec.Map { return rec.AsMap(rec.Fetch(m, name)) }

// fetchDefault is Hash#fetch(name, default).
func fetchDefault(m rec.Map, name string, fallback any) any {
	if value, ok := m[name]; ok {
		return value
	}
	return fallback
}

func maps(values []any) []rec.Map {
	result := make([]rec.Map, len(values))
	for index, value := range values {
		result[index] = rec.AsMap(value)
	}
	return result
}

func values(m rec.Map, names ...string) []any {
	result := make([]any, len(names))
	for index, name := range names {
		result[index] = m[name]
	}
	return result
}

// copyMap is Hash#dup: a shallow copy.
func copyMap(m rec.Map) rec.Map {
	result := make(rec.Map, len(m))
	for name, value := range m {
		result[name] = value
	}
	return result
}

// except is Hash#except.
func except(m rec.Map, names ...string) rec.Map {
	result := rec.Map{}
	for name, value := range m {
		if !slices.Contains(names, name) {
			result[name] = value
		}
	}
	return result
}

// slice is Hash#slice.
func slice(m rec.Map, names ...string) rec.Map {
	result := rec.Map{}
	for _, name := range names {
		if value, ok := m[name]; ok {
			result[name] = value
		}
	}
	return result
}

func anyList[T any](values []T) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func compareNumbers(a, b any) int {
	x, y := number(a), number(b)
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// compareLists is Array#<=> on lists of numbers.
func compareLists(a, b []any) int {
	for index := 0; index < len(a) && index < len(b); index++ {
		if order := compareNumbers(a[index], b[index]); order != 0 {
			return order
		}
	}
	return len(a) - len(b)
}

// contains is Array#include? (==).
func contains(list []any, value any) bool {
	return slices.ContainsFunc(list, func(item any) bool { return rec.Equal(item, value) })
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
