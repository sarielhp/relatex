package bibclean

import (
	"fmt"
	"strings"
)

var monthMacros = map[string]string{
	"jan": "January",
	"feb": "February",
	"mar": "March",
	"apr": "April",
	"may": "May",
	"jun": "June",
	"jul": "July",
	"aug": "August",
	"sep": "September",
	"oct": "October",
	"nov": "November",
	"dec": "December",
}

// preambleMacro is one @preamble definition. A macro with arguments expands only when they are plain:
// args mandatory braced arguments (\newcommand{\X}[2]{..}, \def\X#1#2{..}), no optional one and no
// delimited parameter text (decision 173).
type preambleMacro struct {
	name    string
	hasArgs bool
	args    int
	body    string
}

// expandable reports whether the macro's arguments, if any, are plain mandatory ones.
func (m preambleMacro) expandable() bool {
	return !m.hasArgs || m.args > 0
}

type macroEnv struct {
	strings  map[string]string
	preamble map[string]preambleMacro
}

func newMacroEnv() *macroEnv {
	return &macroEnv{
		strings:  map[string]string{},
		preamble: map[string]preambleMacro{},
	}
}

func (m *macroEnv) addString(name string, parts []ValuePart) {
	val, _ := m.expandParts(parts, "")
	m.strings[strings.ToLower(name)] = val
}

func (m *macroEnv) expandParts(parts []ValuePart, field string) (string, []Reason) {
	var b strings.Builder
	var reasons []Reason
	for _, part := range parts {
		switch part.Kind {
		case PartLiteral, PartNumber:
			b.WriteString(part.Value)
		case PartMacro:
			key := strings.ToLower(part.Value)
			if val, ok := m.strings[key]; ok {
				b.WriteString(val)
				continue
			}
			if val, ok := monthMacros[key]; ok {
				b.WriteString(val)
				continue
			}
			b.WriteString(part.Value)
			reasons = append(reasons, Reason{
				Kind:   "undefined_string",
				Name:   part.Value,
				Field:  field,
				Detail: fmt.Sprintf("undefined @string macro %s", part.Value),
			})
		}
	}
	return b.String(), reasons
}

func (m *macroEnv) addPreamble(parts []ValuePart) {
	src, _ := m.expandParts(parts, "")
	for _, def := range parsePreambleDefs(src) {
		m.preamble[def.name] = def
	}
}

func parsePreambleDefs(src string) []preambleMacro {
	var defs []preambleMacro
	pos := 0
	for pos < len(src) {
		idx := strings.IndexByte(src[pos:], '\\')
		if idx < 0 {
			break
		}
		pos += idx + 1
		cmd := readAlphaWord(src, &pos)
		switch cmd {
		case "newcommand", "providecommand", "renewcommand":
			if pos < len(src) && src[pos] == '*' {
				pos++
			}
			if def, ok := parseLaTeXMacroDef(src, &pos); ok {
				defs = append(defs, def)
			}
		case "def", "gdef", "edef", "xdef":
			if def, ok := parseTeXDef(src, &pos); ok {
				defs = append(defs, def)
			}
		}
	}
	return defs
}

func parseLaTeXMacroDef(src string, pos *int) (preambleMacro, bool) {
	skipSpaces(src, pos)
	name, ok := readMacroTarget(src, pos)
	if !ok {
		return preambleMacro{}, false
	}
	var brackets []string
	skipSpaces(src, pos)
	for *pos < len(src) && src[*pos] == '[' {
		inner, closed := readBalancedBrackets(src, pos)
		if !closed {
			return preambleMacro{}, false
		}
		brackets = append(brackets, inner)
		skipSpaces(src, pos)
	}
	body, closed := readBalancedBraces(src, pos)
	if !closed {
		return preambleMacro{}, false
	}
	macro := preambleMacro{name: name, hasArgs: len(brackets) > 0, body: body}
	// [n] alone: n mandatory arguments; a second bracket is an optional argument's default.
	if len(brackets) == 1 {
		macro.args = argumentCount(strings.TrimSpace(brackets[0]))
	}
	return macro, true
}

// argumentCount is n for "1".."9", else 0.
func argumentCount(text string) int {
	if len(text) == 1 && text[0] >= '1' && text[0] <= '9' {
		return int(text[0] - '0')
	}
	return 0
}

// defArguments is n for TeX parameter text "#1#2...#n" (undelimited), else 0.
func defArguments(parameters string) int {
	parameters = strings.Join(strings.Fields(parameters), "")
	count := len(parameters) / 2
	if count == 0 || count > 9 || len(parameters) != 2*count {
		return 0
	}
	for index := 0; index < count; index++ {
		if parameters[2*index] != '#' || int(parameters[2*index+1]-'0') != index+1 {
			return 0
		}
	}
	return count
}

func parseTeXDef(src string, pos *int) (preambleMacro, bool) {
	skipSpaces(src, pos)
	if *pos >= len(src) || src[*pos] != '\\' {
		return preambleMacro{}, false
	}
	*pos++
	name := readAlphaWord(src, pos)
	if name == "" {
		return preambleMacro{}, false
	}
	start := *pos
	for *pos < len(src) && src[*pos] != '{' {
		*pos++
	}
	parameters := src[start:*pos]
	body, closed := readBalancedBraces(src, pos)
	if !closed {
		return preambleMacro{}, false
	}
	hasArgs := strings.TrimSpace(parameters) != ""
	return preambleMacro{name: name, hasArgs: hasArgs, args: defArguments(parameters), body: body}, true
}

func readMacroTarget(src string, pos *int) (string, bool) {
	if *pos >= len(src) {
		return "", false
	}
	if src[*pos] == '{' {
		inner, ok := readBalancedBraces(src, pos)
		if !ok {
			return "", false
		}
		trimmed := strings.TrimSpace(inner)
		if !strings.HasPrefix(trimmed, `\`) {
			return "", false
		}
		p := 1
		name := readAlphaWord(trimmed, &p)
		return name, name != "" && strings.TrimSpace(trimmed[p:]) == ""
	}
	if src[*pos] != '\\' {
		return "", false
	}
	*pos++
	name := readAlphaWord(src, pos)
	return name, name != ""
}

func readAlphaWord(src string, pos *int) string {
	start := *pos
	for *pos < len(src) && isASCIIAlpha(src[*pos]) {
		*pos++
	}
	return src[start:*pos]
}

func skipSpaces(src string, pos *int) {
	for *pos < len(src) && isASCIISpace(src[*pos]) {
		*pos++
	}
}

func readBalancedBraces(src string, pos *int) (string, bool) {
	return readBalanced(src, pos, '{', '}')
}

func readBalancedBrackets(src string, pos *int) (string, bool) {
	return readBalanced(src, pos, '[', ']')
}

func readBalanced(src string, pos *int, openCh, closeCh byte) (string, bool) {
	if *pos >= len(src) || src[*pos] != openCh {
		return "", false
	}
	*pos++
	start := *pos
	depth := 1
	for *pos < len(src) && depth > 0 {
		switch src[*pos] {
		case openCh:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				body := src[start:*pos]
				*pos++
				return body, true
			}
		}
		*pos++
	}
	return "", false
}

func isASCIIAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}
