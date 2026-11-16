// Package reading is the Go port of Relatex::ReadingEvidence: one ordered view of the
// retained scalar slots of the record stream, as blocks of spans, with the targets labels
// name and the diagnostics of what the reading could not place.
package reading

import (
	"unicode"

	"github.com/sarielhp/relatex/internal/rec"
)

func truthy(value any) bool { return value != nil && value != false }

// toS is Ruby's to_s on a JSON value.
func toS(value any) string { return rec.ToS(value) }

// content is a block's spans.
func content(block rec.Map) []rec.Map {
	items := rec.FetchList(block, "content")
	result := make([]rec.Map, len(items))
	for index, item := range items {
		result[index] = rec.AsMap(item)
	}
	return result
}

func push(block rec.Map, span rec.Map) {
	block["content"] = append(rec.FetchList(block, "content"), span)
}

// squeeze is gsub(/[[:space:]]+/, replacement).
func squeeze(text, replacement string) string {
	result := make([]rune, 0, len(text))
	space := false
	for _, character := range text {
		if unicode.IsSpace(character) {
			space = true
			continue
		}
		if space {
			result = append(result, []rune(replacement)...)
			space = false
		}
		result = append(result, character)
	}
	if space {
		result = append(result, []rune(replacement)...)
	}
	return string(result)
}

// slice is Hash#slice.
func slice(m rec.Map, keys ...string) rec.Map {
	result := rec.Map{}
	for _, key := range keys {
		if value, ok := m[key]; ok {
			result[key] = value
		}
	}
	return result
}

func positive(value any) bool {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("undefined method 'positive?' for %T", value)
	}
	return number > 0
}

func less(left, right any) bool {
	a, ok := rec.Number(left)
	b, isNumber := rec.Number(right)
	if !ok || !isNumber {
		rec.Fail("comparison of %T with %T failed", left, right)
	}
	return a < b
}

// union is Array#| for one value.
func union(list []any, value any) []any {
	for _, item := range list {
		if rec.Equal(item, value) {
			return list
		}
	}
	return append(list, value)
}

func includes(list []any, value any) bool {
	for _, item := range list {
		if rec.Equal(item, value) {
			return true
		}
	}
	return false
}
