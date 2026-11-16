package typstsource

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/rec"
)

// Headings of the source-faithful writer (TypstSourceHeadings): which blocks are headings,
// their levels and numbers, and how references print them.

// probe is TypstSourceHeadings::PROBE: verification only (typst eval --input ltd-probe=1):
// each label's rendered number, as a reference prints it.
const probe = `#let ltd-probe(name) = if sys.inputs.at("ltd-probe", default: none) != none {
  context {
    let el = query(label(name)).first()
    let shown = if el.func() == heading and el.numbering != none { ltd-heading-ref(el) } else {
      let ctr = if el.func() == math.equation { counter(math.equation) } else if el.func() == figure { counter(figure.where(kind: el.kind)) } else if el.func() == footnote { counter(footnote) } else { none }
      if ctr == none or el.numbering == none { none } else { numbering(el.numbering, ..ctr.at(el.location())) }
    }
    [#metadata((label: name, number: shown))<ltd-number>]
  }
}`

var (
	texSpace      = regexp.MustCompile(`\\[,;: ]|~|\x{00A0}`)
	appendixStart = regexp.MustCompile(`\A[ \t\n\v\f\r]*Appendix[ \t\n\v\f\r]`)
	hasDigit      = regexp.MustCompile(`[0-9]`)
	labelParts    = regexp.MustCompile(`[. ]`)
	trailingStop  = regexp.MustCompile(`\\?\.\z`)
)

// headMatch is a match of HEAD_NUMBER: \A\s*(?:Appendix\s+)?(NUMERAL(?:\.NUMERAL)*)([.:])?
// (?=\s|\p{Lu}|\z)\s*, with NUMERAL (?:\d+|[IVXLC]+|[A-Za-z]). Offsets are characters.
type headMatch struct {
	end       int
	number    string
	separator string
	hasSep    bool
}

// matchHead runs the pattern with Ruby's backtracking order; levels bounds the numeral
// chain's parts (0: unbounded).
func matchHead(text string, levels int) (headMatch, bool) {
	runes := []rune(text)
	start := leadingSpace(runes)
	starts := []int{}
	if strings.HasPrefix(string(runes[start:]), "Appendix") {
		after := start + len("Appendix")
		spaces := after
		for spaces < len(runes) && isSpace(runes[spaces]) {
			spaces++
		}
		if spaces > after {
			starts = append(starts, spaces)
		}
	}
	starts = append(starts, start)
	for _, chainStart := range starts {
		if match, ok := matchChain(runes, chainStart, levels); ok {
			return match, true
		}
	}
	return headMatch{}, false
}

func matchChain(runes []rune, start, levels int) (headMatch, bool) {
	for _, end := range chainEnds(runes, start, 1, levels) {
		separators := []int{}
		if end < len(runes) && (runes[end] == '.' || runes[end] == ':') {
			separators = append(separators, end+1)
		}
		separators = append(separators, end)
		for _, after := range separators {
			if after < len(runes) && !isSpace(runes[after]) && !unicode.IsUpper(runes[after]) {
				continue
			}
			final := after
			for final < len(runes) && isSpace(runes[final]) {
				final++
			}
			match := headMatch{end: final, number: string(runes[start:end]), hasSep: after > end}
			if match.hasSep {
				match.separator = string(runes[end])
			}
			return match, true
		}
	}
	return headMatch{}, false
}

// chainEnds: the ends of NUMERAL(?:\.NUMERAL)* from start, in preference order.
func chainEnds(runes []rune, start, parts, levels int) []int {
	ends := []int{}
	for _, end := range numeralEnds(runes, start) {
		if (levels == 0 || parts < levels) && end < len(runes) && runes[end] == '.' {
			ends = append(ends, chainEnds(runes, end+1, parts+1, levels)...)
		}
		ends = append(ends, end)
	}
	return ends
}

// numeralEnds: the ends of NUMERAL at start, in preference order (\d+, then [IVXLC]+, then
// one letter).
func numeralEnds(runes []rune, start int) []int {
	ends := []int{}
	run := func(test func(rune) bool) {
		end := start
		for end < len(runes) && test(runes[end]) {
			end++
		}
		for ; end > start; end-- {
			ends = append(ends, end)
		}
	}
	run(isDigit)
	run(func(char rune) bool { return strings.ContainsRune("IVXLC", char) })
	if start < len(runes) && (runes[start] >= 'A' && runes[start] <= 'Z' || runes[start] >= 'a' && runes[start] <= 'z') {
		ends = append(ends, start+1)
	}
	return ends
}

// headingInfo is a heading's level, printed number and where its title starts.
type headingInfo struct {
	level    int
	number   *string
	printed  *string
	prefix   string
	skip     int
	titleEnd int
	runin    bool
}

// planHeadings is TypstSourceHeadings#plan_headings: every heading in reading order with its
// level, printed number and where its title starts.
func (e *exporter) planHeadings() map[string]headingDirective {
	e.headingInfo = map[string]*headingInfo{}
	order := []string{}
	e.walk(rec.FetchString(e.document, "root"), func(node rec.Map) {
		if info := e.headingOf(node); info != nil {
			id := toS(node["id"])
			e.headingInfo[id] = info
			order = append(order, id)
		}
	})
	entries := make([]headingEntry, len(order))
	for index, id := range order {
		info := e.headingInfo[id]
		entries[index] = headingEntry{level: info.level, printed: info.printed, prefix: info.prefix}
	}
	plan := planHeadings(entries)
	result := map[string]headingDirective{}
	for index, id := range order {
		result[id] = plan[index]
	}
	return result
}

func (e *exporter) walk(id string, visit func(rec.Map)) {
	node := e.node(id)
	visit(node)
	for _, child := range children(node) {
		e.walk(child, visit)
	}
}

func intOf(value any) (int, bool) {
	number, ok := rec.Number(value)
	return int(number), ok
}

func (e *exporter) headingOf(node rec.Map) *headingInfo {
	runin := truthy(dig(node, "semantics", "runin")) || e.strayHeading(node)
	if !e.headingBlock(node, runin) {
		return nil
	}
	text := marked(node)
	levels := 0
	if level, ok := intOf(dig(node, "semantics", "level")); ok {
		levels = level
	}
	match, matched := matchHead(text, levels)
	if levels < 0 {
		matched = false
	}
	var number *string
	if matched {
		number = e.headingNumber(node, match)
	}
	info := &headingInfo{level: e.headingLevel(node, number), number: number, runin: runin}
	info.skip = size(lead(text))
	if number != nil {
		printed := *number + match.separator
		info.printed = &printed
		info.skip = match.end
		if appendixStart.MatchString(string([]rune(text)[:match.end])) {
			info.prefix = "Appendix "
		}
	}
	info.titleEnd = size(text)
	if runin {
		info.titleEnd = e.runinTitleEnd(node, text, info.skip)
	}
	return info
}

// headingBlock: lines before the first section the reading typed as headings (affiliations)
// are front matter. When the collector recorded the paper's sectioning commands, only those
// are headings; a class that prints its title block inside the first section's context
// (AASTeX) leaves several heading blocks on one heading, of which the last is the heading.
func (e *exporter) headingBlock(node rec.Map, runin bool) bool {
	if node["type"] != "heading" {
		return node["type"] == "paragraph" && runin
	}
	id := toS(node["id"])
	for _, front := range e.frontBlocks {
		if front == id {
			return false
		}
	}
	if !e.sectioningKnown() {
		return true
	}
	if !e.headingSemantics(node) {
		return false
	}
	entity := dig(node, "semantics", "entity")
	return entity == nil || e.lastHeadingBlock(entity) == id
}

func (e *exporter) sectioningKnown() bool {
	if e.sectioning == nil {
		known := false
		for _, id := range e.sortedIDs() {
			known = known || dig(e.node(id), "semantics", "kind") == "heading"
		}
		e.sectioning = &known
	}
	return *e.sectioning
}

func (e *exporter) lastHeadingBlock(entity any) string {
	if e.lastHeadings == nil {
		e.lastHeadings = map[string]string{}
		for _, id := range stringList(e.document["block_order"]) {
			node := e.node(id)
			if found := dig(node, "semantics", "entity"); node["type"] == "heading" && truthy(found) {
				e.lastHeadings[toS(found)] = id
			}
		}
	}
	return e.lastHeadings[toS(entity)]
}

func (e *exporter) parentOf(node rec.Map) rec.Map {
	parent, ok := e.parents[toS(node["id"])]
	if !ok {
		return nil
	}
	found, _ := e.nodes[parent].(rec.Map)
	return found
}

func (e *exporter) headingSemantics(node rec.Map) bool {
	return dig(node, "semantics", "kind") == "heading" || dig(e.parentOf(node), "semantics", "kind") == "heading"
}

// strayHeading: amsart sets a run-in heading followed by a theorem inside the theorem's first
// paragraph: a short, partly bold, numbered paragraph ahead of the theorem's head.
func (e *exporter) strayHeading(node rec.Map) bool {
	parent := e.parentOf(node)
	if node["type"] != "paragraph" || parent == nil || rec.Fetch(parent, "type") != "theorem" {
		return false
	}
	first := children(parent)
	if len(first) == 0 || first[0] != toS(node["id"]) {
		return false
	}
	if _, ok := e.theoremHead(parent, node); ok {
		return false
	}
	bold := false
	for _, span := range content(node) {
		if span["type"] != "text" {
			return false
		}
		bold = bold || strings.HasPrefix(toS(dig(span, "style", "series")), "b")
	}
	text := plain(node)
	_, numbered := matchHead(text, 0)
	return bold && size(text) < 120 && numbered
}

// headingNumber: a leading numeral is the heading's number when it has a digit, a dot after
// it, or a label that prints it ("A Note" is a title, "A. Proofs" a numbered appendix).
func (e *exporter) headingNumber(node rec.Map, match headMatch) *string {
	candidate := match.number
	if _, ok := parseNumber(candidate); !ok {
		return nil
	}
	labelled := false
	for _, name := range e.labelsByTarget[toS(node["id"])] {
		labelled = labelled || e.labelNumber(name) == candidate
	}
	if hasDigit.MatchString(candidate) || match.hasSep || labelled {
		return &candidate
	}
	return nil
}

func (e *exporter) headingLevel(node rec.Map, number *string) int {
	if level, ok := intOf(dig(node, "semantics", "level")); ok {
		return level
	}
	if level, ok := intOf(dig(e.parentOf(node), "semantics", "level")); ok {
		return level
	}
	if number != nil {
		return len(rubySplit(*number, "."))
	}
	return 1
}

// runinTitleEnd: a run-in heading's title ends where its bold run ends, or at its first full
// stop.
func (e *exporter) runinTitleEnd(node rec.Map, text string, skip int) int {
	bold := 0
	for _, span := range content(node) {
		if span["type"] != "text" || !strings.HasPrefix(toS(dig(span, "style", "series")), "b") {
			break
		}
		bold += size(rec.FetchString(span, "text"))
	}
	if bold > skip {
		return bold
	}
	runes := []rune(text)
	for at := skip; at < len(runes); at++ {
		if runes[at] == '.' && (at+1 == len(runes) || isSpace(runes[at+1])) {
			return at + 1
		}
	}
	return len(runes)
}

func (e *exporter) heading(node rec.Map) string {
	id := toS(node["id"])
	info := e.headingInfo[id]
	directive := e.headingPlan[id]
	lines := e.headingDirectives(directive)
	if info.level == 1 {
		e.sectionReset(info)
	}
	label := e.ownLabel(node)
	if label != "" && info.number != nil {
		e.recordHeadingReference(info, label)
	}
	lines = append(lines, e.headingLine(info, directive, e.headingTitle(node, info), label))
	// The heading's other labels (\label{sec:a}\label{sec:b}) follow it as link targets.
	if anchors := e.anchors(node); anchors != "" {
		lines = append(lines, anchors)
	}
	if info.runin {
		if rest := rec.Strip(e.inline(content(node), node, info.titleEnd, false)); rest != "" {
			lines = append(lines, protectStart(rest))
		}
	}
	return strings.Join(lines, "\n")
}

func (e *exporter) headingDirectives(directive headingDirective) []string {
	lines := []string{}
	if directive.numbering != "" {
		lines = append(lines, e.scoped("#set heading(numbering: "+directive.numbering+")"))
		segment := "main"
		if directive.hasSegment {
			segment = directive.segment
		}
		e.segment = &segment
		lines = append(lines, "#ltd-segment.update("+typstString(segment)+")")
	}
	if directive.update != nil {
		values := make([]string, len(directive.update))
		for index, item := range directive.update {
			values[index] = item.String()
		}
		lines = append(lines, "#counter(heading).update(("+strings.Join(values, ", ")+",))")
	}
	return lines
}

// headingTitle: the class's heading style (bold, small caps) and printed full stop are not
// the author's.
func (e *exporter) headingTitle(node rec.Map, info *headingInfo) string {
	spans := takeText(dropText(content(node), info.skip), info.titleEnd-info.skip)
	unstyled := make([]rec.Map, len(spans))
	for index, span := range spans {
		copied := rec.Map{}
		for key, value := range span {
			if key != "style" {
				copied[key] = value
			}
		}
		unstyled[index] = copied
	}
	return trailingStop.ReplaceAllString(rec.Strip(e.inline(unstyled, node, 0, false)), "")
}

func (e *exporter) headingLine(info *headingInfo, directive headingDirective, title, label string) string {
	if directive.unnumbered && e.headingNumberingActive() {
		return fmt.Sprintf("#heading(level: %d, numbering: none)[%s]%s", info.level, title, e.labelSuffix(label, false, nil))
	}
	return strings.Repeat("=", info.level) + " " + title + e.labelSuffix(label, e.headingLabel(label, info), nil)
}

// headingLabel: a heading's \ref may print more than the heading (REVTeX's II A for A.), but
// its last numeral is the heading's own; a label that ends otherwise belongs to something
// else.
func (e *exporter) headingLabel(label string, info *headingInfo) bool {
	if label == "" || info.number == nil {
		return true
	}
	latex := texSpace.ReplaceAllString(toS(e.labelNumber(label)), " ")
	parts := []string{}
	for _, part := range labelParts.Split(latex, -1) {
		if part != "" {
			parts = append(parts, part)
		}
	}
	own := rubySplit(*info.number, ".")
	if len(parts) == 0 || len(own) == 0 {
		return len(parts) == 0 && len(own) == 0
	}
	return parts[len(parts)-1] == own[len(own)-1]
}

// recordHeadingReference: the \ref form of a labelled heading, per level and segment: a
// leading word (elsarticle's "Appendix A"), numerals and separators.
func (e *exporter) recordHeadingReference(info *headingInfo, label string) {
	latex := rec.Strip(squeeze(texSpace.ReplaceAllString(toS(e.labelNumber(label)), " "), ' '))
	word := referenceWord(latex)
	tokens := splitKeeping(strings.TrimPrefix(latex, word))
	parts := []formatPart{}
	unknown := false
	for index := 0; index < len(tokens); index += 2 {
		found, ok := parseComponent(tokens[index])
		unknown = unknown || !ok
		part := formatPart{kind: found.kind}
		if index+1 < len(tokens) {
			separator := tokens[index+1]
			part.separator = &separator
		}
		parts = append(parts, part)
	}
	e.labelNumbers[label] = latex
	if latex == "" {
		e.labelNumbers[label] = *info.number
	}
	if unknown || len(parts) > info.level {
		return
	}
	segment := "main"
	if e.segment != nil {
		segment = *e.segment
	}
	key := headingKey{len(parts), segment}
	if _, ok := e.headingFormats[key]; !ok {
		e.headingFormats[key] = headingFormat{prefix: word, parts: parts}
	}
}

// referenceWord is latex[/\A((?![IVXLC]+\s)\p{L}{2,}\s+)(?=\S)/, 1].
func referenceWord(latex string) string {
	runes := []rune(latex)
	romans := 0
	for romans < len(runes) && strings.ContainsRune("IVXLC", runes[romans]) {
		romans++
	}
	if romans > 0 && romans < len(runes) && isSpace(runes[romans]) {
		return ""
	}
	letters := 0
	for letters < len(runes) && unicode.IsLetter(runes[letters]) {
		letters++
	}
	spaces := letters
	for spaces < len(runes) && isSpace(runes[spaces]) {
		spaces++
	}
	if letters < 2 || spaces == letters || spaces == len(runes) {
		return ""
	}
	return string(runes[:spaces])
}

// splitKeeping is String#split(/([. ])/): the separators kept, trailing empty fields dropped.
func splitKeeping(text string) []string {
	tokens := []string{}
	current := []rune{}
	for _, char := range text {
		if char == '.' || char == ' ' {
			tokens = append(tokens, string(current), string(char))
			current = []rune{}
			continue
		}
		current = append(current, char)
	}
	tokens = append(tokens, string(current))
	for len(tokens) > 0 && tokens[len(tokens)-1] == "" {
		tokens = tokens[:len(tokens)-1]
	}
	return tokens
}

func (e *exporter) headingNumberingActive() bool {
	for _, directive := range e.headingPlan {
		if directive.numbering != "" {
			return true
		}
	}
	return false
}

// sectionReset: a new top-level section: section-relative series start again.
func (e *exporter) sectionReset(info *headingInfo) {
	e.currentSection = info.number
	e.equations.setSection(info.number)
	for _, kind := range e.seriesOrder {
		e.series[kind].setSection(info.number)
	}
}

// headingReferences: how a reference prints a heading's number: LaTeX's \ref form, else the
// heading's own number without its prefix and trailing dot or colon.
func (e *exporter) headingReferences() string {
	keys := make([]headingKey, 0, len(e.headingFormats))
	for key := range e.headingFormats {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].segment != keys[b].segment {
			return keys[a].segment < keys[b].segment
		}
		return keys[a].level < keys[b].level
	})
	cases := []string{}
	for _, key := range keys {
		format := e.headingFormats[key]
		cases = append(cases, fmt.Sprintf("if n.len() == %d and segment == %s { %s }", key.level, typstString(key.segment),
			strings.Join(referencePieces(format.prefix, format.parts), " + ")))
	}
	cases = append(cases, `{ let shown = numbering(el.numbering, ..n); `+
		`if type(shown) == str { shown.trim(regex("[.:]"), at: end).trim("Appendix ", at: start) } else { shown } }`)
	return "#let ltd-heading-ref(el) = { let n = counter(heading).at(el.location()); let segment = ltd-segment.at(el.location()); " +
		strings.Join(cases, " else ") + " }\n" +
		"#show ref: it => if it.element != none and it.element.func() == heading and it.element.numbering != none {\n" +
		"  link(it.element.location(), if it.supplement in (none, auto) { ltd-heading-ref(it.element) } else [#it.supplement #ltd-heading-ref(it.element)])\n" +
		"} else { it }"
}

func referencePieces(word string, parts []formatPart) []string {
	pieces := []string{}
	if word != "" {
		pieces = append(pieces, typstString(word))
	}
	for index, part := range parts {
		numeral := fmt.Sprintf("numbering(%s, n.at(%d))", typstString(patterns[part.kind]), index)
		if part.separator != nil {
			numeral += " + " + typstString(*part.separator)
		}
		pieces = append(pieces, numeral)
	}
	return pieces
}
