// Package rec holds the dynamic JSON values the stages pass around (the records of the TeX
// stream, the paper document), read the way the Ruby reference reads its hashes: a missing
// key or a value of the wrong type is a failure of the evidence, as Ruby's fetch and its
// Relatex::Failure are. Failures travel as a panic of type Failure inside a package and are
// turned into an error at its boundary with Recover, the way encoding/json reports errors
// from deep inside a decoder; nothing outside these packages sees the panic.
package rec

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
)

// Map is a JSON object.
type Map = map[string]any

// Failure is the Relatex::Failure of the Ruby reference: a rejected input, with its message.
// Lookup marks the failures Ruby raises as KeyError or IndexError (a missing key, an index
// outside an array) rather than as Relatex::Failure; some callers wrap only those.
type Failure struct {
	Message string
	Lookup  bool
}

func (f Failure) Error() string { return f.Message }

// Fail stops the current stage with a Failure.
func Fail(format string, args ...any) {
	panic(Failure{Message: fmt.Sprintf(format, args...)})
}

// Recover turns a Failure panic into *err; any other panic continues.
func Recover(err *error) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, ok := recovered.(Failure)
	if !ok {
		panic(recovered)
	}
	*err = failure
}

// Fetch returns m[key], failing as Ruby's Hash#fetch does when the key is missing.
func Fetch(m Map, key string) any {
	value, ok := m[key]
	if !ok {
		panic(Failure{Message: fmt.Sprintf("key not found: %q", key), Lookup: true})
	}
	return value
}

// Index returns list[i], failing as Ruby's Array#fetch does outside the array.
func Index(list []any, i int) any {
	if i < 0 || i >= len(list) {
		panic(Failure{Message: fmt.Sprintf("index %d outside of array bounds: %d...%d", i, -len(list), len(list)), Lookup: true})
	}
	return list[i]
}

// FetchMap fetches an object; FetchList, FetchString and FetchInt fetch a key and require its type.
func FetchMap(m Map, key string) Map { return AsMap(Fetch(m, key)) }

// FetchList fetches an array.
func FetchList(m Map, key string) []any { return AsList(Fetch(m, key)) }

// FetchString fetches a string.
func FetchString(m Map, key string) string { return AsString(Fetch(m, key)) }

// FetchInt fetches an integer.
func FetchInt(m Map, key string) int64 { return AsInt(Fetch(m, key)) }

// AsMap requires an object.
func AsMap(value any) Map {
	m, ok := value.(Map)
	if !ok {
		Fail("expected an object, got %T", value)
	}
	return m
}

// AsList requires an array.
func AsList(value any) []any {
	list, ok := value.([]any)
	if !ok {
		Fail("expected an array, got %T", value)
	}
	return list
}

// AsString requires a string.
func AsString(value any) string {
	text, ok := value.(string)
	if !ok {
		Fail("expected a string, got %T", value)
	}
	return text
}

// AsInt requires an integer that fits an int64.
func AsInt(value any) int64 {
	number, ok := value.(int64)
	if !ok {
		Fail("expected an integer, got %T", value)
	}
	return number
}

// IsInt reports whether a value is a Ruby Integer (of any size).
func IsInt(value any) bool {
	switch value.(type) {
	case int64, canonjson.Integer:
		return true
	}
	return false
}

// Positive reports whether a value is an Integer above zero (Ruby's is_a?(Integer) && positive?).
func Positive(value any) bool {
	number, ok := value.(int64)
	if ok {
		return number > 0
	}
	big, ok := value.(canonjson.Integer)
	return ok && big[0] != '-'
}

// Number returns a numeric value as a float64 and whether it is numeric.
func Number(value any) (float64, bool) {
	switch typed := value.(type) {
	case int64:
		return float64(typed), true
	case canonjson.Float:
		return float64(typed), true
	case float64:
		return typed, true
	}
	return 0, false
}

// Equal is Ruby's == on JSON values: numbers compare by value (1 == 1.0), objects and
// arrays element by element.
func Equal(left, right any) bool {
	if a, ok := Number(left); ok {
		b, isNumber := Number(right)
		return isNumber && a == b && !math.IsNaN(a)
	}
	switch typed := left.(type) {
	case Map:
		other, ok := right.(Map)
		return ok && equalMaps(typed, other)
	case []any:
		other, ok := right.([]any)
		return ok && equalLists(typed, other)
	}
	return left == right
}

func equalMaps(left, right Map) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		other, ok := right[key]
		if !ok || !Equal(value, other) {
			return false
		}
	}
	return true
}

func equalLists(left, right []any) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}

// rubySpace is the whitespace Ruby's String#strip removes (NUL included, on both sides).
const rubySpace = " \t\n\v\f\r\x00"

// Strip is Ruby's String#strip.
func Strip(text string) string { return strings.Trim(text, rubySpace) }

// LStrip is Ruby's String#lstrip.
func LStrip(text string) string { return strings.TrimLeft(text, rubySpace) }

// RStrip is Ruby's String#rstrip.
func RStrip(text string) string { return strings.TrimRight(text, rubySpace) }

// Identity is a map's identity, for the Ruby hashes compared by identity
// (compare_by_identity, equal?): the address of the map, which Go never moves.
func Identity(m Map) uintptr { return reflect.ValueOf(m).Pointer() }

// SameKeys is Ruby's m.keys.sort == names.sort.
func SameKeys(m Map, names []string) bool {
	if len(m) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := m[name]; !ok {
			return false
		}
	}
	return true
}
