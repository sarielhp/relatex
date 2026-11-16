// Package sourcetext is the Go port of Relatex::SourceVisibleText and SourceAlignment.fold
// (decision 066): what a TeX source file shows when typeset, character by character, with the
// source offsets each visible character came from. Comments, spaces, braces and commands are
// gaps; accents, dashes, quotes and escaped specials become the characters they print; math
// (064's spans) is left out, since formulas are located whole. Control words are recorded as
// macro calls covering their arguments, which still contribute visible text of their own
// unless the command never prints them (\label, \ref, \cite, environment names, …).
//
// Offsets are Unicode scalar offsets into the file, zero-based and half-open.
package sourcetext

import (
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/texaccents"
)

// Unit is one visible character: what it prints, its source extent, its line, and whether a
// gap (space, comment, markup) precedes it.
type Unit struct {
	Char        string
	Start, Stop int
	Line        int
	Gap         bool
}

// Call is a macro call and the source it covers.
type Call struct {
	Name        string
	Start, Stop int
}

var letters = map[string]string{"ss": "ß", "ae": "æ", "AE": "Æ", "oe": "œ", "OE": "Œ", "o": "ø", "O": "Ø",
	"l": "ł", "L": "Ł", "aa": "å", "AA": "Å", "i": "ı", "j": "ȷ", "dag": "†",
	"ddag": "‡", "S": "§", "P": "¶", "copyright": "©", "pounds": "£", "dots": "…",
	"ldots": "…", "textendash": "–", "textemdash": "—", "textquoteright": "’",
	"textquoteleft": "‘", "textquotedblleft": "“", "textquotedblright": "”"}

var symbols = map[rune]string{'%': "%", '&': "&", '$': "$", '#': "#", '_': "_", '{': "{", '}': "}"}

// silent: brace arguments a command reads without printing them.
var silent = map[string]int{"label": 1, "ref": 1, "eqref": 1, "pageref": 1, "autoref": 1, "cref": 1, "Cref": 1,
	"cite": 1, "citep": 1, "citet": 1, "citealp": 1, "citeauthor": 1, "citeyear": 1,
	"nocite": 1, "begin": 1, "end": 1, "usepackage": 1, "documentclass": 1,
	"input": 1, "include": 1, "bibliography": 1, "bibliographystyle": 1,
	"includegraphics": 1, "hspace": 1, "vspace": 1, "setlength": 2, "addtolength": 2,
	"setcounter": 2, "addtocounter": 2, "newcommand": 2, "renewcommand": 2,
	"providecommand": 2, "newenvironment": 3, "renewenvironment": 3, "href": 1,
	"thispagestyle": 1, "pagestyle": 1, "bibitem": 1, "newtheorem": 2, "color": 1,
	"textcolor": 1, "fontsize": 2, "url": 0, "hypersetup": 1, "graphicspath": 1,
	"raisebox": 1, "rotatebox": 1, "scalebox": 1, "resizebox": 2, "vskip": 0,
	"pagenumbering": 1, "numberwithin": 2, "crefname": 3, "DeclareMathOperator": 2}

// quotes in the order the scanner tries them (“|”|---|--|[`']).
var quotes = []struct{ source, char string }{{"``", "“"}, {"''", "”"}, {"---", "—"}, {"--", "–"}, {"`", "‘"}, {"'", "’"}}

// Visible is SourceVisibleText.
type Visible struct {
	text       []rune
	masks      map[int]int
	lineStarts []int
	Units      []Unit
	Calls      []Call
	gap        bool
	position   int
}

// Span is a math span to mask: its Unicode scalar extent.
type Span struct{ Start, Stop int }

// New is SourceVisibleText.new(text, math: spans).
func New(text string, math []Span) *Visible {
	v := &Visible{text: []rune(text), masks: map[int]int{}, lineStarts: []int{0}, gap: true}
	for _, span := range math {
		v.masks[span.Start] = span.Stop
	}
	for index, character := range v.text {
		if character == '\n' {
			v.lineStarts = append(v.lineStarts, index+1)
		}
	}
	for v.position < len(v.text) {
		v.step()
	}
	return v
}

// LineOf is the 1-based line of a character offset.
func (v *Visible) LineOf(offset int) int {
	return sort.Search(len(v.lineStarts), func(index int) bool { return v.lineStarts[index] > offset })
}

// LineStart is the offset a 1-based line starts at; ok is false beyond the last line.
func (v *Visible) LineStart(line int) (int, bool) {
	if line < 1 || line > len(v.lineStarts) {
		return 0, false
	}
	return v.lineStarts[line-1], true
}

// Length is the text's length in characters.
func (v *Visible) Length() int { return len(v.text) }

// At is the character at an offset ("" outside the text).
func (v *Visible) At(offset int) string {
	if offset < 0 || offset >= len(v.text) {
		return ""
	}
	return string(v.text[offset])
}

// ByteOffsets is SourceVisibleText#byte_offsets: the byte lengths of a span's line-relative
// character columns, as editors that count bytes need them.
func (v *Visible) ByteOffsets(line, endLine int, first, last int) (int, int, bool) {
	start, ok := v.LineStart(line)
	finish, endOK := v.LineStart(endLine)
	if !ok || !endOK {
		return 0, 0, false
	}
	return v.byteLength(start, first), v.byteLength(finish, last), true
}

// byteLength is @text[from, count].bytesize.
func (v *Visible) byteLength(from, count int) int {
	stop := min(from+count, len(v.text))
	return len(string(v.text[from:stop]))
}

// FirstUnit is the index of the first unit on or after a line.
func (v *Visible) FirstUnit(line int) int {
	return sort.Search(len(v.Units), func(index int) bool { return v.Units[index].Line >= line })
}

// UnitsWithin is the units inside [start, stop), in order.
func (v *Visible) UnitsWithin(start, stop int) []Unit {
	from := sort.Search(len(v.Units), func(index int) bool { return v.Units[index].Start >= start })
	result := []Unit{}
	for _, unit := range v.Units[from:] {
		if unit.Start >= stop {
			break
		}
		if unit.Stop <= stop {
			result = append(result, unit)
		}
	}
	return result
}

// TextBetween is @text[start...stop].
func (v *Visible) TextBetween(start, stop int) string {
	start, stop = max(start, 0), min(stop, len(v.text))
	if start >= stop {
		return ""
	}
	return string(v.text[start:stop])
}

func (v *Visible) peek(offset int) rune {
	if v.position+offset >= len(v.text) {
		return -1
	}
	return v.text[v.position+offset]
}

func (v *Visible) hasPrefix(prefix string) bool {
	runes := []rune(prefix)
	if v.position+len(runes) > len(v.text) {
		return false
	}
	for index, character := range runes {
		if v.text[v.position+index] != character {
			return false
		}
	}
	return true
}

// space is Ruby's \s.
func space(character rune) bool {
	return character == ' ' || character == '\t' || character == '\n' || character == '\v' || character == '\f' || character == '\r'
}

func letter(character rune) bool {
	return (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z')
}

func (v *Visible) step() {
	if stop, ok := v.masks[v.position]; ok {
		v.position = max(stop, v.position)
		v.gap = true
		return
	}
	character := v.peek(0)
	switch {
	case character == '%':
		for v.position < len(v.text) && v.text[v.position] != '\n' {
			v.position++
		}
		v.gap = true
	case character == '\\':
		v.command()
	case space(character) || character == '{' || character == '}' || character == '~':
		v.position++
		v.gap = true
	case character == '`' || character == '\'' || v.hasPrefix("--"):
		v.quote()
	default:
		start := v.position
		v.position++
		v.emit(string(character), start, v.position)
	}
}

func (v *Visible) quote() {
	start := v.position
	for _, quote := range quotes {
		if v.hasPrefix(quote.source) {
			v.position += len([]rune(quote.source))
			v.emit(quote.char, start, v.position)
			return
		}
	}
}

func (v *Visible) emit(char string, start, stop int) {
	v.Units = append(v.Units, Unit{Char: char, Start: start, Stop: stop, Line: v.LineOf(start), Gap: v.gap})
	v.gap = false
}

func (v *Visible) command() {
	start := v.position
	if stop, mark, base, ok := v.accent(); ok {
		v.position = stop
		v.accented(mark, base, start)
		return
	}
	if next := v.peek(1); next >= 0 {
		if char, ok := symbols[next]; ok {
			v.position += 2
			v.emit(char, start, v.position)
			return
		}
	}
	if v.verbAhead() {
		v.verbatim(start)
		return
	}
	name := v.controlWord()
	if name == "" {
		v.skipControlSymbol()
		v.gap = true
		return
	}
	v.word(name, start)
}

// controlWord scans \\([A-Za-z@]+)\*? and returns the name without the star, or "".
func (v *Visible) controlWord() string {
	index := v.position + 1
	for index < len(v.text) && (letter(v.text[index]) || v.text[index] == '@') {
		index++
	}
	if index == v.position+1 {
		return ""
	}
	name := string(v.text[v.position+1 : index])
	if index < len(v.text) && v.text[index] == '*' {
		index++
	}
	v.position = index
	return name
}

// skipControlSymbol scans \\\\(?:\s*\[[^\]]*\])? or \\. (any character, newline included).
func (v *Visible) skipControlSymbol() {
	if v.peek(1) == '\\' {
		v.position += 2
		index := v.position
		for index < len(v.text) && space(v.text[index]) {
			index++
		}
		if index < len(v.text) && v.text[index] == '[' {
			closing := index + 1
			for closing < len(v.text) && v.text[closing] != ']' {
				closing++
			}
			if closing < len(v.text) {
				v.position = closing + 1
			}
		}
		return
	}
	// A lone backslash at the end of the file: Ruby's scanner would not advance; step past it.
	v.position = min(v.position+2, len(v.text))
}

// accent matches \\(['`^"~=.])\s*(?:\{\s*(\\?[A-Za-z])\s*\}|(\\?[A-Za-z])) or
// \\([uvHckrdb])(?:\s*\{\s*(\\?[A-Za-z])\s*\}|\s+([A-Za-z])) at the position.
func (v *Visible) accent() (stop int, mark rune, base string, ok bool) {
	mark = v.peek(1)
	if strings.ContainsRune("'`^\"~=.", mark) && mark >= 0 {
		index := v.skipSpaces(v.position + 2)
		if end, letters, found := v.bracedLetter(index); found {
			return end, mark, letters, true
		}
		if end, letters, found := v.bareLetter(index, true); found {
			return end, mark, letters, true
		}
		return 0, 0, "", false
	}
	if strings.ContainsRune("uvHckrdb", mark) && mark >= 0 {
		if end, letters, found := v.bracedLetter(v.skipSpaces(v.position + 2)); found {
			return end, mark, letters, true
		}
		index := v.skipSpaces(v.position + 2)
		if index > v.position+2 {
			if end, letters, found := v.bareLetter(index, false); found {
				return end, mark, letters, true
			}
		}
	}
	return 0, 0, "", false
}

func (v *Visible) skipSpaces(index int) int {
	for index < len(v.text) && space(v.text[index]) {
		index++
	}
	return index
}

// bracedLetter matches \{\s*(\\?[A-Za-z])\s*\} at index.
func (v *Visible) bracedLetter(index int) (int, string, bool) {
	if index >= len(v.text) || v.text[index] != '{' {
		return 0, "", false
	}
	end, base, found := v.bareLetter(v.skipSpaces(index+1), true)
	if !found {
		return 0, "", false
	}
	end = v.skipSpaces(end)
	if end >= len(v.text) || v.text[end] != '}' {
		return 0, "", false
	}
	return end + 1, base, true
}

// bareLetter matches (\\?[A-Za-z]) (or [A-Za-z] without escape) at index.
func (v *Visible) bareLetter(index int, escape bool) (int, string, bool) {
	if escape && index < len(v.text) && v.text[index] == '\\' {
		if index+1 < len(v.text) && letter(v.text[index+1]) {
			return index + 2, string(v.text[index+1]), true
		}
		return 0, "", false
	}
	if index < len(v.text) && letter(v.text[index]) {
		return index + 1, string(v.text[index]), true
	}
	return 0, "", false
}

func (v *Visible) accented(mark rune, base string, start int) {
	if letter, ok := letters[base]; ok {
		base = letter
	}
	v.emit(norm.NFC.String(base+texaccents.Combining[string(mark)]), start, v.position)
}

// verbAhead is \\verb\*?[^A-Za-z\s*].
func (v *Visible) verbAhead() bool {
	if !v.hasPrefix(`\verb`) {
		return false
	}
	index := v.position + 5
	if index < len(v.text) && v.text[index] == '*' {
		index++
	}
	if index >= len(v.text) {
		return false
	}
	delimiter := v.text[index]
	return !letter(delimiter) && !space(delimiter) && delimiter != '*'
}

// verbatim: \verb|text|: the characters between the delimiters print as they are.
func (v *Visible) verbatim(start int) {
	v.position += 5
	if v.peek(0) == '*' {
		v.position++
	}
	delimiter := v.text[v.position]
	v.position++
	v.gap = true
	for v.position < len(v.text) && v.text[v.position] != delimiter {
		at := v.position
		character := v.text[v.position]
		v.position++
		if space(character) {
			v.gap = true
		} else {
			v.emit(string(character), at, v.position)
		}
	}
	if v.position < len(v.text) {
		v.position++
	}
	v.Calls = append(v.Calls, Call{Name: "verb", Start: start, Stop: v.position})
	v.gap = true
}

func (v *Visible) word(name string, start int) {
	if letter, ok := letters[name]; ok {
		for v.position < len(v.text) && (v.text[v.position] == ' ' || v.text[v.position] == '\t') {
			v.position++
		}
		v.emit(letter, start, v.position)
		return
	}
	for range silent[name] {
		v.skipGroup()
	}
	v.Calls = append(v.Calls, Call{Name: name, Start: start, Stop: v.extent(v.position)})
	v.gap = true
}

// skipGroup skips one optional and one brace argument the command does not print.
func (v *Visible) skipGroup() {
	index := v.skipSpaces(v.position)
	if index < len(v.text) && v.text[index] == '[' {
		closing := index + 1
		for closing < len(v.text) && v.text[closing] != ']' {
			closing++
		}
		if closing < len(v.text) {
			index = v.skipSpaces(closing + 1)
		}
	}
	v.position = index
	if v.position < len(v.text) && v.text[v.position] == '{' {
		v.position = v.matching(v.position)
	}
}

// extent: the call's extent: through its silent arguments and then any brace groups that
// follow directly, so \textbf{word} covers its printed argument.
func (v *Visible) extent(from int) int {
	stop := from
	for stop < len(v.text) && v.text[stop] == '{' && v.matching(stop) > stop {
		stop = v.matching(stop)
	}
	return stop
}

// matching is the offset just past the brace group opening at from; escaped braces do not
// count.
func (v *Visible) matching(from int) int {
	depth := 0
	for index := from; index < len(v.text); {
		character := v.text[index]
		if character == '\\' {
			index++
		}
		if character == '{' {
			depth++
		}
		if character == '}' {
			depth--
		}
		index++
		if depth == 0 {
			return index
		}
	}
	return from
}

// Fold is SourceAlignment.fold: NFKC, with typographic quotes folded to straight ones.
func Fold(text string) string {
	return quoteFold.Replace(norm.NFKC.String(text))
}

var quoteFold = strings.NewReplacer("‘", "'", "’", "'", "‚", "'", "′", "'", "“", `"`, "”", `"`, "„", `"`, "″", `"`)

// ScalarLength is a string's length in Unicode scalars (Ruby's String#length).
func ScalarLength(text string) int { return utf8.RuneCountInString(text) }
