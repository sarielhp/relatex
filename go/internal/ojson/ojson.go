// Package ojson is JSON whose objects keep their key order: Ruby's hashes keep the order their
// keys were inserted (or read) in, and JSON.pretty_generate writes them in that order. The
// reports relatex prints and the files Ruby writes with Relatex.write_json are written in it;
// an Object holds such a hash, and Decode reads a file into Objects in its own order.
package ojson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Object is a JSON object in its key order.
type Object []Pair

// Pair is one key of an Object and its value: {key, value}.
type Pair [2]any

// Key is the pair's key.
func (p Pair) Key() string { return p[0].(string) }

// Value is the pair's value.
func (p Pair) Value() any { return p[1] }

// Get is the value of a key, nil without it.
func (o Object) Get(key string) any {
	for _, pair := range o {
		if pair.Key() == key {
			return pair.Value()
		}
	}
	return nil
}

// Has reports whether the object has a key.
func (o Object) Has(key string) bool {
	for _, pair := range o {
		if pair.Key() == key {
			return true
		}
	}
	return false
}

// Set replaces a key's value, or appends the key (Ruby's hash[key] = value).
func (o Object) Set(key string, value any) Object {
	for index, pair := range o {
		if pair.Key() == key {
			o[index][1] = value
			return o
		}
	}
	return append(o, Pair{key, value})
}

// Plain is the object as a plain JSON map, nested objects too (for callers that compare it).
func (o Object) Plain() rec.Map {
	result := rec.Map{}
	for _, pair := range o {
		result[pair.Key()] = Plain(pair.Value())
	}
	return result
}

// Plain is a value with its Objects as plain maps.
func Plain(value any) any {
	switch typed := value.(type) {
	case Object:
		return typed.Plain()
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = Plain(item)
		}
		return result
	}
	return value
}

// Decode reads one JSON value with its objects as Objects in the file's order.
func Decode(r io.Reader) (any, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode json: trailing data after the value")
	}
	return value, nil
}

// ReadFile decodes a JSON file in its order.
func ReadFile(path string) (any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return Decode(file)
}

func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	switch typed := token.(type) {
	case json.Delim:
		if typed == '{' {
			return decodeObject(decoder)
		}
		return decodeArray(decoder)
	case json.Number:
		return canonjson.Number(string(typed))
	}
	return token, nil
}

func decodeObject(decoder *json.Decoder) (Object, error) {
	object := Object{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode json: %w", err)
		}
		value, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		object = object.Set(token.(string), value)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	return object, nil
}

func decodeArray(decoder *json.Decoder) ([]any, error) {
	list := []any{}
	for decoder.More() {
		value, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		list = append(list, value)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	return list, nil
}

// Pretty is JSON.pretty_generate: Objects in their order, plain maps with sorted keys (a
// canonical document read back), the empty containers as Ruby's generator writes them.
func Pretty(value any) []byte {
	var buffer bytes.Buffer
	write(&buffer, value, 0)
	return buffer.Bytes()
}

// WriteFile is Relatex.write_json: Pretty and a newline, through a temporary file.
func WriteFile(path string, value any) error {
	return canonjson.WriteAtomic(path, append(Pretty(value), '\n'))
}

func write(buffer *bytes.Buffer, value any, depth int) {
	switch typed := value.(type) {
	case Object:
		writeObject(buffer, typed, depth)
	case rec.Map:
		writeObject(buffer, sortedObject(typed), depth)
	case []any:
		writeArray(buffer, typed, depth)
	default:
		bytes, err := canonjson.Marshal(value)
		if err != nil {
			rec.Fail("%v", err)
		}
		buffer.Write(bytes)
	}
}

func writeObject(buffer *bytes.Buffer, object Object, depth int) {
	indent := strings.Repeat("  ", depth)
	if len(object) == 0 {
		buffer.WriteString("{\n" + indent + "}")
		return
	}
	buffer.WriteString("{\n")
	for index, pair := range object {
		if index > 0 {
			buffer.WriteString(",\n")
		}
		buffer.WriteString(indent + "  ")
		write(buffer, pair.Key(), depth+1)
		buffer.WriteString(": ")
		write(buffer, pair.Value(), depth+1)
	}
	buffer.WriteString("\n" + indent + "}")
}

func writeArray(buffer *bytes.Buffer, list []any, depth int) {
	indent := strings.Repeat("  ", depth)
	if len(list) == 0 {
		buffer.WriteString("[\n\n" + indent + "]")
		return
	}
	buffer.WriteString("[\n")
	for index, item := range list {
		if index > 0 {
			buffer.WriteString(",\n")
		}
		buffer.WriteString(indent + "  ")
		write(buffer, item, depth+1)
	}
	buffer.WriteString("\n" + indent + "]")
}

// Marshal is JSON.generate: compact, Objects in their order, plain maps sorted.
func Marshal(value any) []byte {
	var buffer bytes.Buffer
	compact(&buffer, value)
	return buffer.Bytes()
}

func compact(buffer *bytes.Buffer, value any) {
	switch typed := value.(type) {
	case Object:
		buffer.WriteByte('{')
		for index, pair := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			compact(buffer, pair.Key())
			buffer.WriteByte(':')
			compact(buffer, pair.Value())
		}
		buffer.WriteByte('}')
	case []any:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			compact(buffer, item)
		}
		buffer.WriteByte(']')
	case rec.Map:
		compact(buffer, sortedObject(typed))
	default:
		bytes, err := canonjson.Marshal(value)
		if err != nil {
			rec.Fail("%v", err)
		}
		buffer.Write(bytes)
	}
}

func sortedObject(m rec.Map) Object {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	object := make(Object, len(keys))
	for index, key := range keys {
		object[index] = Pair{key, m[key]}
	}
	return object
}
