package sourcemap

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The legacy (source map 1.2) literal evidence: SourceTextIndex. Bounded literal evidence,
// never a macro expander or a claim of native columns; unknown command arguments and
// definitions remain unavailable for inference.

var (
	controlOrComment = regexp.MustCompile(`\\(?:[\p{L}\p{M}@]+|.)|%[^\n]*`)
	boundary         = regexp.MustCompile(`\A[ \t\n\v\f\r]*\z|\\(?:begin|end|item|input|include|(?:sub)*section\*?|paragraph|clearpage|newpage|maketitle|bibliography)\b`)
	definition       = regexp.MustCompile(`\\(?:newcommand|renewcommand|providecommand|def|gdef|edef|xdef|let|newenvironment|renewenvironment)\b`)
	visibleArguments = map[string][]int{"textbf": {1}, "textit": {1}, "textsc": {1}, "emph": {1}, "textrm": {1}, "textsf": {1},
		"texttt": {1}, "textnormal": {1}, "text": {1}, "mbox": {1}, "footnote": {1}, "caption": {1}, "href": {2}}
)

const (
	contextLimit = 128
	maxLineBytes = 262_144
)

// TextIndex is SourceTextIndex over a source's lines (each with its newline).
type TextIndex struct {
	lines    []string
	excluded map[int][][2]int
}

// NewTextIndex is SourceTextIndex.new(text.lines).
func NewTextIndex(text string) *TextIndex {
	lines := []string{}
	for text != "" {
		index := strings.IndexByte(text, '\n')
		if index < 0 {
			lines = append(lines, text)
			break
		}
		lines = append(lines, text[:index+1])
		text = text[index+1:]
	}
	return &TextIndex{lines: lines, excluded: map[int][][2]int{}}
}

// Locate is SourceTextIndex#locate: the one literal match of text in the native line's
// context, or nil.
func (t *TextIndex) Locate(text string, nativeLine int) map[string]any {
	if nativeLine < 1 || nativeLine > len(t.lines) {
		return nil
	}
	found := []map[string]any{}
	for _, index := range t.context(nativeLine - 1) {
		found = append(found, t.matches(index, text)...)
	}
	if len(found) != 1 {
		return nil
	}
	return found[0]
}

func (t *TextIndex) matches(index int, text string) []map[string]any {
	line := t.lines[index]
	if !utf8.ValidString(line) || len(line) > maxLineBytes {
		return nil
	}
	excluded, ok := t.excluded[index]
	if !ok {
		excluded = excludedRanges(line)
		t.excluded[index] = excluded
	}
	result := []map[string]any{}
	runes := []rune(line)
	for _, match := range literalMatches(line, text, excluded) {
		first, last := match[0], match[1]
		result = append(result, map[string]any{"line": int64(index + 1), "characters": []any{int64(first), int64(last)},
			"byte_offsets": []any{int64(len(string(runes[:first]))), int64(len(string(runes[:last])))}})
	}
	return result
}

func (t *TextIndex) context(index int) []int {
	if t.boundary(index) {
		return []int{index}
	}
	first := index
	for first > 0 && index-first < contextLimit && !t.boundary(first-1) {
		first--
	}
	last := index
	for last+1 < len(t.lines) && last-first < contextLimit && !t.boundary(last+1) {
		last++
	}
	if (first > 0 && !t.boundary(first-1)) || (last+1 < len(t.lines) && !t.boundary(last+1)) {
		return nil
	}
	result := []int{}
	for line := first; line <= last; line++ {
		result = append(result, line)
	}
	return result
}

func (t *TextIndex) boundary(index int) bool {
	line := t.lines[index]
	return !utf8.ValidString(line) || len(line) > maxLineBytes || boundary.MatchString(line)
}

func wordCharacter(character rune) bool {
	return unicode.IsLetter(character) || unicode.Is(unicode.M, character) || unicode.IsNumber(character) || character == '_'
}

// literalMatches are the non-overlapping occurrences of text in line, as character offsets,
// outside the excluded ranges and not inside a word.
func literalMatches(line, text string, excluded [][2]int) [][2]int {
	if text == "" || !utf8.ValidString(line) || len(line) > maxLineBytes {
		return nil
	}
	if excluded == nil {
		excluded = excludedRanges(line)
	}
	runes, target := []rune(line), []rune(text)
	result := [][2]int{}
	for first := 0; first+len(target) <= len(runes); {
		if !slices.Equal(runes[first:first+len(target)], target) {
			first++
			continue
		}
		last := first + len(target)
		if keepMatch(runes, target, first, last, excluded) {
			result = append(result, [2]int{first, last})
		}
		first = last
	}
	return result
}

func keepMatch(runes, target []rune, first, last int, excluded [][2]int) bool {
	for _, span := range excluded {
		if first < span[1] && last > span[0] {
			return false
		}
	}
	if first > 0 && wordCharacter(runes[first-1]) && wordCharacter(target[0]) {
		return false
	}
	return !(last < len(runes) && wordCharacter(runes[last]) && wordCharacter(target[len(target)-1]))
}

// excludedRanges: definitions exclude their line; commands, their names and the arguments
// they do not print, and comments, their text.
func excludedRanges(line string) [][2]int {
	runes := []rune(line)
	if definition.MatchString(line) {
		return [][2]int{{0, len(runes)}}
	}
	offsets := byteToRune(line)
	ranges := [][2]int{}
	for _, match := range controlOrComment.FindAllStringIndex(line, -1) {
		first, last := offsets[match[0]], offsets[match[1]]
		ranges = append(ranges, [2]int{first, last})
		token := line[match[0]:match[1]]
		if strings.HasPrefix(token, "%") {
			continue
		}
		allowed := visibleArguments[strings.TrimPrefix(token, `\`)]
		position := 0
		for _, argument := range argumentRanges(runes, last) {
			if !argument.optional {
				position++
			}
			if argument.optional || !slices.Contains(allowed, position) {
				ranges = append(ranges, [2]int{argument.start, argument.finish})
			}
		}
	}
	return ranges
}

func byteToRune(line string) map[int]int {
	offsets := map[int]int{}
	count := 0
	for index := range line {
		offsets[index] = count
		count++
	}
	offsets[len(line)] = count
	return offsets
}

type argument struct {
	start, finish int
	optional      bool
}

func argumentRanges(chars []rune, offset int) []argument {
	ranges := []argument{}
	for {
		for offset < len(chars) && strings.ContainsRune(" \t\n\v\f\r", chars[offset]) {
			offset++
		}
		if offset >= len(chars) || (chars[offset] != '{' && chars[offset] != '[') {
			return ranges
		}
		finish := groupEnd(chars, offset)
		ranges = append(ranges, argument{start: offset, finish: finish, optional: chars[offset] == '['})
		offset = finish
	}
}

func groupEnd(chars []rune, first int) int {
	closer := '}'
	if chars[first] == '[' {
		closer = ']'
	}
	depth := 1
	for index := first + 1; index < len(chars); {
		if chars[index] == '\\' {
			index += 2
			continue
		}
		if chars[index] == chars[first] {
			depth++
		}
		if chars[index] == closer {
			depth--
		}
		if depth == 0 {
			return index + 1
		}
		index++
	}
	return len(chars)
}
