package synchronize

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcetext"
)

// reach is SourceAlignment::REACH: the source window, in lines, searched back from a word's
// bracket when an argument was absorbed before being typeset (footnotes, captions, headings
// record their last line).
const reach = 40

// maxFragments is SourceWordGroups::MAX_FRAGMENTS.
const maxFragments = 32

// hyphens is SourceAlignment::HYPHENS.
var hyphens = []string{"-", "\u2010", "\u00ad"}

// silentCalls is SourceAlignment::SILENT_CALLS: calls that print nothing of their own.
var silentCalls = []string{"label", "noindent", "vspace", "hspace", "medskip", "bigskip", "smallskip", "par", "newline",
	"linebreak", "pagebreak", "newpage", "clearpage", "centering", "raggedright", "small", "large", "Large", "footnotesize",
	"scriptsize", "tiny", "normalsize", "textbf", "textit", "emph", "textrm", "textsf", "texttt", "textsc", "textup", "textmd",
	"mbox", "hbox", "bf", "it", "rm", "sf", "tt", "em", "quad", "qquad", "enspace", "nobreak", "protect", "relax"}

// item is SourceAlignment::Item: a word to place, its typeset characters (folded), the glyph
// each came from, and the lines TeX recorded around it.
type item struct {
	key, file     string
	before, after int64
	chars         []string
	lineEnd       bool
	hyphen        bool
}

// placement is SourceAlignment::Placement: where a word landed (precision exact, macro or
// line), as source character offsets, or lines.
type placement struct {
	precision   string
	file        string
	start, stop int
	lines       [2]int64
}

// streamChar is one folded character of a file's visible text: the unit it belongs to and
// whether a gap precedes it.
type streamChar struct {
	char string
	unit int
	gap  bool
}

// alignment is SourceAlignment.
type alignment struct {
	texts      *omap.Map[string, *sourcetext.Visible]
	streams    map[string][]streamChar
	cursor     map[string]int
	placements map[string]*placement
	chains     [][]string
}

func newAlignment(texts *omap.Map[string, *sourcetext.Visible]) *alignment {
	a := &alignment{texts: texts, streams: map[string][]streamChar{}, cursor: map[string]int{}}
	texts.Each(func(name string, visible *sourcetext.Visible) { a.streams[name] = stream(visible) })
	return a
}

func stream(visible *sourcetext.Visible) []streamChar {
	chars := []streamChar{}
	for index, unit := range visible.Units {
		position := 0
		for _, char := range sourcetext.Fold(unit.Char) {
			chars = append(chars, streamChar{char: string(char), unit: index, gap: position == 0 && unit.Gap})
			position++
		}
	}
	return chars
}

func (a *alignment) visible(file string) *sourcetext.Visible {
	visible, _ := a.texts.Get(file)
	return visible
}

// align places items (in reading order); it returns key => placement.
func (a *alignment) align(items []*item) map[string]*placement {
	a.placements = map[string]*placement{}
	a.chains = [][]string{}
	for index := 0; index < len(items); {
		consumed := 0
		if _, ok := a.streams[items[index].file]; ok {
			consumed = a.place(items, index)
		}
		index += 1 + consumed
	}
	a.attributeMacros(items)
	return a.placements
}

// place returns how many items it consumed beyond the current one (the rest of a hyphenated
// word).
func (a *alignment) place(items []*item, index int) int {
	current := items[index]
	if found, ok := a.find(current.file, current.chars, current.before, current.after); ok {
		a.record(current, found)
		return 0
	}
	if joined, ok := a.hyphenated(items, index); ok {
		return joined
	}
	a.punctuation(current)
	return 0
}

func (a *alignment) record(current *item, found [2]int) {
	first, last := found[0], found[1]
	if last < first {
		rec.Fail("Empty source placement")
	}
	units := a.visible(current.file).Units
	stream := a.streams[current.file]
	a.cursor[current.file] = last + 1
	a.placements[current.key] = &placement{precision: "exact", file: current.file, start: units[stream[first].unit].Start,
		stop: units[stream[last].unit].Stop}
}

// hyphenated: a word broken at discretionaries over one or more line ends: each piece but the
// last ends in TeX's hyphen, which is not in the source; the pieces join into one source word.
// It returns the number of pieces after the first.
func (a *alignment) hyphenated(items []*item, index int) (int, bool) {
	chain := []*item{items[index]}
	for chain[len(chain)-1].lineEnd && chain[len(chain)-1].hyphen && len(chain) < maxFragments {
		next := index + len(chain)
		if next >= len(items) || items[next].file != chain[0].file {
			break
		}
		chain = append(chain, items[next])
	}
	if len(chain) < 2 {
		return 0, false
	}
	pieces := make([][]string, len(chain))
	joined := []string{}
	lastLine := int64(math64Min)
	for at, piece := range chain {
		pieces[at] = piece.chars
		if at < len(chain)-1 && len(piece.chars) > 0 {
			pieces[at] = piece.chars[:len(piece.chars)-1]
		}
		// A piece that is only TeX's hyphen has nothing to place.
		if len(pieces[at]) == 0 {
			return 0, false
		}
		joined = append(joined, pieces[at]...)
		lastLine = max(lastLine, max(piece.after, piece.before))
	}
	found, ok := a.find(chain[0].file, joined, chain[0].before, lastLine)
	if !ok {
		return 0, false
	}
	position := found[0]
	keys := make([]string, len(chain))
	for at, piece := range chain {
		a.record(piece, [2]int{position, position + len(pieces[at]) - 1})
		position += len(pieces[at])
		keys[at] = piece.key
	}
	a.chains = append(a.chains, keys)
	return len(chain) - 1, true
}

const math64Min = -1 << 63

// trailingPunctuation is the length of SourceAlignment::PUNCTUATION's match (/[.,;:!?)\]]+\z/).
func trailingPunctuation(chars []string) int {
	count := 0
	for index := len(chars) - 1; index >= 0 && strings.Contains(".,;:!?)]", chars[index]) && len(chars[index]) == 1; index-- {
		count++
	}
	return count
}

// punctuation: a word whose trailing punctuation sits outside the call that produced the word
// (`\cite{x}.`) is matched without it; the punctuation is placed only if the source has it
// right after.
func (a *alignment) punctuation(current *item) {
	cut := trailingPunctuation(current.chars)
	core := current.chars[:len(current.chars)-cut]
	if len(core) == 0 || cut == 0 {
		return
	}
	found, ok := a.find(current.file, core, current.before, current.after)
	if !ok {
		return
	}
	stream := a.streams[current.file]
	for offset, char := range current.chars[len(core):] {
		at := found[1] + 1 + offset
		if at >= len(stream) || stream[at].char != char {
			return
		}
	}
	a.record(current, [2]int{found[0], found[1] + cut})
}

// find is the start and end positions in the file's stream of the word's characters. The
// wider window back is for words of three or more characters: single letters and signs would
// match too much. Punctuation alone may follow other text without a gap (`};.`).
func (a *alignment) find(file string, chars []string, before, after int64) ([2]int, bool) {
	if len(chars) == 0 {
		return [2]int{}, false
	}
	high := max(after, before) + 1
	candidates := a.matches(file, before, high, chars, true)
	if len(candidates) == 0 && len(chars) >= 3 {
		candidates = a.matches(file, before-reach, high, chars, true)
	}
	if len(candidates) == 0 && allPunctuation(chars) {
		candidates = a.matches(file, before, high, chars, false)
	}
	start, ok := choose(candidates, a.cursor[file])
	return [2]int{start, start + len(chars) - 1}, ok
}

func allPunctuation(chars []string) bool {
	for _, char := range chars {
		for _, character := range char {
			if !punct(character) {
				return false
			}
		}
	}
	return true
}

// punct is Onigmo's [[:punct:]] on UTF-8: Unicode punctuation and the ASCII symbols.
func punct(character rune) bool {
	return unicode.IsPunct(character) || (character < 0x80 && unicode.IsSymbol(character))
}

func (a *alignment) matches(file string, low, high int64, chars []string, startGap bool) []int {
	visible := a.visible(file)
	stream := a.streams[file]
	firstUnit := visible.FirstUnit(int(max(low, 1)))
	lastUnit := visible.FirstUnit(int(high + 1))
	first := sort.Search(len(stream), func(index int) bool { return stream[index].unit >= firstUnit })
	last := sort.Search(len(stream), func(index int) bool { return stream[index].unit >= lastUnit })
	result := []int{}
	for position := first; position < last; position++ {
		if a.match(stream, position, chars, startGap) {
			result = append(result, position)
		}
	}
	return result
}

func (a *alignment) match(stream []streamChar, position int, chars []string, startGap bool) bool {
	if startGap && position != 0 && !stream[position].gap {
		return false
	}
	for offset, char := range chars {
		if position+offset >= len(stream) || stream[position+offset].char != char {
			return false
		}
	}
	next := position + len(chars)
	return next >= len(stream) || stream[next].gap || stream[next].unit == stream[next-1].unit
}

// choose is the first candidate at or after the reading cursor, else the last one before it.
func choose(candidates []int, cursor int) (int, bool) {
	for _, position := range candidates {
		if position >= cursor {
			return position, true
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	return candidates[len(candidates)-1], true
}

// attributeMacros: unplaced words between two placed ones in the same file take the macro
// call that produced them: the call whose extent covers the next placed word's start, else
// the last call before it, provided TeX's lines for the word allow that call's line.
func (a *alignment) attributeMacros(items []*item) {
	for index, current := range items {
		if _, placed := a.placements[current.key]; placed {
			continue
		}
		if _, ok := a.streams[current.file]; !ok {
			continue
		}
		if call, ok := a.producingCall(items, index); ok {
			a.placements[current.key] = &placement{precision: "macro", file: current.file, start: call.Start, stop: call.Stop}
			continue
		}
		a.placements[current.key] = &placement{precision: "line", file: current.file,
			lines: [2]int64{current.before, max(current.after, current.before)}}
	}
}

// producingCall: the file's start and end stand in for a missing neighbour. When the previous
// word comes later in the source (a footnote typeset at the page's foot), only a call covering
// the next placed word can have produced this one.
func (a *alignment) producingCall(items []*item, index int) (sourcetext.Call, bool) {
	current := items[index]
	low, high, finite := 0, 0, false
	if previous := a.neighbour(items, index, -1); previous != nil {
		low = previous.stop
	}
	if next := a.neighbour(items, index, 1); next != nil {
		high, finite = next.start, true
	}
	enclosingOnly := finite && low > high
	if enclosingOnly {
		low = 0
	}
	visible := a.visible(current.file)
	calls := []sourcetext.Call{}
	for _, call := range visible.Calls {
		if call.Start >= low && (!finite || call.Start < high) && a.onLines(current, visible, call, high, finite) &&
			!documentBoundary(visible, call) {
			calls = append(calls, call)
		}
	}
	covering, found := sourcetext.Call{}, false
	for at := len(calls) - 1; at >= 0 && finite; at-- {
		if calls[at].Stop > high {
			covering, found = calls[at], true
			break
		}
	}
	if enclosingOnly {
		return covering, found
	}
	for _, call := range calls {
		if !slices.Contains(silentCalls, call.Name) {
			return call, true
		}
	}
	return covering, found
}

// onLines: a call within the lines TeX bracketed the word with, or on the line before when
// nothing but space separates it from the next placed word: a heading such as
// \begin{proof}'s "Proof." is typeset only when the paragraph after it starts.
func (a *alignment) onLines(current *item, visible *sourcetext.Visible, call sourcetext.Call, high int, finite bool) bool {
	line := int64(visible.LineOf(call.Start))
	if line >= current.before && line <= max(current.after, current.before) {
		return true
	}
	return line == current.before-1 && finite && rec.Strip(visible.TextBetween(call.Stop, high)) == ""
}

// documentBoundary: \begin{document} and \end{document} produce page furniture, not text.
func documentBoundary(visible *sourcetext.Visible, call sourcetext.Call) bool {
	return (call.Name == "begin" || call.Name == "end") && strings.Contains(visible.TextBetween(call.Start, call.Stop), "{document}")
}

func (a *alignment) neighbour(items []*item, index, step int) *placement {
	file := items[index].file
	for cursor := index + step; cursor >= 0 && cursor < len(items); cursor += step {
		found := a.placements[items[cursor].key]
		if found != nil && found.precision == "exact" && items[cursor].file == file {
			return found
		}
	}
	return nil
}
