package tablemodel

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Root is the repository root, where the table contracts (schemas/*.json) are read; the
// stage binary sets it before projecting tables.
var Root string

var (
	contractsMutex sync.Mutex
	contracts      = map[string]rec.Map{}
)

// Contract is a schema of schemas/, read once per root.
func Contract(name string) rec.Map {
	contractsMutex.Lock()
	defer contractsMutex.Unlock()
	path := filepath.Join(Root, "schemas", name)
	if schema, ok := contracts[path]; ok {
		return schema
	}
	file, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("tables: read contract: %v", err)) // Ruby reads it at load time.
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		panic(fmt.Sprintf("tables: parse contract: %v", err))
	}
	contracts[path] = rec.AsMap(value)
	return contracts[path]
}

// indexError is Ruby's IndexError (Array#fetch outside the array): unlike KeyError, the
// table evidence does not rescue it, so it is not a rec.Failure.
type indexError struct{ message string }

func (e indexError) Error() string { return e.message }

// key is a Ruby Hash key for JSON values: values that are eql? share it (1 and 1.0 do not).
func key(values ...any) string {
	bytes, err := canonjson.Marshal(values)
	if err != nil {
		rec.Fail("%v", err)
	}
	return string(bytes)
}

// Inspect is Ruby's #inspect for the scalar keys of a KeyError message.
func Inspect(value any) string {
	switch typed := value.(type) {
	case string:
		return strconv.Quote(typed)
	case nil:
		return "nil"
	}
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(bytes)
}

// number is a numeric value as a float64; anything else is Ruby's TypeError.
func number(value any) float64 {
	result, ok := rec.Number(value)
	if !ok {
		rec.Fail("%s can't be coerced into Integer", Inspect(value))
	}
	return result
}

func sub(left, right any) any {
	a, aInt := left.(int64)
	b, bInt := right.(int64)
	if aInt && bInt {
		return a - b
	}
	return number(left) - number(right)
}

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

// Sum is Array#sum: exact while the values are Integers, then Kahan-Babuska compensated
// summation once a Float appears, as Ruby sums.
func Sum(values []any) any {
	var integer int64
	index := 0
	for ; index < len(values); index++ {
		value, ok := values[index].(int64)
		if !ok {
			break
		}
		integer += value
	}
	if index == len(values) {
		return integer
	}
	total, compensation := float64(integer), 0.0
	for _, value := range values[index:] {
		x := number(value)
		if math.IsNaN(total) {
			continue
		}
		if math.IsNaN(x) || math.IsInf(x, 0) {
			total = x
			continue
		}
		next := total + x
		if math.Abs(total) >= math.Abs(x) {
			compensation += (total - next) + x
		} else {
			compensation += (x - next) + total
		}
		total = next
	}
	return total + compensation
}

func fetchInt(m rec.Map, name string) int64 { return rec.AsInt(rec.Fetch(m, name)) }

// FetchList fetches a list to iterate. lualibs writes an empty Lua table as {}: Ruby iterates
// that empty Hash as it does an empty Array, so it reads as an empty list here.
func FetchList(m rec.Map, name string) []any {
	value := rec.Fetch(m, name)
	if object, ok := value.(rec.Map); ok && len(object) == 0 {
		return []any{}
	}
	return rec.AsList(value)
}

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

// contains is Array#include? (==).
func contains(list []any, value any) bool {
	return slices.ContainsFunc(list, func(item any) bool { return rec.Equal(item, value) })
}

// dig is Hash#dig on one key: nil for a missing key or a nil receiver.
func dig(value any, name string) any {
	if value == nil {
		return nil
	}
	m, ok := value.(rec.Map)
	if !ok {
		rec.Fail("%T does not have #dig method", value)
	}
	return m[name]
}

// fetchValues is values_at after fetch: every key must be present.
func fetchValues(m rec.Map, names ...string) []any {
	result := make([]any, len(names))
	for index, name := range names {
		result[index] = rec.Fetch(m, name)
	}
	return result
}

// SortValues is Array#sort on JSON values (numbers, strings or lists of them).
func SortValues(list []any) []any {
	result := slices.Clone(list)
	slices.SortStableFunc(result, CompareValues)
	return result
}

// containsEql is include? with eql? (Array#uniq's equality): 1 and 1.0 differ.
func containsEql(list []any, value any) bool {
	for _, item := range list {
		if key(item) == key(value) {
			return true
		}
	}
	return false
}

// CompareValues is Ruby's <=>; values it cannot order are Ruby's ArgumentError, which the
// table evidence does not rescue.
func CompareValues(a, b any) int {
	if x, ok := rec.Number(a); ok {
		if y, isNumber := rec.Number(b); isNumber {
			return compareNumbers(x, y)
		}
	}
	if x, ok := a.(string); ok {
		if y, isString := b.(string); isString {
			return strings.Compare(x, y)
		}
	}
	if x, ok := a.([]any); ok {
		if y, isList := b.([]any); isList {
			for index := 0; index < len(x) && index < len(y); index++ {
				if order := CompareValues(x[index], y[index]); order != 0 {
					return order
				}
			}
			return len(x) - len(y)
		}
	}
	if a == nil && b == nil {
		return 0
	}
	panic(fmt.Sprintf("tables: comparison of %s with %s failed", Inspect(a), Inspect(b)))
}

func negate(value any) any {
	if integer, ok := value.(int64); ok {
		return -integer
	}
	return -number(value)
}

func truthy(value any) bool { return value != nil && value != false }
