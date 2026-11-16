package synchronize

import (
	stdpath "path"
	"strings"

	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/mathsource"
	"github.com/sarielhp/relatex/internal/mathtokens"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// formulaSource is one shipped formula and the source location the collector recorded for it.
type formulaSource struct {
	formula int64
	source  rec.Map
}

type formulaSpan struct {
	file string
	span *mathsource.Span
}

// mathPlacement is MathPlacement (decision 066): the source span of each typeset formula,
// from the location the collector recorded for it (064) and the formula spans MathSourceText
// finds in the file.
//  1. Shipped formulas sharing a recorded location take the spans on exactly those lines in
//     reading order; formulas sharing a single span are the rows or parts of one display.
//  2. A location matching no span exactly (a display recorded from the line after
//     \begin{equation}) takes the innermost span that contains it.
//  3. A formula without a location (nested, or typeset from a macro) takes the one span that
//     overlaps the lines TeX bracketed its glyphs with, when there is exactly one.
//
// Anything else keeps its lines only.
type mathPlacement struct {
	texts *omap.Map[string, *source]
	files map[string]string
	spans map[int64]formulaSpan
	rows  map[int64]rowSpan
	// words are the source ranges of the tokens a math word's glyphs spell.
	words map[string]rowSpan
	// letters are the source ranges of each glyph of a math word whose glyphs the tokens spell one
	// by one.
	letters map[string][]rowSpan
	runs    map[string][]rune
}

// rowSpan is the source range of one row of a multi-row display (align, gather, ...).
type rowSpan struct {
	file          string
	line, endLine int64
	start, stop   int
}

// formulaSight is a shipped formula with the file and the lines TeX bracketed its glyphs with.
type formulaSight struct {
	formula       int64
	file          string
	before, after int64
}

type locationKey struct {
	file, line, endLine string
}

func newMathPlacement(texts *omap.Map[string, *source], sources []formulaSource, files map[string]string) *mathPlacement {
	m := &mathPlacement{texts: texts, files: files, spans: map[int64]formulaSpan{}, rows: map[int64]rowSpan{}, words: map[string]rowSpan{}, letters: map[string][]rowSpan{}, runs: map[string][]rune{}}
	groups := omap.New[locationKey, []formulaSource]()
	for _, entry := range sources {
		key := locationKey{rec.Inspect(entry.source["file"]), rec.Inspect(entry.source["line"]), rec.Inspect(entry.source["end_line"])}
		group, _ := groups.Get(key)
		groups.Set(key, append(group, entry))
	}
	groups.Each(func(_ locationKey, group []formulaSource) {
		location := group[0].source
		file := expandRoot(rec.ToS(location["file"]))
		for _, name := range texts.Keys() {
			if expandRoot(name) == file {
				formulas := make([]int64, len(group))
				for index, entry := range group {
					formulas[index] = entry.formula
				}
				m.assign(name, lineNumber(location["line"]), lineNumber(location["end_line"]), formulas)
				return
			}
		}
	})
	return m
}

// expandRoot is File.expand_path(path, '/').
func expandRoot(path string) string {
	return expandRootWith(path, fspath.Slash)
}

func expandRootWith(path string, slash func(string) string) string {
	slashed := slash(path)
	if strings.HasPrefix(slashed, "/") || windowsAbsPath(slashed) {
		return stdpath.Clean(slashed)
	}
	return stdpath.Clean("/" + slashed)
}

func windowsAbsPath(path string) bool {
	return len(path) >= 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) &&
		path[1] == ':' && path[2] == '/'
}

func lineNumber(value any) int64 {
	number, ok := value.(int64)
	if !ok {
		rec.Fail("comparison of Integer with %s failed", rec.Inspect(value))
	}
	return number
}

func (m *mathPlacement) assign(name string, line, endLine int64, formulas []int64) {
	entry, _ := m.texts.Get(name)
	exact := []*mathsource.Span{}
	for _, span := range entry.spans {
		if span.Line == line && span.EndLine == endLine {
			exact = append(exact, span)
		}
	}
	if len(exact) == 0 {
		var containing *mathsource.Span
		for _, span := range entry.spans {
			if span.Line <= line && span.EndLine >= endLine && (containing == nil || span.Stop-span.Start < containing.Stop-containing.Start) {
				containing = span
			}
		}
		if containing != nil {
			exact = append(exact, containing)
		}
	}
	for index, formula := range formulas {
		var span *mathsource.Span
		switch {
		case len(exact) == 1:
			span = exact[0]
		case index < len(exact):
			span = exact[index]
		}
		if span != nil {
			m.spans[formula] = formulaSpan{name, span}
		}
	}
}

// origin is the formula's origin, or nil when it cannot be placed.
func (m *mathPlacement) origin(formula int64, file string, before, after int64) rec.Map {
	m.resolve(formula, file, before, after)
	if row, ok := m.rows[formula]; ok {
		entry, _ := m.texts.Get(row.file)
		return spanOrigin(entry, m.files, row.file, int(row.line), int(row.endLine), row.start, row.stop, "math")
	}
	found := m.spans[formula]
	if found.span == nil {
		return nil
	}
	entry, _ := m.texts.Get(found.file)
	span := found.span
	return spanOrigin(entry, m.files, found.file, int(span.Line), int(span.EndLine), span.Start, span.Stop, "math")
}

func (m *mathPlacement) bracketed(file string, before, after int64) formulaSpan {
	low, high := min(before, after), max(before, after)
	if low == 0 {
		low = high
	}
	entry, _ := m.texts.Get(file)
	overlapping, within := []*mathsource.Span{}, []*mathsource.Span{}
	for _, span := range entry.spans {
		if span.Line <= high && span.EndLine >= low {
			overlapping = append(overlapping, span)
			if span.Line >= low && span.EndLine <= high {
				within = append(within, span)
			}
		}
	}
	// TeX brackets a formula's glyphs with the lines around it, which can reach into the display
	// before it; the one span that lies entirely inside the bracket is then the formula's.
	if len(overlapping) > 1 && len(within) == 1 {
		overlapping = within
	}
	if len(overlapping) != 1 {
		return formulaSpan{}
	}
	return formulaSpan{file, overlapping[0]}
}

// resolve places a formula that has no recorded location by the lines TeX bracketed it with.
func (m *mathPlacement) resolve(formula int64, file string, before, after int64) {
	if found, ok := m.spans[formula]; (!ok || found.span == nil) && file != "" && m.texts.Has(file) {
		m.spans[formula] = m.bracketed(file, before, after)
	}
}

// prepare gives each row of a multi-row display its own source range. Formulas that resolve
// to the same display span are its rows in reading order; when their number equals the
// number of source rows, formula i takes row i, otherwise the whole display stays the origin.
func (m *mathPlacement) prepare(sights []formulaSight) {
	order := []*mathsource.Span{}
	groups := map[*mathsource.Span][]int64{}
	seen := map[int64]bool{}
	for _, sight := range sights {
		if seen[sight.formula] {
			continue
		}
		seen[sight.formula] = true
		m.resolve(sight.formula, sight.file, sight.before, sight.after)
		found, ok := m.spans[sight.formula]
		if !ok || found.span == nil || found.span.Form != "environment" || len(mathsource.RowSpans(m.textOf(sight.file), found.span)) == 0 {
			continue
		}
		if _, known := groups[found.span]; !known {
			order = append(order, found.span)
		}
		groups[found.span] = append(groups[found.span], sight.formula)
	}
	for _, span := range order {
		formulas := groups[span]
		file := m.spans[formulas[0]].file
		rows := m.displayRows(file, span)
		if len(rows) != len(formulas) {
			continue
		}
		for index, formula := range formulas {
			m.rows[formula] = rows[index]
		}
	}
}

// displayRows are the source ranges of the rows of a display span, nil when it has none.
func (m *mathPlacement) displayRows(file string, span *mathsource.Span) []rowSpan {
	entry, ok := m.texts.Get(file)
	if !ok {
		return nil
	}
	ranges := mathsource.RowSpans(entry.text, span)
	rows := make([]rowSpan, len(ranges))
	for index, r := range ranges {
		rows[index] = rowSpan{file: file, line: int64(entry.visible.LineOf(r[0])), endLine: int64(entry.visible.LineOf(r[1] - 1)), start: r[0], stop: r[1]}
	}
	return rows
}

// textOf is a file's text, "" for an unknown file.
func (m *mathPlacement) textOf(file string) string {
	if entry, ok := m.texts.Get(file); ok {
		return entry.text
	}
	return ""
}

// bodyOf is the source range of the formula's math: its row of a display, else its span's body.
func (m *mathPlacement) bodyOf(formula int64) (file string, start, stop int, ok bool) {
	if row, found := m.rows[formula]; found {
		return row.file, row.start, row.stop, true
	}
	found, ok := m.spans[formula]
	if !ok || found.span == nil {
		return "", 0, 0, false
	}
	entry, has := m.texts.Get(found.file)
	if !has {
		return "", 0, 0, false
	}
	start, stop, ok = mathsource.BodyRange(entry.text, found.span)
	return found.file, start, stop, ok
}

type bodyKey struct {
	file        string
	start, stop int
}

// alignWords gives each math word the source range of the tokens its glyphs spell. The words
// of formulas that share one body are aligned together, in reading order, so that a token is
// claimed once; a word no tokens spell keeps the formula's (or row's) range.
func (m *mathPlacement) alignWords(words []*word) {
	order := []bodyKey{}
	groups := map[bodyKey][]*word{}
	for _, w := range words {
		formula := w.formula()
		if formula <= 0 {
			continue
		}
		file, start, stop, ok := m.bodyOf(formula)
		if !ok {
			continue
		}
		key := bodyKey{file, start, stop}
		if _, known := groups[key]; !known {
			order = append(order, key)
		}
		groups[key] = append(groups[key], w)
	}
	for _, key := range order {
		entry, _ := m.texts.Get(key.file)
		if _, cached := m.runs[key.file]; !cached {
			m.runs[key.file] = []rune(entry.text)
		}
		tokens := mathtokens.Tokenize(m.runs[key.file], key.start, key.stop)
		clusters := make([]string, len(groups[key]))
		for index, w := range groups[key] {
			clusters[index] = w.text()
		}
		for index, match := range mathtokens.Align(clusters, tokens) {
			if match.OK {
				m.words[groups[key][index].id] = rowSpan{file: key.file, line: int64(entry.visible.LineOf(match.Start)),
					endLine: int64(entry.visible.LineOf(match.Stop - 1)), start: match.Start, stop: match.Stop}
				m.splitLetters(groups[key][index], entry, key.file, tokens, match)
			}
		}
	}
}

// wordOrigin is the origin of a math word whose glyphs spell tokens of its formula, or nil.
func (m *mathPlacement) wordOrigin(id string) rec.Map {
	found, ok := m.words[id]
	if !ok {
		return nil
	}
	entry, _ := m.texts.Get(found.file)
	return spanOrigin(entry, m.files, found.file, int(found.line), int(found.endLine), found.start, found.stop, "math")
}

// splitLetters records the range of each glyph of a word the tokens spell glyph by glyph.
func (m *mathPlacement) splitLetters(w *word, entry *source, file string, tokens []mathtokens.Token, whole mathtokens.Match) {
	if len(w.glyphs) < 2 {
		return
	}
	glyphs := make([]string, len(w.glyphs))
	for index, g := range w.glyphs {
		glyphs[index] = g.text
	}
	parts, ok := mathtokens.Split(glyphs, tokens, whole.Start, whole.Stop)
	if !ok {
		return
	}
	spans := make([]rowSpan, len(parts))
	for index, part := range parts {
		spans[index] = rowSpan{file: file, line: int64(entry.visible.LineOf(part.Start)),
			endLine: int64(entry.visible.LineOf(part.Stop - 1)), start: part.Start, stop: part.Stop}
	}
	m.letters[w.id] = spans
}

// letterOrigins are the origins of a word's glyphs, one per glyph, or nil.
func (m *mathPlacement) letterOrigins(id string) []rec.Map {
	spans, ok := m.letters[id]
	if !ok {
		return nil
	}
	origins := make([]rec.Map, len(spans))
	for index, found := range spans {
		entry, _ := m.texts.Get(found.file)
		origins[index] = spanOrigin(entry, m.files, found.file, int(found.line), int(found.endLine), found.start, found.stop, "math")
	}
	return origins
}
