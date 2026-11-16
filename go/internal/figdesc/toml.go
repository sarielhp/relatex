package figdesc

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The sidecar is read by a reader of the small part of TOML 1.0 it needs: comments, table
// headers with bare or quoted dotted keys, and key = value lines whose value is a basic,
// literal or multi-line basic string or a boolean. Anything else is an error naming the line,
// so a hand edit that relatex would misread never passes silently.

// field is one key = value line (or lines, for a multi-line string) of a section.
type field struct {
	value      any
	start, end int // the lines it occupies, [start, end)
}

// section is a table: its header line and the lines up to the next header.
type section struct {
	path       []string // the header's key parts: ["figs", "disk"], ["approval"]
	start, end int
	fields     map[string]field
	order      []string
}

// document is a parsed sidecar: its lines and its sections in file order, and whether the author
// wrote it with CRLF line ends (Windows), which a rewrite keeps.
type document struct {
	lines    []string
	sections []*section
	crlf     bool
}

func parseTOML(text string) (*document, error) {
	doc := &document{lines: strings.Split(text, "\n"), crlf: strings.Contains(text, "\r\n")}
	for index, line := range doc.lines {
		doc.lines[index] = strings.TrimSuffix(line, "\r")
	}
	var current *section
	for index := 0; index < len(doc.lines); index++ {
		line := strings.TrimSpace(doc.lines[index])
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(line, "["):
			next, err := doc.header(line, index)
			if err != nil {
				return nil, err
			}
			if current != nil {
				current.end = index
			}
			current = next
		default:
			end, err := doc.keyValue(current, line, index)
			if err != nil {
				return nil, err
			}
			index = end - 1
		}
	}
	if current != nil {
		current.end = len(doc.lines)
	}
	return doc, nil
}

func (d *document) header(line string, index int) (*section, error) {
	if strings.HasPrefix(line, "[[") {
		return nil, lineError(index, "arrays of tables are not used in this file")
	}
	path, rest, err := parseKey(line[1:])
	if err != nil {
		return nil, lineError(index, err.Error())
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "]") || !blankOrComment(rest[1:]) {
		return nil, lineError(index, "a table header ends with ]")
	}
	for _, existing := range d.sections {
		if strings.Join(existing.path, "\x00") == strings.Join(path, "\x00") {
			return nil, lineError(index, "table ["+displayPath(path)+"] is defined twice")
		}
	}
	next := &section{path: path, start: index, fields: map[string]field{}}
	d.sections = append(d.sections, next)
	return next, nil
}

func (d *document) keyValue(current *section, line string, index int) (int, error) {
	if current == nil {
		return 0, lineError(index, "a setting outside a table; start the entry with a header such as [figs.NAME]")
	}
	path, rest, err := parseKey(line)
	if err != nil {
		return 0, lineError(index, err.Error())
	}
	if len(path) != 1 {
		return 0, lineError(index, "dotted keys are not used inside an entry")
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "=") {
		return 0, lineError(index, "expected key = value")
	}
	value, end, err := parseValue(d.lines, index, strings.TrimSpace(rest[1:]))
	if err != nil {
		return 0, err
	}
	if _, seen := current.fields[path[0]]; seen {
		return 0, lineError(index, "key "+strconv.Quote(path[0])+" is set twice")
	}
	current.fields[path[0]] = field{value: value, start: index, end: end}
	current.order = append(current.order, path[0])
	return end, nil
}

// parseKey reads a dotted key of bare and quoted parts; rest is what follows it.
func parseKey(text string) (parts []string, rest string, err error) {
	rest = strings.TrimLeft(text, " \t")
	for {
		var part string
		switch {
		case strings.HasPrefix(rest, `"`):
			part, rest, err = basicString(rest)
		case strings.HasPrefix(rest, "'"):
			part, rest, err = literalString(rest)
		default:
			length := 0
			for length < len(rest) && bareKeyByte(rest[length]) {
				length++
			}
			if length == 0 {
				return nil, "", fmt.Errorf("expected a key")
			}
			part, rest = rest[:length], rest[length:]
		}
		if err != nil {
			return nil, "", err
		}
		parts = append(parts, part)
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, ".") {
			return parts, rest, nil
		}
		rest = strings.TrimLeft(rest[1:], " \t")
	}
}

func bareKeyByte(character byte) bool {
	return character == '_' || character == '-' || character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}

// parseValue reads the value that starts at text on line index; end is the line after it.
func parseValue(lines []string, index int, text string) (value any, end int, err error) {
	var rest string
	switch {
	case strings.HasPrefix(text, `"""`):
		return multilineString(lines, index, text[3:])
	case strings.HasPrefix(text, `"`):
		value, rest, err = basicString(text)
	case strings.HasPrefix(text, "'"):
		value, rest, err = literalString(text)
	case strings.HasPrefix(text, "true"):
		value, rest = true, text[4:]
	case strings.HasPrefix(text, "false"):
		value, rest = false, text[5:]
	default:
		return nil, 0, lineError(index, "a value is a quoted string, true or false")
	}
	if err != nil {
		return nil, 0, lineError(index, err.Error())
	}
	if !blankOrComment(rest) {
		return nil, 0, lineError(index, "unexpected text after the value")
	}
	return value, index + 1, nil
}

// multilineString reads a """ string from text (after the quotes) over the following lines.
func multilineString(lines []string, index int, text string) (any, int, error) {
	var raw strings.Builder
	first := true
	for line := index; line < len(lines); line++ {
		if !first {
			raw.WriteString("\n")
		}
		chunk := lines[line]
		if first {
			chunk = text
		}
		if closing := closingQuotes(chunk); closing >= 0 {
			raw.WriteString(chunk[:closing])
			if !blankOrComment(chunk[closing+3:]) {
				return nil, 0, lineError(line, "unexpected text after the value")
			}
			value, err := unescape(strings.TrimPrefix(raw.String(), "\n"))
			if err != nil {
				return nil, 0, lineError(index, err.Error())
			}
			return value, line + 1, nil
		}
		raw.WriteString(chunk)
		first = false
	}
	return nil, 0, lineError(index, `a """ string is not closed`)
}

// closingQuotes is the index of the first unescaped """ in chunk, or -1.
func closingQuotes(chunk string) int {
	for position := 0; position+2 < len(chunk); position++ {
		if chunk[position] == '\\' {
			position++
			continue
		}
		if strings.HasPrefix(chunk[position:], `"""`) {
			return position
		}
	}
	return -1
}

// basicString reads a "..." string at the start of text.
func basicString(text string) (string, string, error) {
	for position := 1; position < len(text); position++ {
		switch text[position] {
		case '\\':
			position++
		case '"':
			value, err := unescape(text[1:position])
			return value, text[position+1:], err
		}
	}
	return "", "", fmt.Errorf("a string is not closed")
}

// literalString reads a '...' string (no escapes) at the start of text.
func literalString(text string) (string, string, error) {
	end := strings.IndexByte(text[1:], '\'')
	if end < 0 {
		return "", "", fmt.Errorf("a string is not closed")
	}
	return text[1 : end+1], text[end+2:], nil
}

var simpleEscapes = map[byte]string{'"': `"`, '\\': `\`, 'n': "\n", 't': "\t", 'r': "\r", 'b': "\b", 'f': "\f"}

func unescape(raw string) (string, error) {
	var out strings.Builder
	for position := 0; position < len(raw); position++ {
		if raw[position] != '\\' {
			out.WriteByte(raw[position])
			continue
		}
		if position+1 >= len(raw) {
			return "", fmt.Errorf("a string ends with a backslash")
		}
		position++
		if replacement, ok := simpleEscapes[raw[position]]; ok {
			out.WriteString(replacement)
			continue
		}
		width := map[byte]int{'u': 4, 'U': 8}[raw[position]]
		if width == 0 || position+width >= len(raw) {
			return "", fmt.Errorf("unknown escape \\%c", raw[position])
		}
		code, err := strconv.ParseUint(raw[position+1:position+1+width], 16, 32)
		if err != nil || !utf8.ValidRune(rune(code)) {
			return "", fmt.Errorf("bad unicode escape")
		}
		out.WriteRune(rune(code))
		position += width
	}
	return out.String(), nil
}

func blankOrComment(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

func lineError(index int, message string) error {
	return fmt.Errorf("line %d: %s", index+1, message)
}

func displayPath(path []string) string {
	parts := make([]string, len(path))
	for index, part := range path {
		parts[index] = quoteKey(part)
	}
	return strings.Join(parts, ".")
}

// quoteKey is a key as written: bare when it can be, else a basic string.
func quoteKey(key string) string {
	bare := key != ""
	for index := 0; index < len(key); index++ {
		bare = bare && bareKeyByte(key[index])
	}
	if bare {
		return key
	}
	return quoteString(key)
}

// quoteString is a TOML basic string.
func quoteString(text string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, character := range text {
		switch {
		case character == '"' || character == '\\':
			out.WriteByte('\\')
			out.WriteRune(character)
		case character == '\n':
			out.WriteString(`\n`)
		case character == '\t':
			out.WriteString(`\t`)
		case character < 0x20 || character == 0x7f:
			fmt.Fprintf(&out, `\u%04X`, character)
		default:
			out.WriteRune(character)
		}
	}
	out.WriteByte('"')
	return out.String()
}
