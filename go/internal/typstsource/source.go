// Package typstsource is the Go port of Relatex::TypstSourceExport (decision 063): a Typst
// project the author recognizes as their paper, with their labels, @refs, citations,
// numbering and wording. Numbers come from Typst's own counters, set so every printed
// number equals LaTeX's (numbering.go).
package typstsource

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// markup is TypstSource::MARKUP: characters with a meaning of their own in Typst markup.
const markup = "\\#*_`$<>@[]~\"'"

// typstString is TypstSource.string: a Typst string literal.
func typstString(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(value) + `"`
}

// escape is TypstSource.escape(text, brackets: true).
func escape(text string) string { return escapeBrackets(text, true) }

// escapeBrackets is TypstSource.escape: every character with a meaning of its own is escaped,
// nothing else. Brackets the caller knows to be balanced may stay: balanced brackets are text
// to Typst. A comment opener, a dash ligature, a minus sign and an ellipsis read as text.
func escapeBrackets(text string, brackets bool) string {
	var builder strings.Builder
	for _, char := range text {
		if strings.ContainsRune(markup, char) && (brackets || (char != '[' && char != ']')) {
			builder.WriteByte('\\')
		}
		builder.WriteRune(char)
	}
	escaped := []rune(builder.String())
	escaped = insertBefore(escaped, func(runes []rune, at int) bool {
		return runes[at] == '/' && at+1 < len(runes) && (runes[at+1] == '/' || runes[at+1] == '*')
	})
	escaped = insertBefore(escaped, func(runes []rune, at int) bool {
		if runes[at] != '-' || at+1 >= len(runes) {
			return false
		}
		if runes[at+1] == '-' {
			return true
		}
		return isDigit(runes[at+1]) && (at == 0 || !letterOrNumber(runes[at-1]))
	})
	escaped = insertBefore(escaped, func(runes []rune, at int) bool {
		return runes[at] == '.' && at+2 < len(runes) && runes[at+1] == '.' && runes[at+2] == '.'
	})
	return string(escaped)
}

// insertBefore is gsub(/X(?=...)/, '\\X') for a one-character match: a backslash before
// every character the test accepts, the tests seeing the text before any insertion.
func insertBefore(runes []rune, test func([]rune, int) bool) []rune {
	result := make([]rune, 0, len(runes))
	for at, char := range runes {
		if test(runes, at) {
			result = append(result, '\\')
		}
		result = append(result, char)
	}
	return result
}

// protectStart is TypstSource.protect_start: a paragraph's first characters must not start
// a heading, list, enum or term.
func protectStart(text string) string {
	runes := []rune(text)
	lead := leadingSpace(runes)
	if lead < len(runes) && strings.ContainsRune("=+-/", runes[lead]) && lead+1 < len(runes) && isSpace(runes[lead+1]) {
		runes = append(append(append([]rune{}, runes[:lead]...), '\\'), runes[lead:]...)
	}
	lead = leadingSpace(runes)
	digits := lead
	for digits < len(runes) && isDigit(runes[digits]) {
		digits++
	}
	if digits > lead && digits+1 < len(runes) && runes[digits] == '.' && isSpace(runes[digits+1]) {
		runes = append(append(append([]rune{}, runes[:digits]...), '\\'), runes[digits:]...)
	}
	return string(runes)
}

// labelName is TypstSource.label_name: a LaTeX label or citation key as a Typst label name:
// letters, digits, _ - . :, ending in a letter, digit or _ so @name cannot swallow the
// punctuation after it.
func labelName(name string) string {
	runes := []rune(name)
	for index, char := range runes {
		if !letterOrNumber(char) && !strings.ContainsRune("_-.:", char) {
			runes[index] = '-'
		}
	}
	end := len(runes)
	for end > 0 && strings.ContainsRune(".:-", runes[end-1]) {
		end--
	}
	cleaned := string(runes[:end])
	if end == 0 || !(unicode.IsLetter(runes[0]) || runes[0] == '_') {
		cleaned = "l-" + cleaned
	}
	return cleaned
}

var (
	counterReset   = regexp.MustCompile(`\\global[ \t\n\v\f\r]*\\c@[A-Za-z0-9_]+[ \t\n\v\f\r]*[0-9]+[ \t\n\v\f\r]*\\relax[ \t\n\v\f\r]*`)
	appendixPrefix = regexp.MustCompile(`\A[ \t\n\v\f\r]*Appendix[ \t\n\v\f\r]+`)
	texSpaces      = regexp.MustCompile(`\\[,;: ]|~|\x{00A0}`)
	parenthesized  = regexp.MustCompile(`\A\((.*)\)\z`)
)

// comparableNumber is TypstSource.comparable_number: a printed number as references compare
// it: TeX spaces as spaces, no trailing dot, no parentheses around it. A counter reset a
// class leaves in \@currentlabel and an "Appendix" prefix are not the number.
func comparableNumber(value any) string {
	number := counterReset.ReplaceAllString(toS(value), "")
	number = appendixPrefix.ReplaceAllString(number, "")
	number = rec.Strip(squeeze(texSpaces.ReplaceAllString(number, " "), ' '))
	number = strings.TrimSuffix(number, ".")
	return parenthesized.ReplaceAllString(number, "$1")
}

// squeeze is String#squeeze(char).
func squeeze(text string, char rune) string {
	var builder strings.Builder
	previous := false
	for _, current := range text {
		if current == char && previous {
			continue
		}
		previous = current == char
		builder.WriteRune(current)
	}
	return builder.String()
}

// isSpace is Ruby's \s: ASCII whitespace with the vertical tab.
func isSpace(char rune) bool {
	return char == ' ' || char == '\t' || char == '\n' || char == '\v' || char == '\f' || char == '\r'
}

func isDigit(char rune) bool { return char >= '0' && char <= '9' }

func letterOrNumber(char rune) bool { return unicode.IsLetter(char) || unicode.IsNumber(char) }

func leadingSpace(runes []rune) int {
	index := 0
	for index < len(runes) && isSpace(runes[index]) {
		index++
	}
	return index
}

// lead is text[/\A\s*/].
func lead(text string) string {
	runes := []rune(text)
	return string(runes[:leadingSpace(runes)])
}

// trail is text[/\s*\z/].
func trail(text string) string {
	runes := []rune(text)
	end := len(runes)
	for end > 0 && isSpace(runes[end-1]) {
		end--
	}
	return string(runes[end:])
}

// size is String#size: characters.
func size(text string) int { return len([]rune(text)) }

// from is text[count..] in characters ("" past the end).
func from(text string, count int) string {
	runes := []rune(text)
	if count >= len(runes) {
		return ""
	}
	return string(runes[count:])
}

// prefix is text[0, count] in characters.
func prefix(text string, count int) string {
	runes := []rune(text)
	if count > len(runes) {
		count = len(runes)
	}
	if count < 0 {
		count = 0
	}
	return string(runes[:count])
}

// runeOffset converts a byte offset of text to characters.
func runeOffset(text string, offset int) int { return len([]rune(text[:offset])) }

// fields is String#split with no argument: runs of ASCII whitespace separate words.
func fields(text string) []string { return strings.FieldsFunc(text, isSpace) }

var lower = cases.Lower(language.Und)

// downcase is String#downcase: full Unicode case mapping.
func downcase(text string) string { return lower.String(text) }

func truthy(value any) bool { return value != nil && value != false }

// toS is Ruby's to_s on a JSON value.
func toS(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	}
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return string(bytes)
}

// dig is Hash#dig over nested objects.
func dig(value any, keys ...string) any {
	for _, key := range keys {
		m, ok := value.(rec.Map)
		if !ok {
			return nil
		}
		value = m[key]
	}
	return value
}

func list(value any) []any {
	items, _ := value.([]any)
	return items
}

func maps(value any) []rec.Map {
	items := list(value)
	result := make([]rec.Map, 0, len(items))
	for _, item := range items {
		result = append(result, rec.AsMap(item))
	}
	return result
}
