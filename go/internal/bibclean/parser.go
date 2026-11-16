// Package bibclean cleans BibTeX and biblatex bibliographies for Typst's Pergamon/citegeist.
package bibclean

import (
	"fmt"
	"strings"
	"unicode"
)

// PartKind classifies one segment of a BibTeX field or @string expression.
type PartKind int

const (
	// PartLiteral is a braced or quoted string literal.
	PartLiteral PartKind = iota
	// PartMacro is an unquoted identifier referencing a @string or month macro.
	PartMacro
	// PartNumber is an unquoted integer literal.
	PartNumber
)

// ValuePart is one segment in a `#`-concatenated BibTeX expression.
type ValuePart struct {
	Kind  PartKind
	Value string
}

// Field is one field inside a BibTeX entry.
type Field struct {
	Name  string
	Parts []ValuePart
}

// Entry is one bibliographic record parsed from a .bib file.
type Entry struct {
	Type   string
	Key    string
	Fields []Field
	Source string
	Line   int
}

// stringDef is one @string macro definition.
type stringDef struct {
	name  string
	parts []ValuePart
}

// parsedBib holds the declarations and entries parsed from one .bib input.
type parsedBib struct {
	strings   []stringDef
	preambles [][]ValuePart
	entries   []Entry
}

type bibParser struct {
	name string
	src  string
	pos  int
	line int
}

func parseBib(name, src string) (parsedBib, error) {
	p := &bibParser{name: name, src: src, line: 1}
	var out parsedBib
	for p.pos < len(p.src) {
		p.skipOutside()
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] != '@' {
			p.advance()
			continue
		}
		if err := p.parseDirective(&out); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (p *bibParser) parseDirective(out *parsedBib) error {
	startLine := p.line
	p.advance() // consume '@'
	kind := strings.ToLower(p.readIdent())
	if kind == "" {
		return p.errorf(startLine, "expected entry type after @")
	}
	p.skipWSAndComments()
	if kind == "comment" {
		return p.skipCommentDirective()
	}
	closeDelim, err := p.readOpenDelim(startLine)
	if err != nil {
		return err
	}
	switch kind {
	case "string":
		def, err := p.parseStringBody(closeDelim)
		if err != nil {
			return err
		}
		out.strings = append(out.strings, def)
		return nil
	case "preamble":
		parts, err := p.parseExpr(closeDelim)
		if err != nil {
			return err
		}
		p.skipWSAndComments()
		if err := p.expectCloseDelim(closeDelim); err != nil {
			return err
		}
		out.preambles = append(out.preambles, parts)
		return nil
	default:
		entry, err := p.parseEntryBody(kind, startLine, closeDelim)
		if err != nil {
			return err
		}
		out.entries = append(out.entries, entry)
		return nil
	}
}

func (p *bibParser) skipCommentDirective() error {
	if p.pos >= len(p.src) {
		return nil
	}
	ch := p.src[p.pos]
	if ch == '{' {
		_, err := p.readBraced()
		return err
	}
	if ch == '(' {
		return p.skipParens()
	}
	for p.pos < len(p.src) && p.src[p.pos] != '\n' {
		p.advance()
	}
	return nil
}

func (p *bibParser) skipParens() error {
	startLine := p.line
	p.advance() // consume '('
	depth := 1
	for p.pos < len(p.src) && depth > 0 {
		switch p.src[p.pos] {
		case '(':
			depth++
		case ')':
			depth--
		case '{':
			if _, err := p.readBraced(); err != nil {
				return err
			}
			continue
		case '"':
			if _, err := p.readQuoted(); err != nil {
				return err
			}
			continue
		}
		p.advance()
	}
	if depth != 0 {
		return p.errorf(startLine, "unclosed @comment(...)")
	}
	return nil
}

func (p *bibParser) parseStringBody(closeDelim byte) (stringDef, error) {
	p.skipWSAndComments()
	line := p.line
	name := p.readIdent()
	if name == "" {
		return stringDef{}, p.errorf(line, "expected macro name in @string")
	}
	p.skipWSAndComments()
	if p.pos >= len(p.src) || p.src[p.pos] != '=' {
		return stringDef{}, p.errorf(p.line, "expected '=' after @string name %q", name)
	}
	p.advance()
	parts, err := p.parseExpr(closeDelim)
	if err != nil {
		return stringDef{}, err
	}
	p.skipWSAndComments()
	if err := p.expectCloseDelim(closeDelim); err != nil {
		return stringDef{}, err
	}
	return stringDef{name: strings.ToLower(name), parts: parts}, nil
}

func (p *bibParser) parseEntryBody(kind string, startLine int, closeDelim byte) (Entry, error) {
	p.skipWSAndComments()
	key := p.readKey(closeDelim)
	if key == "" {
		return Entry{}, p.errorf(startLine, "missing citation key in @%s", kind)
	}
	entry := Entry{Type: kind, Key: key, Source: p.name, Line: startLine}
	for {
		p.skipWSAndComments()
		if p.pos >= len(p.src) {
			return entry, p.errorf(startLine, "unexpected end of file in @%s{%s", kind, key)
		}
		if p.src[p.pos] == closeDelim {
			p.advance()
			return entry, nil
		}
		if p.src[p.pos] == ',' {
			p.advance()
			p.skipWSAndComments()
			if p.pos < len(p.src) && p.src[p.pos] == closeDelim {
				p.advance()
				return entry, nil
			}
		}
		field, err := p.parseField(closeDelim)
		if err != nil {
			return entry, err
		}
		entry.Fields = append(entry.Fields, field)
	}
}

func (p *bibParser) parseField(closeDelim byte) (Field, error) {
	line := p.line
	name := strings.ToLower(p.readIdent())
	if name == "" {
		return Field{}, p.errorf(line, "expected field name")
	}
	p.skipWSAndComments()
	if p.pos >= len(p.src) || p.src[p.pos] != '=' {
		return Field{}, p.errorf(p.line, "expected '=' after field %q", name)
	}
	p.advance()
	parts, err := p.parseExpr(closeDelim)
	if err != nil {
		return Field{}, err
	}
	return Field{Name: name, Parts: parts}, nil
}

func (p *bibParser) parseExpr(entryClose byte) ([]ValuePart, error) {
	var parts []ValuePart
	for {
		p.skipWSAndComments()
		part, err := p.parsePart(entryClose)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
		p.skipWSAndComments()
		if p.pos >= len(p.src) || p.src[p.pos] != '#' {
			break
		}
		p.advance()
	}
	return parts, nil
}

func (p *bibParser) parsePart(entryClose byte) (ValuePart, error) {
	if p.pos >= len(p.src) {
		return ValuePart{}, p.errorf(p.line, "unexpected end of file in field value")
	}
	switch p.src[p.pos] {
	case '{':
		val, err := p.readBraced()
		return ValuePart{Kind: PartLiteral, Value: val}, err
	case '"':
		val, err := p.readQuoted()
		return ValuePart{Kind: PartLiteral, Value: val}, err
	default:
		tok := p.readBareToken(entryClose)
		if tok == "" {
			return ValuePart{}, p.errorf(p.line, "expected field value")
		}
		if isDigits(tok) {
			return ValuePart{Kind: PartNumber, Value: tok}, nil
		}
		return ValuePart{Kind: PartMacro, Value: tok}, nil
	}
}

func (p *bibParser) readBraced() (string, error) {
	startLine := p.line
	p.advance() // consume '{'
	start := p.pos
	depth := 1
	for p.pos < len(p.src) && depth > 0 {
		switch p.src[p.pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end := p.pos
				p.advance()
				return p.src[start:end], nil
			}
		}
		p.advance()
	}
	return "", p.errorf(startLine, "unclosed '{'")
}

func (p *bibParser) readQuoted() (string, error) {
	startLine := p.line
	p.advance() // consume '"'
	start := p.pos
	depth := 0
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case '"':
			if depth == 0 {
				end := p.pos
				p.advance()
				return p.src[start:end], nil
			}
		}
		p.advance()
	}
	return "", p.errorf(startLine, "unclosed '\"'")
}

func (p *bibParser) readOpenDelim(startLine int) (byte, error) {
	if p.pos >= len(p.src) {
		return 0, p.errorf(startLine, "expected '{' or '('")
	}
	switch p.src[p.pos] {
	case '{':
		p.advance()
		return '}', nil
	case '(':
		p.advance()
		return ')', nil
	default:
		return 0, p.errorf(p.line, "expected '{' or '(', got %q", p.src[p.pos])
	}
}

func (p *bibParser) expectCloseDelim(closeDelim byte) error {
	if p.pos >= len(p.src) || p.src[p.pos] != closeDelim {
		return p.errorf(p.line, "expected closing %q", closeDelim)
	}
	p.advance()
	return nil
}

func (p *bibParser) skipOutside() {
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '%' {
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.advance()
			}
			continue
		}
		if ch == '@' {
			return
		}
		p.advance()
	}
}

func (p *bibParser) skipWSAndComments() {
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '%' {
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.advance()
			}
			continue
		}
		if !isASCIISpace(ch) {
			return
		}
		p.advance()
	}
}

func (p *bibParser) readIdent() string {
	start := p.pos
	for p.pos < len(p.src) && isIdentChar(p.src[p.pos]) {
		p.advance()
	}
	return p.src[start:p.pos]
}

func (p *bibParser) readKey(closeDelim byte) string {
	start := p.pos
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == ',' || ch == closeDelim || isASCIISpace(ch) {
			break
		}
		p.advance()
	}
	return p.src[start:p.pos]
}

func (p *bibParser) readBareToken(closeDelim byte) string {
	start := p.pos
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == ',' || ch == '#' || ch == '=' || ch == closeDelim || ch == '{' || ch == '}' || ch == '"' || ch == '%' || isASCIISpace(ch) {
			break
		}
		p.advance()
	}
	return p.src[start:p.pos]
}

func (p *bibParser) advance() {
	if p.pos < len(p.src) {
		if p.src[p.pos] == '\n' {
			p.line++
		}
		p.pos++
	}
}

func (p *bibParser) errorf(line int, format string, args ...any) error {
	prefix := fmt.Sprintf("%s:%d: ", p.name, line)
	if p.name == "" {
		prefix = fmt.Sprintf("line %d: ", line)
	}
	return fmt.Errorf("%s%s", prefix, fmt.Sprintf(format, args...))
}

func isASCIISpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\v' || ch == '\f'
}

func isIdentChar(ch byte) bool {
	if ch <= 0x20 || ch >= 0x7f {
		return false
	}
	return !strings.ContainsRune("=,#{}()\"'%@\\", rune(ch))
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
