package evidence

import (
	"slices"
	"sort"
	"strconv"

	"github.com/sarielhp/relatex/internal/rec"
)

// truthy is Ruby's truthiness: everything but nil and false.
func truthy(value any) bool {
	return value != nil && value != false
}

// ints converts identities to JSON array values.
func ints(values []int64) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

// subtract is Ruby's Array#-: the elements of left not in right, in left's order.
func subtract(left, right []int64) []int64 {
	result := []int64{}
	for _, value := range left {
		if !slices.Contains(right, value) {
			result = append(result, value)
		}
	}
	return result
}

// uniqueValues is Ruby's Array#uniq on JSON values (eql?: 1 and 1.0 stay distinct).
func uniqueValues(values []any) []any {
	result := []any{}
	for _, value := range values {
		if !slices.ContainsFunc(result, func(seen any) bool { return eql(seen, value) }) {
			result = append(result, value)
		}
	}
	return result
}

// uniqueList reports whether a list has no repeated element (Ruby's list.uniq == list).
func uniqueList(values []any) bool {
	return len(uniqueValues(values)) == len(values)
}

// eql is Ruby's eql?: equal values of the same class.
func eql(left, right any) bool {
	switch left.(type) {
	case int64:
		_, ok := right.(int64)
		return ok && rec.Equal(left, right)
	case rec.Map, []any:
		return rec.Equal(left, right)
	}
	return left == right
}

func intersects(left, right []int64) bool {
	for _, value := range left {
		if slices.Contains(right, value) {
			return true
		}
	}
	return false
}

// asInt is a lookup key for an identity that may be missing (Ruby looks up nil and finds
// nothing; identities are positive, so 0 finds nothing either).
func asInt(value any) int64 {
	number, ok := value.(int64)
	if !ok {
		return 0
	}
	return number
}

func sortedKeys(m rec.Map) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
