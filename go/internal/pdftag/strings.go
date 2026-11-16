package pdftag

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// stringBytes decodes a literal or hex string token to its bytes.
func stringBytes(item token) []byte {
	if item.kind == tokHex {
		return hexBytes(item.raw)
	}
	return literalBytes(item.raw[1 : len(item.raw)-1])
}

func hexBytes(raw []byte) []byte {
	digits := make([]byte, 0, len(raw))
	for _, c := range raw[1 : len(raw)-1] {
		if !isWhite(c) {
			digits = append(digits, c)
		}
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	decoded := make([]byte, len(digits)/2)
	if _, err := hex.Decode(decoded, digits); err != nil {
		return nil
	}
	return decoded
}

var escapes = map[byte]byte{'n': '\n', 'r': '\r', 't': '\t', 'b': '\b', 'f': '\f', '(': '(', ')': ')', '\\': '\\'}

// literalBytes decodes a literal string's body (ISO 32000-2 7.3.4.2).
func literalBytes(body []byte) []byte {
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '\r':
			out = append(out, '\n')
			if i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
		case c != '\\' || i+1 == len(body):
			out = append(out, c)
		default:
			var value []byte
			value, i = escaped(body, i+1)
			out = append(out, value...)
		}
	}
	return out
}

// escaped decodes the escape at body[i] (after the backslash); it returns the bytes and the
// index of the escape's last byte.
func escaped(body []byte, i int) ([]byte, int) {
	c := body[i]
	if value, ok := escapes[c]; ok {
		return []byte{value}, i
	}
	switch {
	case c == '\r':
		if i+1 < len(body) && body[i+1] == '\n' {
			i++
		}
		return nil, i
	case c == '\n':
		return nil, i
	case c >= '0' && c <= '7':
		end := i
		for end < len(body) && end < i+3 && body[end] >= '0' && body[end] <= '7' {
			end++
		}
		value, _ := strconv.ParseUint(string(body[i:end]), 8, 16)
		return []byte{byte(value)}, end - 1
	}
	return []byte{c}, i
}

// arrayItems reads the elements of an array token.
func arrayItems(item token) ([]token, error) {
	l := &lexer{data: item.raw[1 : len(item.raw)-1]}
	var items []token
	for {
		element, err := l.next()
		if err == errEnd {
			return items, nil
		}
		if err != nil {
			return nil, err
		}
		items = append(items, element)
	}
}

// hexString writes bytes as a hex string token.
func hexString(data []byte) string {
	return "<" + strings.ToUpper(hex.EncodeToString(data)) + ">"
}

// number reads a number token.
func number(item token) (float64, error) {
	if item.kind != tokNumber {
		return 0, fmt.Errorf("not a number: %s", item.raw)
	}
	return strconv.ParseFloat(string(item.raw), 64)
}
