// Package mathsource is MathSourceText: where the formulas of a TeX source file are (inline,
// display and environment math, their rows and bodies), as the source map, its validation and
// the formula verification read them (decision 102: a model package, read by the core and the
// bundle reader alike).
package mathsource

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	"github.com/sarielhp/relatex/internal/rec"
)

// mathEnvironments and verbatimEnvironments are MathSourceText's.
var (
	mathEnvironments = []string{"equation", "equation*", "align", "align*", "gather", "gather*", "multline", "multline*",
		"flalign", "flalign*", "alignat", "alignat*", "eqnarray", "eqnarray*", "displaymath", "math"}
	verbatimEnvironments = []string{"verbatim", "verbatim*", "Verbatim", "lstlisting", "minted", "comment"}
	// rubySpace is Ruby's \s.
	rowTokens        = regexp.MustCompile(`(?s)%[^\n]*|\\\\(?:[ \t\n\v\f\r]*\[[^\]]*\])?|\\(?:begin|end)\b|\\(?:[A-Za-z]+|.)|[{}]|[^\\{}%]+`)
	commandName      = regexp.MustCompile(`(?s)\A\\(?:[A-Za-z]+\*?|.)`)
	shortCommand     = regexp.MustCompile(`(?s)\A\\(?:[A-Za-z]+|.)`)
	beginEnvironment = regexp.MustCompile(`\A\\begin[ \t\n\v\f\r]*\{[^{}]+\}`)
	environmentName  = regexp.MustCompile(`\{(.+)\}`)
)

// Span is one math span of a source file: its form (inline, display, environment), its
// environment, the lines it starts and ends on, its body and, for source synchronization
// (decision 066), the whole formula with its delimiters as Unicode scalar offsets.
type Span struct {
	Form, Environment string
	Line, EndLine     int64
	Body              string
	Start, Stop       int
}

// SourceText is MathSourceText: the math spans of a TeX source file. Comments, escaped
// dollars, verbatim and math nested in \text{…} are not spans of their own. Offsets are in
// bytes inside; spans report Unicode scalar offsets, as Ruby does.
type SourceText struct {
	text       string
	textMacros []string
	lineStarts []int
	Spans      []*Span
	// counted: the Unicode scalar offset of byte countedByte, so offsets count forward.
	countedByte, counted int
}

// NewSourceText is MathSourceText.new(text, text_macros:).
func NewSourceText(text string, textMacros []string) *SourceText {
	s := &SourceText{text: text, lineStarts: []int{0}}
	for _, name := range textMacros {
		s.textMacros = append(s.textMacros, "\\"+name)
	}
	for index := 0; index < len(text); index++ {
		if text[index] == '\n' {
			s.lineStarts = append(s.lineStarts, index+1)
		}
	}
	s.scan()
	return s
}

// Rows is MathSourceText.rows: the rows of an alignment body, split at \\ outside braces and
// nested environments, with a trailing spacing argument (\\[2pt]) dropped.
func Rows(body string) []string {
	rows := []string{""}
	depth := 0
	for _, token := range rowTokens.FindAllString(body, -1) {
		if depth == 0 && strings.HasPrefix(token, `\\`) {
			rows = append(rows, "")
			continue
		}
		if strings.HasPrefix(token, "%") {
			continue
		}
		switch token {
		case `\begin`, "{":
			depth++
		case `\end`, "}":
			depth--
		}
		rows[len(rows)-1] += token
	}
	result := []string{}
	for _, row := range rows {
		if stripped := rec.Strip(row); stripped != "" {
			result = append(result, stripped)
		}
	}
	return result
}

// lineOf is the zero-based line of a byte offset.
func (s *SourceText) lineOf(offset int) int {
	index := sort.Search(len(s.lineStarts), func(i int) bool { return s.lineStarts[i] > offset })
	if index == len(s.lineStarts) {
		return len(s.lineStarts) - 1
	}
	return index - 1
}

func (s *SourceText) scan() {
	index := 0
	for index < len(s.text) {
		switch s.text[index] {
		case '%':
			index = s.commentEnd(index)
		case '\\':
			index = s.command(index)
		case '$':
			index = s.dollar(index)
		default:
			index++
		}
	}
}

func (s *SourceText) commentEnd(index int) int {
	if end := strings.IndexByte(s.text[index:], '\n'); end >= 0 {
		return index + end
	}
	return len(s.text)
}

// at is the text a pattern (anchored with \A) matches starting exactly at index.
func (s *SourceText) at(pattern *regexp.Regexp, index int) string {
	return pattern.FindString(s.text[index:])
}

func (s *SourceText) command(index int) int {
	name := s.at(commandName, index)
	if name == "" {
		return index + 1
	}
	switch {
	case name == `\(`:
		return s.math(index, len(name), `\)`, "inline")
	case name == `\[`:
		return s.math(index, len(name), `\]`, "display")
	case name == `\begin`:
		return s.environment(index)
	case name == `\verb` || name == `\verb*`:
		return s.verb(index + len(name))
	case slices.Contains(s.textMacros, name):
		s.add("inline", "", index, index+len(name)-1, name)
		return index + len(name)
	}
	return index + len(name)
}

func (s *SourceText) dollar(index int) int {
	if index+1 < len(s.text) && s.text[index+1] == '$' {
		return s.math(index, 2, "$$", "display")
	}
	return s.math(index, 1, "$", "inline")
}

func (s *SourceText) verb(index int) int {
	if index >= len(s.text) {
		return index
	}
	delimiter, width := utf8.DecodeRuneInString(s.text[index:])
	if end := strings.IndexRune(s.text[index+width:], delimiter); end >= 0 {
		return index + width + end + utf8.RuneLen(delimiter)
	}
	return len(s.text) + 1
}

func (s *SourceText) environment(index int) int {
	found := s.at(beginEnvironment, index)
	if found == "" {
		return index + len(`\begin`)
	}
	name := environmentName.FindStringSubmatch(found)[1]
	bodyStart := index + len(found)
	closing, closed := s.environmentEnd(name, bodyStart)
	if slices.Contains(verbatimEnvironments, name) {
		if closed {
			return closing[1]
		}
		return len(s.text)
	}
	if !slices.Contains(mathEnvironments, name) || !closed {
		return bodyStart
	}
	s.add("environment", name, index, closing[1]-1, s.text[bodyStart:closing[0]])
	return closing[1]
}

// environmentEnd is the matching \end{name}: its offset and the offset after it, counting
// nested \begin{name}. A comment (any %, as Ruby's pattern finds it) is skipped.
func (s *SourceText) environmentEnd(name string, from int) ([2]int, bool) {
	pattern := endPattern(name)
	depth := 1
	position := from
	for position <= len(s.text) {
		match := pattern.FindStringSubmatchIndex(s.text[position:])
		if match == nil {
			break
		}
		start, end := position+match[0], position+match[1]
		kind := ""
		if match[2] >= 0 {
			kind = s.text[position+match[2] : position+match[3]]
		}
		if end == position {
			position++ // both alternatives consume a character; a guard against an empty match
		} else {
			position = end
		}
		if kind == "" {
			continue
		}
		if kind == "begin" {
			depth++
		} else {
			depth--
		}
		if depth == 0 {
			return [2]int{start, end}, true
		}
	}
	return [2]int{}, false
}

var endPatterns sync.Map

func endPattern(name string) *regexp.Regexp {
	if cached, ok := endPatterns.Load(name); ok {
		return cached.(*regexp.Regexp)
	}
	pattern := regexp.MustCompile(`\\(begin|end)[ \t\n\v\f\r]*\{` + regexp.QuoteMeta(name) + `\}|%[^\n]*`)
	endPatterns.Store(name, pattern)
	return pattern
}

// math: a formula body up to its closing delimiter at brace depth 0; $ inside braces is
// math nested in text (\text{if $x$}), not the end.
func (s *SourceText) math(index, opening int, closer, form string) int {
	finish, found := s.mathEnd(index+opening, closer)
	if !found {
		return len(s.text)
	}
	s.add(form, "", index, finish+len(closer)-1, s.text[index+opening:finish])
	return finish + len(closer)
}

func (s *SourceText) mathEnd(position int, closer string) (int, bool) {
	depth := 0
	for position < len(s.text) {
		if depth == 0 && strings.HasPrefix(s.text[position:], closer) {
			return position, true
		}
		switch character := s.text[position]; character {
		case '%':
			position = s.commentEnd(position)
		case '\\':
			if token := s.at(shortCommand, position); token != "" {
				position += len(token)
			} else {
				position++
			}
		default:
			if character == '{' {
				depth++
			} else if character == '}' && depth > 0 {
				depth--
			}
			position += characterWidth(s.text, position)
		}
	}
	return 0, false
}

func characterWidth(text string, position int) int {
	if text[position] < utf8.RuneSelf {
		return 1
	}
	_, width := utf8.DecodeRuneInString(text[position:])
	return width
}

// add records a span from byte from to byte to (inclusive of to's character start).
func (s *SourceText) add(form, environment string, from, to int, body string) {
	stop := to + characterWidth(s.text, to)
	s.Spans = append(s.Spans, &Span{Form: form, Environment: environment, Line: int64(s.lineOf(from) + 1),
		EndLine: int64(s.lineOf(to) + 1), Body: body, Start: s.scalarOffset(from),
		Stop: s.scalarOffset(stop)})
}

// scalarOffset is the Unicode scalar offset of a byte offset; spans arrive in source order, so
// counting goes on from the last offset.
func (s *SourceText) scalarOffset(offset int) int {
	if offset < s.countedByte {
		s.countedByte, s.counted = 0, 0
	}
	s.counted += utf8.RuneCountInString(s.text[s.countedByte:offset])
	s.countedByte = offset
	return s.counted
}

// RowRanges is Rows with positions: the byte range inside body of each non-blank row, trimmed
// of surrounding white space, split at \\ outside braces and nested environments.
func RowRanges(body string) [][2]int {
	ranges := [][2]int{}
	start, depth := 0, 0
	trimmed := func(from, to int) {
		text := body[from:to]
		left := len(text) - len(strings.TrimLeft(text, " \t\n\v\f\r"))
		right := len(strings.TrimRight(text, " \t\n\v\f\r"))
		if right > left {
			ranges = append(ranges, [2]int{from + left, from + right})
		}
	}
	for _, location := range rowTokens.FindAllStringIndex(body, -1) {
		token := body[location[0]:location[1]]
		if depth == 0 && strings.HasPrefix(token, `\\`) {
			trimmed(start, location[0])
			start = location[1]
			continue
		}
		if strings.HasPrefix(token, "%") {
			continue
		}
		switch token {
		case `\begin`, "{":
			depth++
		case `\end`, "}":
			depth--
		}
	}
	trimmed(start, len(body))
	return ranges
}

// rowEnvironments are the displays whose rows are typeset as separate formulas, one per row.
var rowEnvironments = []string{"align", "align*", "gather", "gather*", "flalign", "flalign*", "alignat", "alignat*",
	"eqnarray", "eqnarray*"}

// RowSpans are the Unicode scalar ranges, in text, of the rows of a multi-row display span
// (align, gather, ...): nil for any other span, or when the span's body cannot be found in
// the text.
func RowSpans(text string, span *Span) [][2]int {
	if span.Form != "environment" || !slices.Contains(rowEnvironments, span.Environment) || span.Body == "" {
		return nil
	}
	first, last := byteOffset(text, span.Start), byteOffset(text, span.Stop)
	if first < 0 || last < first {
		return nil
	}
	within := strings.Index(text[first:last], span.Body)
	if within < 0 {
		return nil
	}
	body, bodyStart := span.Body, first+within
	if strings.HasPrefix(span.Environment, "alignat") {
		// \begin{alignat}{n}: the column count is not part of the first row.
		skip := len(columnArgument(body))
		body, bodyStart = body[skip:], bodyStart+skip
	}
	ranges := RowRanges(body)
	rows := make([][2]int, len(ranges))
	for index, r := range ranges {
		rows[index] = [2]int{scalarCount(text, bodyStart+r[0]), scalarCount(text, bodyStart+r[1])}
	}
	return rows
}

// columnArgument is an environment body's leading {...} group with the white space before it.
func columnArgument(body string) string {
	trimmed := strings.TrimLeft(body, " \t\n\r")
	if !strings.HasPrefix(trimmed, "{") {
		return ""
	}
	end := strings.IndexByte(trimmed, '}')
	if end < 0 {
		return ""
	}
	return body[:len(body)-len(trimmed)+end+1]
}

// byteOffset is the byte offset of a Unicode scalar offset, -1 beyond the text.
func byteOffset(text string, scalar int) int {
	if scalar < 0 {
		return -1
	}
	index := indexOf(text)
	if index.ascii {
		if scalar > len(text) {
			return -1
		}
		return scalar
	}
	if scalar < len(index.starts) {
		return int(index.starts[scalar])
	}
	if scalar == len(index.starts) {
		return len(text)
	}
	return -1
}

// scalarCount is the number of Unicode scalars in text[:offset].
func scalarCount(text string, offset int) int {
	index := indexOf(text)
	if index.ascii {
		return offset
	}
	return sort.Search(len(index.starts), func(i int) bool { return int(index.starts[i]) >= offset })
}

// textIndex is where each scalar of one text starts; a text of ASCII alone needs none. A source is
// asked about thousands of times (once per word of a math display), so the last text is remembered.
type textIndex struct {
	data   *byte
	length int
	ascii  bool
	starts []int32
}

var (
	indexLock sync.Mutex
	lastIndex textIndex
)

func indexOf(text string) textIndex {
	indexLock.Lock()
	defer indexLock.Unlock()
	data := unsafe.StringData(text)
	if lastIndex.data == data && lastIndex.length == len(text) {
		return lastIndex
	}
	index := textIndex{data: data, length: len(text), ascii: true}
	for position := 0; position < len(text); position++ {
		if text[position] >= utf8.RuneSelf {
			index.ascii = false
			break
		}
	}
	if !index.ascii {
		index.starts = make([]int32, 0, len(text))
		for position := range text {
			index.starts = append(index.starts, int32(position))
		}
	}
	lastIndex = index
	return index
}

// BodyRange is the Unicode scalar range, in text, of a math span's body: what lies between
// the delimiters (the \begin{..} and \end{..}, the dollars). ok is false when the body cannot
// be found in the text.
func BodyRange(text string, span *Span) (start, stop int, ok bool) {
	first, last := byteOffset(text, span.Start), byteOffset(text, span.Stop)
	if first < 0 || last < first || span.Body == "" {
		return 0, 0, false
	}
	within := strings.Index(text[first:last], span.Body)
	if within < 0 {
		return 0, 0, false
	}
	return scalarCount(text, first+within), scalarCount(text, first+within+len(span.Body)), true
}
