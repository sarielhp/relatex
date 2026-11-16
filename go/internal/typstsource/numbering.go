package typstsource

import (
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Numbering (TypstSourceNumbering) reproduces the numbers LaTeX printed. Each series
// (headings, equations, a theorem counter, figures, tables) follows Typst's own counter and
// numbering pattern; where the counter would differ from the printed number the writer
// updates it, and a number no pattern produces (1a, *) is set on its element.

type numeralKind string

const (
	arabic     numeralKind = "arabic"
	upper      numeralKind = "upper"
	lowerAlpha numeralKind = "lower"
	roman      numeralKind = "roman"
	lowerRoman numeralKind = "lower_roman"
)

// patterns is TypstSourceNumbering::PATTERN.
var patterns = map[numeralKind]string{arabic: "1", upper: "A", lowerAlpha: "a", roman: "I", lowerRoman: "i"}

// romanSymbols and romanAmounts are the roman numerals' symbols, largest first.
var (
	romanSymbols = []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	romanAmounts = []int64{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
)

// value is a component's value; Ruby's nil (an invalid roman numeral) is valid == false.
type value struct {
	number *big.Int
	valid  bool
}

func intValue(number int64) value { return value{number: big.NewInt(number), valid: true} }

func (v value) equal(other value) bool {
	if !v.valid || !other.valid {
		return v.valid == other.valid
	}
	return v.number.Cmp(other.number) == 0
}

func (v value) String() string {
	if !v.valid {
		return ""
	}
	return v.number.String()
}

func (v value) plus(amount int64) value {
	if !v.valid {
		panic(fmt.Errorf("undefined method '+' for nil"))
	}
	return value{number: new(big.Int).Add(v.number, big.NewInt(amount)), valid: true}
}

// component is one printed component: its value and the kind of numeral.
type component struct {
	value value
	kind  numeralKind
}

var (
	digitsPattern     = regexp.MustCompile(`\A[0-9]+\z`)
	romanPattern      = regexp.MustCompile(`\A[IVXLC]+\z`)
	lowerRomanPattern = regexp.MustCompile(`\A[ivxl]+\z`)
)

// parseComponent is TypstSourceNumbering.component.
func parseComponent(text string) (component, bool) {
	switch {
	case digitsPattern.MatchString(text):
		number, _ := new(big.Int).SetString(text, 10)
		return component{value: value{number: number, valid: true}, kind: arabic}, true
	case (romanPattern.MatchString(text) && len(text) > 1) || text == "I":
		return component{value: romanValue(text), kind: roman}, true
	case lowerRomanPattern.MatchString(text) && len(text) > 1:
		return component{value: romanValue(strings.ToUpper(text)), kind: lowerRoman}, true
	case len(text) == 1 && text[0] >= 'A' && text[0] <= 'Z':
		return component{value: intValue(int64(text[0]) - 64), kind: upper}, true
	case len(text) == 1 && text[0] >= 'a' && text[0] <= 'z':
		return component{value: intValue(int64(text[0]) - 96), kind: lowerAlpha}, true
	}
	return component{}, false
}

// romanValue is TypstSourceNumbering.roman: nil (invalid) when symbols remain.
func romanValue(text string) value {
	total := int64(0)
	rest := text
	for index, symbol := range romanSymbols {
		for strings.HasPrefix(rest, symbol) {
			total += romanAmounts[index]
			rest = strings.TrimPrefix(rest, symbol)
		}
	}
	if rest != "" {
		return value{}
	}
	return intValue(total)
}

// stripRuby is String#strip.
func stripRuby(text string) string { return rec.Strip(text) }

// rubySplit is String#split(separator): trailing empty fields are dropped.
func rubySplit(text, separator string) []string {
	parts := strings.Split(text, separator)
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// parseNumber is TypstSourceNumbering.parse: "2.3" or "A.1" as components; false when any
// part is no numeral (1a, *).
func parseNumber(text string) ([]component, bool) {
	parts := rubySplit(stripRuby(text), ".")
	if len(parts) == 0 || slices.Contains(parts, "") {
		return nil, false
	}
	components := []component{}
	for _, part := range parts {
		found, ok := parseComponent(part)
		if !ok {
			return nil, false
		}
		components = append(components, found)
	}
	return components, true
}

func numberPattern(components []component) string {
	parts := make([]string, len(components))
	for index, part := range components {
		parts[index] = patterns[part.kind]
	}
	return strings.Join(parts, ".")
}

// mode is a series' numbering mode: [:plain, pattern] or [:sectioned, "A|1"].
type mode struct {
	kind    string
	pattern string
}

// directive is what the next element of a series needs.
type seriesDirective struct {
	mode     *mode
	update   *value
	override *string
}

// series is TypstSourceNumbering::Series: a numbered series of one kind of element. wrap is
// the printed form around the number ("(%s)" for equations); section the printed number of
// the enclosing top-level section.
type series struct {
	wrap    string
	value   value
	mode    *mode
	section *string
	printed []string
}

func newSeries(wrap string) *series { return &series{wrap: wrap, value: intValue(0)} }

// setSection is Series#section!: a new top-level section; a section-relative series starts
// again.
func (s *series) setSection(printed *string) {
	s.section = printed
	if s.mode != nil && s.mode.kind == "sectioned" {
		s.value = intValue(0)
	}
}

// seen is Series#seen?: a number this series printed before (a restated theorem).
func (s *series) seen(printed string) bool { return slices.Contains(s.printed, stripRuby(printed)) }

// next is Series#next: the directives for the next element printed with printed.
func (s *series) next(printed string) seriesDirective {
	s.printed = append(s.printed, stripRuby(printed))
	inner, ok := s.unwrap(printed)
	var found *mode
	if ok {
		found = s.modeFor(inner)
	}
	s.value = s.value.plus(1)
	if found == nil {
		override := printed
		return seriesDirective{override: &override}
	}
	result := seriesDirective{}
	if s.mode == nil || *found != *s.mode {
		if found.kind == "sectioned" && (s.mode == nil || s.mode.kind != "sectioned") {
			s.value = intValue(1)
		}
		result.mode = found
		s.mode = found
	}
	expected := inner[len(inner)-1].value
	if !expected.equal(s.value) {
		update := expected.plus(-1)
		result.update = &update
		s.value = expected
	}
	return result
}

// numbering is Series#numbering: a pattern string, or a function of the section counter.
func (s *series) numbering(current mode) string {
	wrapped := strings.Replace(s.wrap, "%s", current.pattern, 1)
	if current.kind == "plain" {
		return typstString(wrapped)
	}
	sectionPattern, own, _ := strings.Cut(current.pattern, "|")
	before, after, _ := strings.Cut(s.wrap, "%s")
	parts := []string{}
	if before != "" {
		parts = append(parts, typstString(before))
	}
	parts = append(parts, "numbering("+typstString(sectionPattern)+", counter(heading).get().first())", `"."`,
		"numbering("+typstString(own)+", n)")
	if after != "" {
		parts = append(parts, typstString(after))
	}
	return "n => " + strings.Join(parts, " + ")
}

func (s *series) unwrap(printed string) ([]component, bool) {
	before, after, _ := strings.Cut(s.wrap, "%s")
	text := stripRuby(printed)
	if !strings.HasPrefix(text, before) || !strings.HasSuffix(text, after) || size(text) <= size(before)+size(after) {
		return nil, false
	}
	runes := []rune(text)
	return parseNumber(string(runes[size(before) : len(runes)-size(after)]))
}

func (s *series) modeFor(components []component) *mode {
	if len(components) == 1 {
		return &mode{kind: "plain", pattern: numberPattern(components)}
	}
	if len(components) != 2 || s.section == nil || !components[0].value.equal(s.sectionValue()) {
		return nil
	}
	return &mode{kind: "sectioned", pattern: patterns[components[0].kind] + "|" + patterns[components[1].kind]}
}

// sectionValue is Series#section_value (an invalid value: Ruby's nil).
func (s *series) sectionValue() value {
	parsed, ok := parseNumber(*s.section)
	if !ok || len(parsed) != 1 {
		return value{}
	}
	return parsed[0].value
}

// headingEntry is one heading the plan covers: level, printed number, prefix.
type headingEntry struct {
	level   int
	printed *string
	prefix  string
}

// headingDirective is a heading's plan: unnumbered, or a numbering at a segment's start and
// a counter update.
type headingDirective struct {
	unnumbered bool
	numbering  string
	segment    string
	hasSegment bool
	update     []value
}

type parsedHeading struct {
	level      int
	components []component
	dot        string
	prefix     string
}

var trailingDot = regexp.MustCompile(`[.:]\z`)

// planHeadings is TypstSourceNumbering::Headings.plan: one counter with a component per
// level. A heading prints its full number (1.2), or only its own level's numeral (REVTeX's
// II, A). One numbering per segment (the main matter, an appendix in letters), counter
// updates where Typst's count would differ from the printed number.
func planHeadings(headings []headingEntry) []headingDirective {
	var top *component
	parsed := make([]*parsedHeading, len(headings))
	entries := []parsedHeading{}
	for index, heading := range headings {
		if heading.printed == nil {
			continue
		}
		components, ok := parseNumber(trailingDot.ReplaceAllString(*heading.printed, ""))
		if !ok {
			continue
		}
		components = romanContinued(components, top)
		if heading.level == 1 {
			first := components[0]
			top = &first
		}
		entry := parsedHeading{heading.level, components, trailingDot.FindString(*heading.printed), heading.prefix}
		parsed[index] = &entry
		entries = append(entries, entry)
	}
	segments := segmentFormats(entries)
	state := []value{}
	segment := numeralKind("")
	result := make([]headingDirective, len(parsed))
	for index, entry := range parsed {
		if entry == nil {
			result[index] = headingDirective{unnumbered: true}
			continue
		}
		directive := headingDirective{}
		if startsSegment(*entry, segment) {
			if key := entry.components[0].kind; key != segment {
				segment = key
				directive.numbering = segments[segment].numbering()
				directive.segment, directive.hasSegment = string(segment), true
			}
		}
		state = stepState(state, entry.level)
		expected := expectedState(state, entry)
		if !sameValues(expected, state) {
			update := append(slices.Clone(expected[:len(expected)-1]), expected[len(expected)-1].plus(-1))
			directive.update = update
			state = expected
		}
		result[index] = directive
	}
	return result
}

// startsSegment: a heading may start a segment (the main matter, an appendix in letters): a
// top-level one, the first, or one deeper that prints its whole chain in another kind. A book's
// appendix chapter set as a paragraph ("Appendix A") above an unnumbered title leaves its
// sections (A.1) to start the segment.
func startsSegment(entry parsedHeading, segment numeralKind) bool {
	if entry.level == 1 || segment == "" {
		return true
	}
	return len(entry.components) == entry.level && entry.components[0].kind != segment
}

// stepState: Typst steps a level: deeper levels go, missing ones are 0, this one counts up.
func stepState(state []value, level int) []value {
	if len(state) > level {
		state = state[:level]
	}
	state = slices.Clone(state)
	for len(state) < level {
		state = append(state, intValue(0))
	}
	state[level-1] = state[level-1].plus(1)
	return state
}

func expectedState(state []value, entry *parsedHeading) []value {
	shown := make([]value, len(entry.components))
	for index, part := range entry.components {
		shown[index] = part.value
	}
	if len(shown) >= entry.level {
		return shown[len(shown)-entry.level:]
	}
	return append(slices.Clone(state[:entry.level-len(shown)]), shown...)
}

func sameValues(left, right []value) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !left[index].equal(right[index]) {
			return false
		}
	}
	return true
}

// romanContinued: a lone V, X, L or C after roman section IV, IX, ... is the next roman
// numeral, not a letter.
func romanContinued(components []component, top *component) []component {
	first := components[0]
	if top == nil || top.kind != roman || first.kind != upper {
		return components
	}
	letter := string(rune(first.value.number.Int64() + 64))
	next := value{}
	if slices.Contains([]string{"V", "X", "L", "C"}, letter) {
		next = romanValue(letter)
	}
	if !next.equal(top.value.plus(1)) || !next.valid {
		return components
	}
	return append([]component{{value: next, kind: roman}}, components[1:]...)
}

// segmentFormat is one segment's format: level => kinds shown, the trailing dot or colon, and
// each level's prefix.
type segmentFormat struct {
	levels   map[int][]numeralKind
	order    []int
	dot      string
	prefixes map[int]string
}

func segmentFormats(entries []parsedHeading) map[numeralKind]*segmentFormat {
	formats := map[numeralKind]*segmentFormat{}
	segment := numeralKind("")
	for _, entry := range entries {
		if startsSegment(entry, segment) {
			segment = entry.components[0].kind
		}
		format, ok := formats[segment]
		if !ok {
			format = &segmentFormat{levels: map[int][]numeralKind{}, dot: entry.dot, prefixes: map[int]string{}}
			formats[segment] = format
		}
		if _, known := format.levels[entry.level]; !known {
			kinds := make([]numeralKind, len(entry.components))
			for index, part := range entry.components {
				kinds[index] = part.kind
			}
			format.levels[entry.level] = kinds
			format.order = append(format.order, entry.level)
		}
		if _, known := format.prefixes[entry.level]; !known {
			format.prefixes[entry.level] = entry.prefix
		}
	}
	return formats
}

// numbering is Headings.numbering: a pattern string when every level shows its whole chain
// (1.2.1), else a function that shows each level's own numerals.
func (f *segmentFormat) numbering() string {
	deepest := slices.Max(f.order)
	chain := true
	for _, prefix := range f.prefixes {
		chain = chain && prefix == ""
	}
	for level, kinds := range f.levels {
		deep := f.levels[deepest]
		chain = chain && len(kinds) == level && level <= len(deep) && slices.Equal(kinds, deep[:level])
	}
	if chain {
		return typstString(kindPattern(f.levels[deepest]) + f.dot)
	}
	levels := slices.Sorted(slices.Values(f.order))
	cases := make([]string, len(levels))
	for index, level := range levels {
		kinds := f.levels[level]
		prefix := ""
		if f.prefixes[level] != "" {
			prefix = typstString(f.prefixes[level]) + " + "
		}
		cases[index] = fmt.Sprintf("if n.len() == %d { %snumbering(%s, ..n.slice(-%d)) }", level, prefix,
			typstString(kindPattern(kinds)+f.dot), len(kinds))
	}
	return "(..nums) => { let n = nums.pos(); " + strings.Join(cases, " else ") + ` else { numbering("1.", ..n) } }`
}

func kindPattern(kinds []numeralKind) string {
	parts := make([]string, len(kinds))
	for index, kind := range kinds {
		parts[index] = patterns[kind]
	}
	return strings.Join(parts, ".")
}
