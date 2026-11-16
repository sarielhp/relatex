// Package canonjson reads JSON and writes it the way relatex's Ruby side writes canonical
// JSON (Acceptance.canonical followed by JSON.generate in Ruby's json 2.7): object keys
// sorted bytewise, no whitespace, integers as written, floats in Ruby's Float#to_s form,
// and strings escaped only where Ruby escapes them. The Go stages and tools/compare_stage
// compare their outputs with the Ruby reference through this form.
package canonjson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Float is a JSON number written with a fraction or an exponent. Ruby reads such a number
// as a Float and writes it back in Float#to_s form, whatever its spelling.
type Float float64

// Integer is a JSON number without a fraction or an exponent that does not fit an int64,
// kept as written: Ruby reads it as an Integer of any size. Smaller ones decode as int64.
type Integer string

// Decode reads one JSON value. Objects become map[string]any, arrays []any, numbers Float,
// int64 or (beyond int64) Integer, and the rest string, bool or nil.
func Decode(r io.Reader) (any, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("decode json: trailing data after the value")
	}
	return convert(value)
}

func convert(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		return number(string(typed))
	case map[string]any:
		for key, item := range typed {
			converted, err := convert(item)
			if err != nil {
				return nil, err
			}
			typed[key] = converted
		}
		return typed, nil
	case []any:
		for index, item := range typed {
			converted, err := convert(item)
			if err != nil {
				return nil, err
			}
			typed[index] = converted
		}
		return typed, nil
	default:
		return typed, nil
	}
}

func number(text string) (any, error) {
	if !strings.ContainsAny(text, ".eE") {
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			return parsed, nil // -0 becomes 0, as Ruby reads it.
		}
		return Integer(text), nil
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, fmt.Errorf("parse number %q: %w", text, err)
	}
	return Float(parsed), nil
}

// Marshal writes a value in canonical form.
func Marshal(value any) ([]byte, error) {
	var buffer bytes.Buffer
	if err := write(&buffer, value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func write(buffer *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buffer.WriteString("null")
	case bool:
		buffer.WriteString(strconv.FormatBool(typed))
	case string:
		writeString(buffer, typed)
	case Integer:
		buffer.WriteString(string(typed))
	case int:
		buffer.WriteString(strconv.Itoa(typed))
	case int64:
		buffer.WriteString(strconv.FormatInt(typed, 10))
	case Float:
		return writeFloat(buffer, float64(typed))
	case float64:
		return writeFloat(buffer, typed)
	case map[string]any:
		return writeObject(buffer, typed)
	case []any:
		return writeArray(buffer, typed)
	default:
		return fmt.Errorf("canonical json: unsupported value of type %T", value)
	}
	return nil
}

func writeObject(buffer *bytes.Buffer, object map[string]any) error {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	buffer.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			buffer.WriteByte(',')
		}
		writeString(buffer, key)
		buffer.WriteByte(':')
		if err := write(buffer, object[key]); err != nil {
			return err
		}
	}
	buffer.WriteByte('}')
	return nil
}

func writeArray(buffer *bytes.Buffer, items []any) error {
	buffer.WriteByte('[')
	for index, item := range items {
		if index > 0 {
			buffer.WriteByte(',')
		}
		if err := write(buffer, item); err != nil {
			return err
		}
	}
	buffer.WriteByte(']')
	return nil
}

// writeString escapes what Ruby's generator escapes: the quote, the backslash and control
// characters below U+0020. Everything else, including "/", "<", DEL and U+2028, is
// written as is.
func writeString(buffer *bytes.Buffer, text string) {
	buffer.WriteByte('"')
	for index := 0; index < len(text); index++ {
		character := text[index]
		switch character {
		case '"':
			buffer.WriteString(`\"`)
		case '\\':
			buffer.WriteString(`\\`)
		case '\b':
			buffer.WriteString(`\b`)
		case '\f':
			buffer.WriteString(`\f`)
		case '\n':
			buffer.WriteString(`\n`)
		case '\r':
			buffer.WriteString(`\r`)
		case '\t':
			buffer.WriteString(`\t`)
		default:
			writeByte(buffer, character)
		}
	}
	buffer.WriteByte('"')
}

func writeByte(buffer *bytes.Buffer, character byte) {
	if character < 0x20 {
		fmt.Fprintf(buffer, `\u%04x`, character)
		return
	}
	buffer.WriteByte(character)
}

// writeFloat writes Ruby's Float#to_s: the shortest round-trip digits, in fixed notation
// when the decimal point falls above -4 and at most at 16 for a value with a fraction, or 15
// for a whole number; otherwise as d.ddde+XX with an exponent of at least two digits.
func writeFloat(buffer *bytes.Buffer, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("canonical json: %v is not representable", value)
	}
	if value == 0 {
		buffer.WriteString(map[bool]string{true: "-0.0", false: "0.0"}[math.Signbit(value)])
		return nil
	}
	if value < 0 {
		buffer.WriteByte('-')
		value = -value
	}
	var scratch [40]byte
	digits, point := shortestDigits(scratch[:0], value)
	limit := 15
	if point < len(digits) {
		limit = 16
	}
	if point > -4 && point <= limit {
		writeFixed(buffer, digits, point)
		return nil
	}
	buffer.WriteString(scientific(string(digits), point))
	return nil
}

// shortestDigits is shortest on bytes: the digits are appended to scratch.
func shortestDigits(scratch []byte, value float64) ([]byte, int) {
	formatted := strconv.AppendFloat(scratch, value, 'e', -1, 64)
	split := bytes.IndexByte(formatted, 'e')
	power, _ := strconv.Atoi(string(formatted[split+1:]))
	mantissa := formatted[:split]
	if len(mantissa) > 1 {
		// d.ddd: the point goes, the digits close up.
		copy(mantissa[1:], mantissa[2:])
		mantissa = mantissa[:len(mantissa)-1]
	}
	return mantissa, power + 1
}

// writeFixed is fixed on bytes.
func writeFixed(buffer *bytes.Buffer, digits []byte, point int) {
	switch {
	case point <= 0:
		buffer.WriteString("0.")
		for range -point {
			buffer.WriteByte('0')
		}
		buffer.Write(digits)
	case len(digits) <= point:
		buffer.Write(digits)
		for range point - len(digits) {
			buffer.WriteByte('0')
		}
		buffer.WriteString(".0")
	default:
		buffer.Write(digits[:point])
		buffer.WriteByte('.')
		buffer.Write(digits[point:])
	}
}

func scientific(digits string, point int) string {
	fraction := digits[1:]
	if fraction == "" {
		fraction = "0"
	}
	exponent := point - 1
	sign := "+"
	if exponent < 0 {
		sign = "-"
		exponent = -exponent
	}
	return fmt.Sprintf("%s.%se%s%02d", digits[:1], fraction, sign, exponent)
}

// Digest is Acceptance.digest: the SHA-256 (hex) of a value's canonical JSON.
func Digest(value any) (string, error) {
	bytes, err := Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}

// Number is a JSON number as Decode reads it: an int64, an Integer beyond int64, or a Float.
func Number(text string) (any, error) { return number(text) }
