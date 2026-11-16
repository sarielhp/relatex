package frontmatter

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/rec"
)

// Ruby's regular expressions differ from RE2 where these helpers stand in: \s is ASCII
// whitespace with the vertical tab, \b is a Unicode word boundary (\w itself is ASCII), and
// lookarounds are checked by hand. Positions are byte offsets throughout; every delimiter
// the readers look for is ASCII, so they fall on character boundaries as Ruby's do.

// space is Ruby's \s.
const space = `[ \t\n\v\f\r]`

// rx compiles a Ruby pattern: \s and \S outside character classes become Ruby's ASCII
// whitespace classes; \s inside a class ([^}\s]) becomes its characters.
func rx(pattern string) *regexp.Regexp {
	var builder strings.Builder
	inClass := false
	for index := 0; index < len(pattern); index++ {
		if pattern[index] != '\\' || index+1 == len(pattern) {
			switch pattern[index] {
			case '[':
				inClass = true
			case ']':
				inClass = false
			}
			builder.WriteByte(pattern[index])
			continue
		}
		switch next := pattern[index+1]; next {
		case 's':
			if inClass {
				builder.WriteString(space[1 : len(space)-1])
			} else {
				builder.WriteString(space)
			}
		case 'S':
			builder.WriteString(`[^ \t\n\v\f\r]`)
		default:
			builder.WriteByte('\\')
			builder.WriteByte(next)
		}
		index++
	}
	return regexp.MustCompile(builder.String())
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\v' || b == '\f' || b == '\r'
}

func isASCIILetter(b byte) bool { return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') }

// isWord is Onigmo's Unicode word character, which its \b uses.
func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.Is(unicode.M, r) || unicode.Is(unicode.Nd, r) || unicode.Is(unicode.Pc, r)
}

// boundary is \b at position after a word character: the text ends or a non-word follows.
func boundary(text string, position int) bool {
	if position >= len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[position:])
	return !isWord(r)
}

// skipSpace is \s* from position.
func skipSpace(text string, position int) int {
	for position < len(text) && isSpace(text[position]) {
		position++
	}
	return position
}

// commandAt matches \NAME at position for the first name (in order) the check accepts,
// as Ruby tries the alternatives of \\(?:a|b|c) followed by a condition. It returns the
// name and where the match ends.
func commandAt(text string, position int, names []string, check func(text string, end int) (int, bool)) (string, int, bool) {
	if position >= len(text) || text[position] != '\\' {
		return "", 0, false
	}
	for _, name := range names {
		if !strings.HasPrefix(text[position+1:], name) {
			continue
		}
		if end, ok := check(text, position+1+len(name)); ok {
			return name, end, true
		}
	}
	return "", 0, false
}

// findCommand is the first match of commandAt at or after from.
func findCommand(text string, from int, names []string, check func(string, int) (int, bool)) (start, end int, name string, ok bool) {
	for position := from; position < len(text); position++ {
		if text[position] != '\\' {
			continue
		}
		if found, ending, matched := commandAt(text, position, names, check); matched {
			return position, ending, found, true
		}
	}
	return 0, 0, "", false
}

// replaceCommands is gsub over findCommand's matches.
func replaceCommands(text string, names []string, check func(string, int) (int, bool), replace func(name string) string) string {
	var builder strings.Builder
	position := 0
	for {
		start, end, name, ok := findCommand(text, position, names, check)
		if !ok {
			break
		}
		builder.WriteString(text[position:start])
		builder.WriteString(replace(name))
		position = end
	}
	builder.WriteString(text[position:])
	return builder.String()
}

// wordEnd is \b: the command's name ends at a word boundary.
func wordEnd(text string, end int) (int, bool) { return end, boundary(text, end) }

// wordEndSpace is \b\s*.
func wordEndSpace(text string, end int) (int, bool) {
	if !boundary(text, end) {
		return 0, false
	}
	return skipSpace(text, end), true
}

// notLetter is (?![A-Za-z]).
func notLetter(text string, end int) bool { return end >= len(text) || !isASCIILetter(text[end]) }

// beforeBrace is \s*(?=\{): the end before the brace.
func beforeBrace(text string, end int) (int, bool) {
	end = skipSpace(text, end)
	return end, end < len(text) && text[end] == '{'
}

// unescaped reports the positions of character c not preceded by a backslash ((?<!\\)c).
func unescaped(text string, c byte) []int {
	result := []int{}
	for index := 0; index < len(text); index++ {
		if text[index] == c && (index == 0 || text[index-1] != '\\') {
			result = append(result, index)
		}
	}
	return result
}

// squeezeSpace is gsub(/\s+/, replacement).
func squeezeSpace(text, replacement string) string {
	return spaceRun.ReplaceAllLiteralString(text, replacement)
}

var spaceRun = rx(`\s+`)

// strip is Ruby's String#strip.
func strip(text string) string { return rec.Strip(text) }

// awkSplit is String#split with no pattern: fields between runs of ASCII whitespace.
func awkSplit(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return r < 0x80 && isSpace(byte(r)) })
}

// nfc and nfkc are unicode_normalize(:nfc) and (:nfkc).
func nfc(text string) string  { return norm.NFC.String(text) }
func nfkc(text string) string { return norm.NFKC.String(text) }

var (
	lowerCaser = cases.Lower(language.Und)
	upperCaser = cases.Upper(language.Und)
)

// downcase and upcase are Ruby's full case mappings, character by character (Ruby applies
// no final-sigma context).
func downcase(text string) string { return mapRunes(text, lowerCaser) }
func upcase(text string) string   { return mapRunes(text, upperCaser) }

func mapRunes(text string, caser cases.Caser) string {
	var builder strings.Builder
	for _, r := range text {
		builder.WriteString(caser.String(string(r)))
	}
	return builder.String()
}

// runeIndex is String#index(label, cursor) in characters, -1 when absent.
func runeIndex(text, label []rune, cursor int) int {
	for start := cursor; start+len(label) <= len(text); start++ {
		if equalRunes(text[start:start+len(label)], label) {
			return start
		}
	}
	return -1
}

func equalRunes(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
