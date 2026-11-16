// Package schema is the Go port of Relatex::Schema: the deliberately small JSON Schema
// vocabulary of relatex's contracts (schemas/*.json). An unsupported keyword fails; this is
// not a general JSON Schema engine. Failures name the JSON path of the offending value, with
// the Ruby reference's messages.
package schema

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

var keywords = map[string]bool{
	"$schema": true, "title": true, "description": true, "type": true, "const": true, "enum": true,
	"minimum": true, "maximum": true, "required": true, "properties": true, "additionalProperties": true,
	"items": true, "oneOf": true, "pattern": true,
}

// The only admitted patterns: a full SHA-256 and an sRGB color.
var patterns = map[string]*regexp.Regexp{
	"^[0-9a-f]{64}$": regexp.MustCompile(`\A[0-9a-f]{64}\z`),
	"^#[0-9a-f]{6}$": regexp.MustCompile(`\A#[0-9a-f]{6}\z`),
	// A BCP 47 language tag (decision 080).
	"^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$": regexp.MustCompile(`\A[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*\z`),
}

// Validate checks a value against a schema; it fails (rec.Fail) like Schema.validate!.
func Validate(value any, schema rec.Map, path string) {
	for key := range schema {
		if !keywords[key] {
			rec.Fail("Unsupported schema vocabulary at %s", path)
		}
	}
	if kind, ok := schema["type"]; ok && kind != nil && !hasType(value, rec.AsString(kind)) {
		rec.Fail("Schema type mismatch at %s", path)
	}
	if constant, ok := schema["const"]; ok && !rec.Equal(value, constant) {
		rec.Fail("Schema constant mismatch at %s", path)
	}
	if enum, ok := schema["enum"]; ok && enum != nil && !included(rec.AsList(enum), value) {
		rec.Fail("Schema enum mismatch at %s", path)
	}
	validatePattern(value, schema, path)
	validateRange(value, schema, path)
	if object, ok := value.(rec.Map); ok {
		validateObject(object, schema, path)
	}
	if items, ok := schema["items"]; ok {
		if list, isList := value.([]any); isList {
			for index, item := range list {
				Validate(item, rec.AsMap(items), fmt.Sprintf("%s[%d]", path, index))
			}
		}
	}
	alternatives, ok := schema["oneOf"]
	if !ok || alternatives == nil {
		return
	}
	matches := 0
	for _, alternative := range candidates(value, rec.AsList(alternatives)) {
		if Valid(value, rec.AsMap(alternative), path) {
			matches++
		}
	}
	if matches != 1 {
		rec.Fail("Schema alternative mismatch at %s (%d alternatives match)", path, matches)
	}
}

// Valid reports whether a value validates.
func Valid(value any, schema rec.Map, path string) (valid bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(rec.Failure); !ok {
				panic(recovered)
			}
			valid = false
		}
	}()
	Validate(value, schema, path)
	return true
}

func hasType(value any, kind string) bool {
	switch kind {
	case "object":
		_, ok := value.(rec.Map)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		return rec.IsInt(value)
	case "boolean":
		_, ok := value.(bool)
		return ok
	}
	rec.Fail("key not found: %q", kind) // Ruby's TYPES.fetch
	return false
}

func included(list []any, value any) bool {
	for _, item := range list {
		if rec.Equal(item, value) {
			return true
		}
	}
	return false
}

func validatePattern(value any, schema rec.Map, path string) {
	source, ok := schema["pattern"]
	if !ok {
		return
	}
	pattern, known := patterns[rec.AsString(source)]
	if !known {
		rec.Fail("Unsupported schema pattern at %s", path)
	}
	text, isString := value.(string)
	if !isString || !pattern.MatchString(text) {
		rec.Fail("Schema pattern mismatch at %s", path)
	}
}

func validateRange(value any, schema rec.Map, path string) {
	if minimum, ok := schema["minimum"]; ok && compare(value, minimum, path) < 0 {
		rec.Fail("Schema lower bound at %s", path)
	}
	if maximum, ok := schema["maximum"]; ok && compare(value, maximum, path) > 0 {
		rec.Fail("Schema upper bound at %s", path)
	}
}

// compare orders two numbers; Ruby raises comparing a number with anything else.
func compare(left, right any, path string) int {
	a, okLeft := rec.Number(left)
	b, okRight := rec.Number(right)
	if _, big := left.(canonjson.Integer); big || !okLeft || !okRight {
		rec.Fail("comparison of %T with %T failed at %s", left, right, path)
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func validateObject(value rec.Map, schema rec.Map, path string) {
	required := []any{}
	if list, ok := schema["required"]; ok {
		required = rec.AsList(list)
	}
	properties := rec.Map{}
	if declared, ok := schema["properties"]; ok {
		properties = rec.AsMap(declared)
	}
	missing := []string{}
	for _, key := range required {
		if _, ok := value[rec.AsString(key)]; !ok {
			missing = append(missing, rec.AsString(key))
		}
	}
	if len(missing) > 0 {
		rec.Fail("Schema missing field at %s: %s", path, joined(missing))
	}
	if additional, ok := schema["additionalProperties"]; ok && additional == false {
		unknown := []string{}
		for key := range value {
			if _, declared := properties[key]; !declared {
				unknown = append(unknown, key)
			}
		}
		if len(unknown) > 0 {
			rec.Fail("Schema unknown field at %s: %s", path, joined(unknown))
		}
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if item, ok := value[key]; ok {
			Validate(item, rec.AsMap(properties[key]), path+"."+key)
		}
	}
}

func joined(names []string) string {
	return strings.Join(names, ", ")
}

// candidates is Schema.candidates: when every alternative requires one property to equal a
// constant (an event's kind), only the alternatives whose constant the value has can match.
func candidates(value any, alternatives []any) []any {
	key := discriminator(alternatives)
	object, ok := value.(rec.Map)
	if key == "" || !ok {
		return alternatives
	}
	selected := []any{}
	for _, alternative := range alternatives {
		properties, _ := rec.AsMap(alternative)["properties"].(rec.Map)
		contract, _ := properties[key].(rec.Map)
		if rec.Equal(contract["const"], object[key]) {
			selected = append(selected, alternative)
		}
	}
	return selected
}

func discriminator(alternatives []any) string {
	var shared map[string]bool
	for _, alternative := range alternatives {
		object := rec.AsMap(alternative)
		properties, _ := object["properties"].(rec.Map)
		required, _ := object["required"].([]any)
		keys := map[string]bool{}
		for _, name := range required {
			contract, ok := properties[rec.AsString(name)].(rec.Map)
			if _, constant := contract["const"]; ok && constant {
				keys[rec.AsString(name)] = true
			}
		}
		shared = intersect(shared, keys)
	}
	names := make([]string, 0, len(shared))
	for name := range shared {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func intersect(shared, keys map[string]bool) map[string]bool {
	if shared == nil {
		return keys
	}
	result := map[string]bool{}
	for key := range shared {
		if keys[key] {
			result[key] = true
		}
	}
	return result
}
