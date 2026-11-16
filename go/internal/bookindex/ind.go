package bookindex

import (
	"regexp"
	"strings"
)

// Entry is one \item, \subitem or \subsubitem of a makeindex .ind file: its level, its term as
// written (the key's actual text) and its locators, in order.
type Entry struct {
	Level    int
	Term     string
	Locators []Locator
}

// Locator is one page or range the entry prints ("12", "12–15"), or a cross-reference
// ("see", "see also") whose page makeindex keeps but never prints.
type Locator struct {
	Kind   string // "page", "see" or "seealso"
	Text   string // what the index prints for a page: "12", "12–15" (makeindex's "--" is an en dash)
	Page   string // the page it starts on
	Target string // a cross-reference's target, as written
}

var (
	entryCommand = regexp.MustCompile(`\\(item|subitem|subsubitem|indexspace)\b|\\end\{theindex\}|\\begin\{theindex\}`)
	pageNumber   = regexp.MustCompile(`\A(?:[0-9]+|[ivxlcdm]+|[IVXLCDM]+)\z`)
	spaces       = regexp.MustCompile(`\s+`)
	levels       = map[string]int{"item": 1, "subitem": 2, "subsubitem": 3}
)

// Normalize collapses white space as makeindex's line breaking introduces it (", \n\t\t").
func Normalize(text string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(text, " "))
}

// ParseInd reads the entries of a makeindex .ind file in order; ok is false when an entry's
// text has no term.
func ParseInd(text string) (entries []Entry, ok bool) {
	matches := entryCommand.FindAllStringSubmatchIndex(text, -1)
	for index, match := range matches {
		if match[2] < 0 {
			continue
		}
		level, isEntry := levels[text[match[2]:match[3]]]
		if !isEntry {
			continue
		}
		end := len(text)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		term, locators := splitLocators(Normalize(text[match[1]:end]))
		if term == "" {
			return entries, false
		}
		entries = append(entries, Entry{Level: level, Term: term, Locators: locators})
	}
	return entries, true
}

// splitLocators parts an entry's text at its top-level ", " (makeindex's delim_0 and delim_n)
// and reads locators from the end; what precedes the first part that is no locator is the term.
func splitLocators(text string) (string, []Locator) {
	parts := topLevelParts(text)
	first := len(parts)
	var reversed [][]Locator
	for first > 1 {
		locators, ok := parseLocator(parts[first-1])
		if !ok {
			break
		}
		reversed = append(reversed, locators)
		first--
	}
	locators := []Locator{}
	for index := len(reversed) - 1; index >= 0; index-- {
		locators = append(locators, reversed[index]...)
	}
	return strings.Join(parts[:first], ", "), locators
}

// topLevelParts splits text at ", " outside braces.
func topLevelParts(text string) []string {
	parts := []string{}
	depth, start := 0, 0
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '{':
			depth++
		case '}':
			depth--
		case '\\':
			index++
		case ',':
			if depth == 0 && index+1 < len(text) && text[index+1] == ' ' {
				parts = append(parts, text[start:index])
				start = index + 2
				index++
			}
		}
	}
	return append(parts, text[start:])
}

// parseLocator reads one locator: a bare page or range, an encapsulated one (\hyperpage{12, 13},
// \textbf{12}, hyperref's \hyperindexformat{\textbf}{12}) or a cross-reference (\see{target}{12}).
func parseLocator(token string) ([]Locator, bool) {
	token = strings.TrimSpace(token)
	if pages, ok := pageList(token); ok {
		return pages, true
	}
	name, arguments, ok := command(token)
	if !ok {
		return nil, false
	}
	if (name == "hyperindexformat" || name == "hyperxindexformat") && len(arguments) == 2 {
		if inner, innerArguments, isCommand := command(arguments[0]); isCommand && len(innerArguments) == 1 {
			if see := crossReference(inner, innerArguments[0]); see != nil {
				return []Locator{*see}, true
			}
		}
		return pageList(arguments[1])
	}
	if len(arguments) == 2 {
		if see := crossReference(name, arguments[0]); see != nil {
			return []Locator{*see}, true
		}
		return nil, false
	}
	if len(arguments) == 1 {
		return pageList(arguments[0])
	}
	return nil, false
}

func crossReference(name, target string) *Locator {
	if name == "see" || name == "seealso" {
		return &Locator{Kind: name, Target: target}
	}
	return nil
}

// pageList reads "12", "12--15" or hyperref's merged "12, 13--15".
func pageList(text string) ([]Locator, bool) {
	result := []Locator{}
	for _, part := range strings.Split(strings.TrimSpace(text), ", ") {
		bounds := strings.Split(strings.TrimSpace(part), "--")
		if len(bounds) > 2 {
			return nil, false
		}
		for _, bound := range bounds {
			if !pageNumber.MatchString(bound) {
				return nil, false
			}
		}
		result = append(result, Locator{Kind: "page", Text: strings.Join(bounds, "–"), Page: bounds[0]})
	}
	return result, true
}

// command reads \name{a}{b}... spanning the whole token.
func command(token string) (string, []string, bool) {
	if !strings.HasPrefix(token, `\`) {
		return "", nil, false
	}
	index := 1
	for index < len(token) && isLetter(token[index]) {
		index++
	}
	name := token[1:index]
	if name == "" {
		return "", nil, false
	}
	arguments := []string{}
	for index < len(token) {
		for index < len(token) && token[index] == ' ' {
			index++
		}
		if index >= len(token) || token[index] != '{' {
			return "", nil, false
		}
		end := closing(token, index)
		if end < 0 {
			return "", nil, false
		}
		arguments = append(arguments, token[index+1:end])
		index = end + 1
	}
	return name, arguments, len(arguments) > 0
}

// closing is the index of the brace closing the one at open, -1 without one.
func closing(text string, open int) int {
	depth := 0
	for index := open; index < len(text); index++ {
		switch text[index] {
		case '\\':
			index++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '@' }

// KeyPath is the levels of a makeindex key as the .ind prints them: the key split at its
// unquoted "!", each level's actual text (after an unquoted "@"), the encapsulation (after an
// unquoted "|") left out, and makeindex's quote character removed. encap is the encapsulation.
func KeyPath(key string) (path []string, encap string) {
	var level, actual strings.Builder
	hasActual := false
	finish := func() {
		text := level.String()
		if hasActual {
			text = actual.String()
		}
		path = append(path, Normalize(text))
		level.Reset()
		actual.Reset()
		hasActual = false
	}
	for index := 0; index < len(key); index++ {
		c := key[index]
		switch {
		case c == '\\' && index+1 < len(key) && key[index+1] == '"':
			write(&level, &actual, hasActual, `\"`)
			index++
		case c == '"' && index+1 < len(key):
			write(&level, &actual, hasActual, key[index+1:index+2])
			index++
		case c == '!':
			finish()
		case c == '@' && !hasActual:
			hasActual = true
		case c == '|':
			finish()
			return path, key[index+1:]
		default:
			write(&level, &actual, hasActual, key[index:index+1])
		}
	}
	finish()
	return path, ""
}

func write(level, actual *strings.Builder, hasActual bool, text string) {
	if hasActual {
		actual.WriteString(text)
		return
	}
	level.WriteString(text)
}
