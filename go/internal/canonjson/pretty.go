package canonjson

import (
	"bytes"
	"sort"
	"strings"
)

// Pretty is Ruby's JSON.pretty_generate of a canonical value (keys sorted): two-space
// indentation, ": " after a key; an empty object or array keeps its line breaks as Ruby's
// generator writes them.
func Pretty(value any) ([]byte, error) {
	var buffer bytes.Buffer
	if err := writePretty(&buffer, value, 0); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writePretty(buffer *bytes.Buffer, value any, depth int) error {
	switch typed := value.(type) {
	case map[string]any:
		return prettyObject(buffer, typed, depth)
	case []any:
		return prettyArray(buffer, typed, depth)
	}
	return write(buffer, value)
}

func prettyObject(buffer *bytes.Buffer, object map[string]any, depth int) error {
	if len(object) == 0 {
		buffer.WriteString("{\n" + strings.Repeat("  ", depth) + "}")
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	inner := strings.Repeat("  ", depth+1)
	buffer.WriteString("{\n")
	for index, key := range keys {
		if index > 0 {
			buffer.WriteString(",\n")
		}
		buffer.WriteString(inner)
		writeString(buffer, key)
		buffer.WriteString(": ")
		if err := writePretty(buffer, object[key], depth+1); err != nil {
			return err
		}
	}
	buffer.WriteString("\n" + strings.Repeat("  ", depth) + "}")
	return nil
}

func prettyArray(buffer *bytes.Buffer, items []any, depth int) error {
	if len(items) == 0 {
		buffer.WriteString("[\n\n" + strings.Repeat("  ", depth) + "]")
		return nil
	}
	inner := strings.Repeat("  ", depth+1)
	buffer.WriteString("[\n")
	for index, item := range items {
		if index > 0 {
			buffer.WriteString(",\n")
		}
		buffer.WriteString(inner)
		if err := writePretty(buffer, item, depth+1); err != nil {
			return err
		}
	}
	buffer.WriteString("\n" + strings.Repeat("  ", depth) + "]")
	return nil
}

// PrettyKeys is JSON.pretty_generate of an object whose keys Ruby inserted in the given
// order (a Hash literal written with Relatex.write_json); nested objects are sorted.
func PrettyKeys(object map[string]any, keys []string) ([]byte, error) {
	if len(keys) == 0 {
		return []byte("{\n}"), nil
	}
	var buffer bytes.Buffer
	buffer.WriteString("{\n")
	for index, key := range keys {
		if index > 0 {
			buffer.WriteString(",\n")
		}
		buffer.WriteString("  ")
		writeString(&buffer, key)
		buffer.WriteString(": ")
		if err := writePretty(&buffer, object[key], 1); err != nil {
			return nil, err
		}
	}
	buffer.WriteString("\n}")
	return buffer.Bytes(), nil
}
