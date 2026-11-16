package reading

import (
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tables"
)

// The label targets of ReadingLabelTargets: the blocks a label names when its target
// context holds no block of its own: captions, headings, formulas and pictures the reading
// finds around the label.

// rubySpace is Ruby's \s: ASCII whitespace with the vertical tab.
const rubySpace = `[ \t\n\v\f\r]`

var (
	sectionCounters = []string{"chapter", "section", "subsection", "subsubsection"}
	equationNumber  = regexp.MustCompile(`\A\d+[a-z]?\z`)
)

func (e *Evidence) entityBlocks(entity any) []rec.Map {
	return e.blocksWhere(func(block rec.Map) bool { return rec.Equal(rec.Fetch(block, "entity"), entity) })
}

func one(blocks []rec.Map) []rec.Map {
	if len(blocks) == 1 {
		return blocks
	}
	return []rec.Map{}
}

func textOf(block rec.Map, test func(rec.Map) bool) string {
	parts := []rec.Map{}
	for _, span := range content(block) {
		if test(span) {
			parts = append(parts, span)
		}
	}
	return joinedText(parts)
}

// boxedCaptionBlocks: a caption typeset in a box that flows with its figure (wrapfig) is
// read in the figure's own paragraph context, where the label also runs: its one block there.
func (e *Evidence) boxedCaptionBlocks(label rec.Map) []rec.Map {
	caption := false
	for _, row := range e.records {
		caption = caption || (row["kind"] == "diagnostic" && row["code"] == "caption_context" &&
			rec.Equal(row["entity"], rec.Fetch(label, "target_entity")))
	}
	if !caption {
		return []rec.Map{}
	}
	own := e.entityBlocks(rec.Fetch(label, "entity"))
	// The caption is the block that starts with its name and number (Figure 3:).
	opening := regexp.MustCompile(`\A` + rubySpace + `*\p{L}+\.?` + rubySpace + `+` + regexp.QuoteMeta(toS(rec.Fetch(label, "number"))) + `\b`)
	opens := func(blocks []rec.Map) []rec.Map {
		captions := []rec.Map{}
		for _, block := range blocks {
			if opening.MatchString(textOf(block, func(span rec.Map) bool { return span["type"] == "text" })) {
				captions = append(captions, block)
			}
		}
		return captions
	}
	if len(own) == 1 {
		// A wrapfigure's picture paragraph runs the label while the caption sits in a block of its own.
		if len(opens(own)) == 0 {
			if caption := one(opens(e.entityBlocks(rec.Fetch(label, "target_entity")))); len(caption) == 1 {
				return caption
			}
		}
		return own
	}
	return one(opens(own))
}

func (e *Evidence) unownedLabelBlocks(label rec.Map) []rec.Map {
	for _, find := range []func(rec.Map) []rec.Map{e.richCaptionBlocks, e.followingCaptionBlocks, e.printedTagBlocks, e.precedingFormulaBlocks,
		e.ownSubfigureBlocks, e.precedingHeadingBlocks, e.enclosingHeadingBlocks, e.ownCaptionLineBlocks, e.unobservedItemBlocks} {
		if blocks := find(label); len(blocks) > 0 {
			return blocks
		}
	}
	return []rec.Map{}
}

// leadingText is the text of a block's opening run of text spans.
func leadingText(block rec.Map) string {
	parts := []rec.Map{}
	for _, span := range content(block) {
		if span["type"] != "text" {
			break
		}
		parts = append(parts, span)
	}
	return joinedText(parts)
}

// ownCaptionLineBlocks: a caption a package sets as a line of its own (algorithm2e's
// "Algorithm 1: ...", with no caption context) names the label in the same context: the one
// block opening with its number. An algorithm2e line label (AlgoLine) names the line its
// number opens ("2compute ...").
func (e *Evidence) ownCaptionLineBlocks(label rec.Map) []rec.Map {
	entity := rec.Fetch(label, "entity")
	if own := e.captionLineBlocks(label, entity); len(own) > 0 {
		return own
	}
	// The caption may run on the other side of its caption context (the float's entity or the
	// caption's own: papers2/0372's table labels, algorithm2e's label before \caption).
	if partner := e.captionPartner(entity); partner != nil && positive(partner) {
		if caption := e.captionLineBlocks(label, partner); len(caption) > 0 {
			return caption
		}
	}
	if named := e.namedCaptionLineBlocks(label); len(named) > 0 {
		return named
	}
	return e.unnumberedBoxBlocks(label)
}

// captionPartner is the other side of a caption context: the float of a caption entity, or the
// caption entity of a float; nil when no caption context names the entity.
func (e *Evidence) captionPartner(entity any) any {
	for _, row := range e.records {
		if row["kind"] != "diagnostic" || row["code"] != "caption_context" {
			continue
		}
		if rec.Equal(row["entity"], entity) && row["float"] != nil && positive(row["float"]) {
			return row["float"]
		}
		if rec.Equal(row["float"], entity) {
			return row["entity"]
		}
	}
	return nil
}

// captionLineBlocks: the one block of entity that opens with the label's printed caption line.
// Two statements of one number in the entity ("Remark 6.4.1:" and "Example 6.4.1:", books/0006)
// leave the one whose name is the label's counter.
func (e *Evidence) captionLineBlocks(label rec.Map, entity any) []rec.Map {
	opens := captionLineOpening(label)
	if opens == nil {
		return []rec.Map{}
	}
	own := e.blocksWhere(func(block rec.Map) bool {
		return rec.Equal(rec.Fetch(block, "entity"), entity) && opens(captionText(block))
	})
	if named := namedByCounter(own, label); len(own) > 1 && len(named) == 1 {
		return named
	}
	return one(own)
}

// namedCaptionLineBlocks: a statement whose head is set into the next paragraph of another context
// (a remark opening with a list: its head runs in the first item, papers2/0414; an example opening
// with a list in books/0006) names the one block of the document that opens with its caption line
// under its counter's name ("Remark 1 (...)." for remark 1).
func (e *Evidence) namedCaptionLineBlocks(label rec.Map) []rec.Map {
	opens := captionLineOpening(label)
	if opens == nil || strings.HasPrefix(toS(label["anchor"]), "AlgoLine.") || toS(label["counter"]) == "" {
		return []rec.Map{}
	}
	return one(namedByCounter(e.blocksWhere(func(block rec.Map) bool { return opens(captionText(block)) }), label))
}

// captionText is the text a caption line opens with: the opening run of text spans after the
// pictures a class sets first in the caption paragraph (aipproc's figure and its "FIGURE 1." caption,
// papers3/0852).
func captionText(block rec.Map) string {
	spans := content(block)
	start := 0
	for start < len(spans) && (spans[start]["type"] == "image" || spans[start]["type"] == "rendered_graphic") {
		start++
	}
	if start == 0 {
		return leadingText(block)
	}
	return leadingText(rec.Merge(block, rec.Map{"content": maps(spans[start:])}))
}

// namesCounter: a caption name is the counter's ("Remark" of remark), or a word of a class's own
// counter name ("Algorithm" of MYalgorithmic, papers5/1242).
func namesCounter(word, counter string) bool {
	if counter == "" {
		return false
	}
	return strings.EqualFold(word, counter) || len(word) >= 4 && strings.Contains(strings.ToLower(counter), strings.ToLower(word))
}

// captionWord is the name a caption line opens with ("Remark" of "Remark 1.").
var captionWord = regexp.MustCompile(`\A` + rubySpace + `*(\p{L}+)`)

// namedByCounter: the blocks whose caption line opens with the label's counter as its name.
func namedByCounter(blocks []rec.Map, label rec.Map) []rec.Map {
	counter := toS(label["counter"])
	named := []rec.Map{}
	for _, block := range blocks {
		if word := captionWord.FindStringSubmatch(captionText(block)); word != nil && namesCounter(word[1], counter) {
			named = append(named, block)
		}
	}
	return named
}

// captionLineOpening: the test of a text opening with the label's printed caption line; nil when the
// label prints none (no literal number, an equation or a sectioning label).
func captionLineOpening(label rec.Map) func(string) bool {
	number := evidence.LiteralNumber(toS(rec.Fetch(label, "number")))
	counter := label["counter"]
	if number == nil || *number == "" || slices.Contains([]any{"equation", "chapter", "section", "subsection", "subsubsection"}, counter) {
		return nil
	}
	escaped := regexp.QuoteMeta(*number)
	// The number ends at a separator ("Table 1:", "Lemma 5.2."), at a parenthetical title ("Lemma 5.1
	// (Lovász Local Lemma).", papers4/1105) or at the caption's first word ("Table 1 Stellar wind…",
	// a class with no separator, papers2/0352).
	caption := regexp.MustCompile(`\A` + rubySpace + `*\p{L}+` + rubySpace + `+` + escaped +
		`(?:` + rubySpace + `*[:.]|` + rubySpace + `+[(\p{Lu}])`)
	if !strings.HasPrefix(toS(label["anchor"]), "AlgoLine.") {
		return caption.MatchString
	}
	return func(text string) bool {
		// \A\s*NUMBER(?!\d), without the lookahead RE2 lacks.
		rest := strings.TrimLeft(text, " \t\n\v\f\r")
		if !strings.HasPrefix(rest, *number) {
			return false
		}
		rest = rest[len(*number):]
		return rest == "" || rest[0] < '0' || rest[0] > '9'
	}
}

// sectionCounterOf is the counter of a label or, where a class records none (emulateapj's
// \section), the sectioning counter its anchor names ("section.3").
func sectionCounterOf(label rec.Map) string {
	if counter := toS(label["counter"]); counter != "" {
		return counter
	}
	name, _, found := strings.Cut(toS(label["anchor"]), ".")
	if found && slices.Contains(sectionCounters, name) {
		return name
	}
	// An appendix section's anchor names the appendix (hyperref's appendix.A); it is a section
	// that prints a letter (papers4/1072 app:bin, papers5/1210, papers2/0372).
	if found && name == "appendix" {
		return "section"
	}
	return ""
}

// printedNumberBlocks: a sectioning label names the block that prints its number. A class
// whose own sectioning records no section context for a run-in heading (JMLR's \subsection)
// leaves the label on the enclosing section, whose heading prints a shorter number; the
// heading set just before the label prints this label's number.
func (e *Evidence) printedNumberBlocks(label rec.Map, blocks []rec.Map) []rec.Map {
	number := toS(label["number"])
	if number == "" || !slices.Contains(sectionCounters, sectionCounterOf(label)) {
		return []rec.Map{}
	}
	for _, block := range blocks {
		if printedNumber(leadingText(block), number) {
			return e.separatedHeadingBlocks(blocks, number)
		}
	}
	if preceding := e.precedingHeadingBlocks(label); len(preceding) == 1 {
		return preceding
	}
	return e.onlyHeadingPrinting(number)
}

// appendixName is the name an appendix heading prints before its letter ("APPENDIX E: ...").
var appendixName = regexp.MustCompile(`\A` + rubySpace + `*(?i:appendix)` + rubySpace + `+`)

// separatedHeadingBlocks: a context whose first block is not its heading (emulateapj sets the title,
// the affiliations and the first section in one context, papers5/1245) names the one heading of it
// that prints the number as a word of its own ("1. Introduction", not the affiliation mark of
// "1Meridian Southern Observatory"); none when the first block prints it.
func (e *Evidence) separatedHeadingBlocks(blocks []rec.Map, number string) []rec.Map {
	if len(blocks) < 2 || separatedNumber(leadingText(blocks[0]), number) {
		return []rec.Map{}
	}
	headings := []rec.Map{}
	for _, block := range blocks {
		if slices.Contains([]any{"heading", "section"}, block["type"]) && separatedNumber(leadingText(block), number) {
			headings = append(headings, block)
		}
	}
	return one(headings)
}

// separatedNumber: the text opens with the whole number, followed by its end, a space or punctuation.
func separatedNumber(text, number string) bool {
	if !printedNumber(text, number) {
		return false
	}
	rest := strings.TrimLeft(text, " \t\n\v\f\r")[len(number):]
	return rest == "" || !isASCIILetter(rest[0])
}

// onlyHeadingPrinting: a sectioning label set where no heading of its own precedes it (after
// the unnumbered bibliography: \label{LastPage}) records the last numbered section; it names
// the one heading of the document that prints that number (as the only text that opens with it).
func (e *Evidence) onlyHeadingPrinting(number string) []rec.Map {
	titles := []string{}
	for _, row := range e.records {
		if row["kind"] != "candidate" {
			continue
		}
		if text := rec.Strip(toS(rec.Fetch(row, "text"))); printedNumber(text, number) {
			titles = append(titles, text)
		}
	}
	if len(titles) != 1 {
		// Other texts open with the number too (a folio, a table cell, a list item): the label names
		// the one heading that prints it, after its name in an appendix ("APPENDIX E: THE ...", MNRAS)
		// (papers2/0422 last, papers3/0669 lastpage).
		return one(e.blocksWhere(func(block rec.Map) bool {
			text := appendixName.ReplaceAllString(leadingText(block), "")
			return slices.Contains([]any{"heading", "section"}, block["type"]) && printedNumber(text, number)
		}))
	}
	return one(e.blocksWhere(func(block rec.Map) bool { return printedNumber(leadingText(block), number) }))
}

// printedNumber: the text opens with the whole number, so "3" names a heading of "3. Title"
// but not one of "3.1 Title" (that is another counter's heading).
func printedNumber(text, number string) bool {
	text = strings.TrimLeft(text, " \t\n\v\f\r")
	if !strings.HasPrefix(text, number) {
		return false
	}
	rest := text[len(number):]
	if rest == "" {
		return true
	}
	if rest[0] >= '0' && rest[0] <= '9' {
		return false
	}
	// A lettered number (an appendix's A) ends where a word would go on: "A. BINNING", not
	// "ACKNOWLEDGMENTS".
	if last := number[len(number)-1]; isASCIILetter(last) && isASCIILetter(rest[0]) {
		return false
	}
	// "3.1" is another counter's heading; a class may print its own counter as "3." first.
	return rest[0] != '.' || len(rest) == 1 || rest[1] < '0' || rest[1] > '9'
}

// precedingHeadingBlocks: a class's own sectioning (JMLR) records no section context: a
// section label names the paragraph set just before it, when that heading (or its number
// box) begins with its number.
func (e *Evidence) precedingHeadingBlocks(label rec.Map) []rec.Map {
	if !slices.Contains(sectionCounters, sectionCounterOf(label)) {
		return []rec.Map{}
	}
	// A page that breaks at the heading ships its running head and folio between the heading
	// and its label: they are page furniture, not candidates for the heading (papers2/0351,
	// 0352, 0372).
	furniture := e.furnitureCandidates()
	candidates := []rec.Map{}
	for _, row := range e.records {
		if row["kind"] == "candidate" && less(rec.Fetch(row, "seq"), rec.Fetch(label, "seq")) &&
			!furniture[rec.FetchInt(row, "candidate")] {
			candidates = append(candidates, row)
		}
	}
	if len(candidates) > 2 {
		candidates = candidates[len(candidates)-2:]
	}
	number := toS(rec.Fetch(label, "number"))
	if len(candidates) == 0 || number == "" {
		return []rec.Map{}
	}
	// The label names the heading that prints its whole number (an appendix "C", not the "Combining"
	// of the paragraph before it): a starred heading before it (a \section*{...}\label{...}) prints
	// none and leaves the counter of the heading before that.
	start := -1
	for index, row := range candidates {
		if printedNumber(rec.Strip(toS(rec.Fetch(row, "text"))), number) {
			start = index
		}
	}
	if start < 0 {
		return []rec.Map{}
	}
	// A number box (emulateapj sets "1." in an hbox of its own) is followed by the title's
	// candidate: the heading block is then the one that candidate's flow holds.
	for _, row := range candidates[start:] {
		owner := e.candidateOwner(row)
		if blocks := one(e.blocksWhere(func(block rec.Map) bool { return rec.Equal(rec.Fetch(block, "owner"), owner) })); len(blocks) > 0 {
			return blocks
		}
	}
	return []rec.Map{}
}

// followingRunInHeadingBlocks: a run-in heading (a \subsubsection set into the paragraph it opens)
// records a context with no block of its own, and its label comes before the paragraph ships: the
// label names the next paragraph of its entity when that paragraph prints the label's whole number
// (papers4/0977 sec:justice_conflict). Inline boxes set within that paragraph (citation numbers)
// ship before it and page furniture may come between; any other paragraph stops the look-ahead.
func (e *Evidence) followingRunInHeadingBlocks(label rec.Map) []rec.Map {
	number := toS(rec.Fetch(label, "number"))
	if number == "" || !slices.Contains(sectionCounters, sectionCounterOf(label)) {
		return []rec.Map{}
	}
	furniture := e.furnitureCandidates()
	for _, row := range e.records {
		if row["kind"] != "candidate" || row["group"] != "paragraph" || !less(rec.Fetch(label, "seq"), rec.Fetch(row, "seq")) ||
			furniture[rec.FetchInt(row, "candidate")] {
			continue
		}
		if !rec.Equal(rec.Fetch(row, "entity"), rec.Fetch(label, "entity")) ||
			!printedNumber(rec.Strip(toS(rec.Fetch(row, "text"))), number) {
			return []rec.Map{}
		}
		owner := e.candidateOwner(row)
		return one(e.blocksWhere(func(block rec.Map) bool { return rec.Equal(rec.Fetch(block, "owner"), owner) }))
	}
	return []rec.Map{}
}

// furnitureCandidates: the candidates whose glyphs the shipout set aside as page decoration
// (running heads, folios, a publisher notice: excluded_page_decoration).
func (e *Evidence) furnitureCandidates() map[int64]bool {
	if e.furniture == nil {
		e.furniture = map[int64]bool{}
		for _, row := range e.records {
			if row["code"] == "excluded_page_decoration" && row["candidate"] != nil {
				e.furniture[rec.AsInt(row["candidate"])] = true
			}
		}
	}
	return e.furniture
}

// candidateOwner: a candidate record follows its flow candidate (same entity and group);
// their numbering differs.
func (e *Evidence) candidateOwner(candidate rec.Map) any {
	var found rec.Map
	seq := rec.FetchInt(candidate, "seq")
	for _, id := range e.owners.Keys() {
		row, _ := e.owners.Get(id)
		if rec.FetchInt(row, "seq") == seq-1 && rec.Equal(rec.Fetch(row, "entity"), rec.Fetch(candidate, "entity")) &&
			rec.Equal(rec.Fetch(row, "group"), rec.Fetch(candidate, "group")) {
			found = row
			break
		}
	}
	if found != nil {
		return rec.Fetch(found, "owner")
	}
	return rec.Fetch(candidate, "candidate")
}

// ownSubfigureBlocks: a subfigure label whose subcaption is not observed (subfig's
// \subfloat) names the subfigure it runs in: its one block.
func (e *Evidence) ownSubfigureBlocks(label rec.Map) []rec.Map {
	if !strings.HasPrefix(toS(label["counter"]), "sub") {
		return []rec.Map{}
	}
	return one(e.entityBlocks(rec.Fetch(label, "entity")))
}

// mainCaptionOpening starts a float's own caption ("Figure 1."): a subfigure label, whose number carries a
// letter, does not name it when the subfigure's own caption is not observed.
var mainCaptionOpening = regexp.MustCompile(`\A` + rubySpace + `*\p{L}+\.?` + rubySpace + `*\d+` + rubySpace + `*[.:]`)

// followingCaptionBlocks: listings (and the subfigure package) write a label before its
// caption: the label names the next caption of its own counter (a subfigure label, of its
// float's).
func (e *Evidence) followingCaptionBlocks(label rec.Map) []rec.Map {
	counter := label["counter"]
	for _, row := range e.records {
		if row["kind"] != "diagnostic" || row["code"] != "caption_context" || !less(rec.Fetch(label, "seq"), rec.Fetch(row, "seq")) {
			continue
		}
		category := row["category"]
		if (rec.Equal(category, counter) || rec.Equal(category, strings.TrimPrefix(toS(counter), "sub"))) && row["numbered"] == true {
			blocks := e.entityBlocks(rec.Fetch(row, "entity"))
			if strings.HasPrefix(toS(counter), "sub") && len(blocks) > 0 && mainCaptionOpening.MatchString(leadingText(blocks[0])) {
				continue
			}
			return blocks
		}
	}
	return []rec.Map{}
}

// precedingFormulaBlocks: an equation label outside an equation context (an eqnarray row)
// names the row it is typeset in: the block of the last formula started before it.
func (e *Evidence) precedingFormulaBlocks(label rec.Map) []rec.Map {
	if label["counter"] != "equation" || !equationNumber.MatchString(toS(rec.Fetch(label, "number"))) {
		return []rec.Map{}
	}
	// The last formula before the label that the reading holds (not a nested or measured one).
	read := map[any]rec.Map{}
	for _, block := range e.blocks {
		for _, span := range content(block) {
			if truthy(span["formula"]) {
				read[span["formula"]] = block
			}
		}
	}
	var candidate rec.Map
	for _, id := range e.formulas.Keys() {
		row, _ := e.formulas.Get(id)
		if _, held := read[rec.Fetch(row, "formula")]; !held || !less(rec.Fetch(row, "seq"), rec.Fetch(label, "seq")) {
			continue
		}
		if candidate == nil || less(rec.Fetch(candidate, "seq"), rec.Fetch(row, "seq")) {
			candidate = row
		}
	}
	if candidate == nil {
		return []rec.Map{}
	}
	return []rec.Map{read[rec.Fetch(candidate, "formula")]}
}

func (e *Evidence) richCaptionBlocks(label rec.Map) []rec.Map {
	if label["counter"] != "table" {
		return []rec.Map{}
	}
	var caption rec.Map
	for _, row := range e.records {
		if row["kind"] != "diagnostic" || row["code"] != "rich_table_caption" || row["source"] != tables.RichSource ||
			!less(rec.Fetch(row, "seq"), rec.Fetch(label, "seq")) || !rec.Equal(rec.Fetch(row, "anchor"), rec.Fetch(label, "anchor")) {
			continue
		}
		if caption == nil || less(rec.Fetch(caption, "seq"), rec.Fetch(row, "seq")) {
			caption = row
		}
	}
	if caption == nil {
		return []rec.Map{}
	}
	cell := rec.Fetch(caption, "cell")
	for _, block := range e.blocks {
		if hasSpan(block, func(span rec.Map) bool { return rec.Equal(e.origins[rec.Identity(span)]["cell"], cell) }) {
			return []rec.Map{block}
		}
	}
	return []rec.Map{}
}

func (e *Evidence) theoremTargetBlocks(label rec.Map) []rec.Map {
	entity := rec.Fetch(label, "target_entity")
	observed := false
	for _, record := range e.records {
		observed = observed || (record["code"] == "theorem_context" && record["source"] == "amsthm-arguments" &&
			rec.Equal(record["entity"], entity) && less(rec.Fetch(record, "seq"), rec.Fetch(label, "seq")))
	}
	if !observed {
		return []rec.Map{}
	}
	return e.blocksWhere(func(block rec.Map) bool {
		current := rec.Fetch(block, "entity")
		for positive(current) && !rec.Equal(current, entity) {
			parent, ok := e.parents[current]
			if !ok {
				panic(rec.Failure{Message: "key not found: " + toS(current), Lookup: true})
			}
			current = parent
		}
		return rec.Equal(current, entity)
	})
}

func isASCIILetter(char byte) bool {
	return (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')
}

// starredSectionAnchor: hyperref's anchor of an unnumbered heading (section*.2, chapter*.2).
var starredSectionAnchor = regexp.MustCompile(`\A(chapter|section|subsection|subsubsection)\*\.`)

// enclosingHeadingBlocks: a sectioning label whose heading prints no number names the heading of
// the section it is set in. An unnumbered heading (\chapter*, or secnumdepth 0 with the number typed
// in the title) records an empty number and a starred anchor (papers2/0385, books/0006); an
// emulateapj appendix prints "APPENDIX" and its title but not its letter (papers2/0352 app:exosat).
// The label names the innermost heading context around it, when that heading's counter is the
// label's and its one heading block prints no other number.
func (e *Evidence) enclosingHeadingBlocks(label rec.Map) []rec.Map {
	number := toS(rec.Fetch(label, "number"))
	counter := sectionCounterOf(label)
	if number == "" {
		match := starredSectionAnchor.FindStringSubmatch(toS(label["anchor"]))
		if match == nil {
			return []rec.Map{}
		}
		counter = match[1]
	}
	if !slices.Contains(sectionCounters, counter) {
		return []rec.Map{}
	}
	section := e.enclosingHeading(rec.Fetch(label, "entity"))
	if section == nil || e.headingCounters[section] != counter {
		return []rec.Map{}
	}
	heading := one(e.blocksWhere(func(block rec.Map) bool {
		return rec.Equal(rec.Fetch(block, "entity"), section) && slices.Contains([]any{"heading", "section"}, block["type"])
	}))
	if len(heading) == 1 && number != "" {
		text := strings.TrimLeft(leadingText(heading[0]), " \t\n\v\f\r")
		if !printedNumber(text, number) && headingNumber.MatchString(text) {
			return []rec.Map{}
		}
	}
	return heading
}

// headingNumber opens a heading that prints a number of its own ("A. NIGHT LOG", "3.2 Results",
// "IV Methods"): an appendix table's label (number 4, anchor appendix.A) does not name it (papers2/0372).
var headingNumber = regexp.MustCompile(`\A(?:\d+|[A-Z]|[IVXLCDM]+)(?:\.\d+)*\.?(?:` + rubySpace + `|\z)`)

// enclosingHeading: the innermost heading context that holds entity (itself or an ancestor); nil
// when none does.
func (e *Evidence) enclosingHeading(entity any) any {
	if e.headingCounters == nil {
		e.headingCounters = map[any]string{}
		for _, row := range e.records {
			if row["kind"] == "diagnostic" && row["code"] == "heading_context" {
				e.headingCounters[rec.Fetch(row, "entity")] = toS(row["counter"])
			}
		}
	}
	for current := entity; positive(current); {
		if _, found := e.headingCounters[current]; found {
			return current
		}
		parent, ok := e.parents[current]
		if !ok {
			return nil
		}
		current = parent
	}
	return nil
}

// unnumberedBoxAnchor: the anchors of labels that name a page, a note or front matter, not a box.
var unnumberedBoxAnchor = regexp.MustCompile(`\A(?:Doc-Start|page\.|footnote\.|Hfootnote\.|address\.|Item\.)`)

// boxTitle opens a box whose title prints its name and number ("Focus Box 2: Evaluating ...").
var boxTitle = regexp.MustCompile(`\A` + rubySpace + `*\p{L}+(?:` + rubySpace + `+\p{L}+)*` + rubySpace + `+\d+` + rubySpace + `*:`)

// unnumberedBoxBlocks: a label in a box whose title is typed with its number (a tcolorbox titled
// "Focus Box 2: ...", papers5/1187) records no number and whatever anchor hyperref set last: it
// names the one block of its context that opens with such a title.
func (e *Evidence) unnumberedBoxBlocks(label rec.Map) []rec.Map {
	entity := rec.Fetch(label, "entity")
	if toS(rec.Fetch(label, "number")) != "" || toS(label["counter"]) != "" || !positive(entity) ||
		unnumberedBoxAnchor.MatchString(toS(label["anchor"])) {
		return []rec.Map{}
	}
	return one(e.blocksWhere(func(block rec.Map) bool {
		return rec.Equal(rec.Fetch(block, "entity"), entity) && boxTitle.MatchString(leadingText(block))
	}))
}
