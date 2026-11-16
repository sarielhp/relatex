package pdftag

import (
	"bytes"
	"fmt"
)

// tokenKind classifies a content-stream token.
type tokenKind int

const (
	tokNumber tokenKind = iota
	tokName
	tokString // literal string, raw bytes including the parentheses
	tokHex    // hex string, raw bytes including the angle brackets
	tokArray  // a whole array, raw bytes including the brackets
	tokDict   // a whole dictionary, raw bytes including the angle brackets
	tokOperator
)

// token is one operand or operator, as the bytes it was read from.
type token struct {
	kind tokenKind
	raw  []byte
}

// operation is an operator with its operands; raw is every byte from the first operand to the
// operator's end, so an unchanged operation is written back exactly.
type operation struct {
	operator string
	operands []token
	raw      []byte
}

// lexer reads PDF content-stream syntax (ISO 32000-2 7.2 and 8.9.7 for inline images).
type lexer struct {
	data []byte
	pos  int
}

func isWhite(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func isDelimiter(c byte) bool {
	return bytes.IndexByte([]byte("()<>[]{}/%"), c) >= 0
}

// skipSpace skips white space and comments.
func (l *lexer) skipSpace() {
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		switch {
		case isWhite(c):
			l.pos++
		case c == '%':
			for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
		default:
			return
		}
	}
}

// regular reads a run of regular characters (a number, an operator or a name's body).
func (l *lexer) regular() {
	for l.pos < len(l.data) && !isWhite(l.data[l.pos]) && !isDelimiter(l.data[l.pos]) {
		l.pos++
	}
}

// literal reads a literal string from its opening parenthesis.
func (l *lexer) literal() error {
	depth := 0
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		l.pos++
		switch c {
		case '\\':
			l.pos++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("unterminated string")
}

// until reads to and including the closing byte.
func (l *lexer) until(closing byte) error {
	end := bytes.IndexByte(l.data[l.pos:], closing)
	if end < 0 {
		return fmt.Errorf("unterminated %q", closing)
	}
	l.pos += end + 1
	return nil
}

// composite reads an array or a dictionary, nested ones and strings inside included.
func (l *lexer) composite(open, closing string) error {
	depth := 0
	for l.pos < len(l.data) {
		l.skipSpace()
		if l.pos >= len(l.data) {
			break
		}
		switch {
		case bytes.HasPrefix(l.data[l.pos:], []byte(open)):
			depth++
			l.pos += len(open)
		case bytes.HasPrefix(l.data[l.pos:], []byte(closing)):
			depth--
			l.pos += len(closing)
			if depth == 0 {
				return nil
			}
		default:
			if _, err := l.next(); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("unterminated %s", open)
}

// next reads one token; ok is false at the end of the data.
func (l *lexer) next() (token, error) {
	l.skipSpace()
	start := l.pos
	if start >= len(l.data) {
		return token{}, errEnd
	}
	kind, err := l.scan(l.data[start])
	return token{kind: kind, raw: l.data[start:l.pos]}, err
}

func (l *lexer) scan(c byte) (tokenKind, error) {
	switch {
	case c == '(':
		return tokString, l.literal()
	case c == '[':
		return tokArray, l.composite("[", "]")
	case c == '<' && l.pos+1 < len(l.data) && l.data[l.pos+1] == '<':
		return tokDict, l.composite("<<", ">>")
	case c == '<':
		return tokHex, l.until('>')
	case c == '/':
		l.pos++
		l.regular()
		return tokName, nil
	case c == ']' || c == '>' || c == ')' || c == '{' || c == '}':
		l.pos++
		return tokOperator, fmt.Errorf("unexpected %q", c)
	}
	begin := l.pos
	l.regular()
	if l.pos == begin {
		l.pos++
		return tokOperator, fmt.Errorf("unexpected %q", c)
	}
	if numeric(l.data[begin:l.pos]) {
		return tokNumber, nil
	}
	return tokOperator, nil
}

var errEnd = fmt.Errorf("end of content")

// parseContent splits a content stream into operations.
func parseContent(data []byte) ([]operation, error) {
	l := &lexer{data: data}
	var operations []operation
	var operands []token
	start := -1
	for {
		before := l.pos
		item, err := l.next()
		if err == errEnd {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("content stream at byte %d: %w", before, err)
		}
		if start < 0 {
			start = l.pos - len(item.raw)
		}
		if item.kind != tokOperator || isKeyword(item.raw) {
			operands = append(operands, item)
			continue
		}
		op := operation{operator: string(item.raw), operands: operands, raw: data[start:l.pos]}
		if op.operator == "BI" {
			if err := l.inlineImage(&op, data, start); err != nil {
				return nil, err
			}
		}
		operations = append(operations, op)
		operands, start = nil, -1
	}
	if len(operands) > 0 {
		return nil, fmt.Errorf("content stream ends with operands and no operator")
	}
	return operations, nil
}

func isKeyword(raw []byte) bool {
	text := string(raw)
	return text == "true" || text == "false" || text == "null"
}

// inlineImage extends a BI operation through its ID data and EI.
func (l *lexer) inlineImage(op *operation, data []byte, start int) error {
	marker := bytes.Index(data[l.pos:], []byte("ID"))
	if marker < 0 {
		return fmt.Errorf("inline image without ID")
	}
	search := l.pos + marker + 3
	for {
		end := bytes.Index(data[search:], []byte("EI"))
		if end < 0 {
			return fmt.Errorf("inline image without EI")
		}
		at := search + end
		after := at + 2
		if isWhite(data[at-1]) && (after == len(data) || isWhite(data[after]) || isDelimiter(data[after])) {
			l.pos = after
			op.operator, op.raw = "BI", data[start:after]
			return nil
		}
		search = at + 2
	}
}

// numeric reports whether a regular word is a number.
func numeric(word []byte) bool {
	return len(bytes.Trim(word, "+-.0123456789")) == 0
}
