package reading

import (
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

var (
	parenthesized = regexp.MustCompile(`\A\(.+\)\z`)
	subLetter     = regexp.MustCompile(`\A[a-z]\z`)
)

// formulaLabel: a label in an unnumbered display (gather*) carries no equation number: like
// LaTeX, it names the enclosing numbered element (its section), not the formula.
func formulaLabel(label rec.Map) bool {
	return truthy(label["formula"]) && !truthy(label["enclosing"])
}

// enclosingLabel: a label in an unnumbered display (gather*) without a row of its own names,
// like LaTeX, the enclosing numbered element (its section) when its counter says so.
func enclosingLabel(label rec.Map) rec.Map {
	counter := label["counter"]
	if !truthy(label["formula"]) || !strings.HasSuffix(toS(label["math_environment"]), "*") || !truthy(counter) || counter == "equation" {
		return nil
	}
	return rec.Merge(label, rec.Map{"enclosing": true})
}

// withoutSeq is label.except('seq').
func withoutSeq(label rec.Map) rec.Map {
	result := rec.Map{}
	for key, value := range label {
		if key != "seq" {
			result[key] = value
		}
	}
	return result
}

func (e *Evidence) resolveTargets() {
	groups := omap.New[any, []rec.Map]()
	for _, record := range e.records {
		if rec.Fetch(record, "kind") != "label" {
			continue
		}
		name := rec.Fetch(record, "name")
		labels, _ := groups.Get(name)
		groups.Set(name, append(labels, record))
	}
	groups.Each(func(name any, all []rec.Map) {
		// A caption measured before it is set runs its \label twice, identically.
		labels := []rec.Map{}
		for _, label := range all {
			duplicate := false
			for _, kept := range labels {
				duplicate = duplicate || reflect.DeepEqual(withoutSeq(kept), withoutSeq(label))
			}
			if !duplicate {
				labels = append(labels, label)
			}
		}
		// A name defined twice: as in LaTeX, the last definition is the one references print.
		if len(labels) > 1 {
			e.diagnostics = append(e.diagnostics, rec.Map{"code": "multiply_defined_label", "label": name})
		}
		label := labels[0]
		for _, row := range labels[1:] {
			if less(rec.Fetch(label, "seq"), rec.Fetch(row, "seq")) {
				label = row
			}
		}
		e.resolveLabel(name, label)
	})
}

func (e *Evidence) resolveLabel(name any, label rec.Map) {
	label = e.printedRowLabel(e.displayTagLabel(label))
	blocks := e.targetBlocks(label)
	unmatched := len(blocks) != 1
	if !unmatched && e.mathRows != nil {
		_, ok := e.mathRows.target(label, blocks[0])
		unmatched = !ok
	}
	if enclosing := enclosingLabel(label); unmatched && enclosing != nil {
		label = enclosing
		blocks = e.targetBlocks(label)
	}
	if len(blocks) == 0 || (formulaLabel(label) && len(blocks) != 1) {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "unresolved_label_owner", "label": name})
		if len(blocks) == 0 && !formulaLabel(label) {
			e.unowned = append(e.unowned, label)
		}
		return
	}
	e.resolveTarget(label, blocks[0])
}

func (e *Evidence) resolveTarget(label, block rec.Map) {
	row := rec.Map{}
	if formulaLabel(label) && e.mathRows != nil {
		var ok bool
		if row, ok = e.mathRows.target(label, block); !ok {
			e.diagnostics = append(e.diagnostics, rec.Map{"code": "unresolved_math_row_target", "label": rec.Fetch(label, "name")})
			return
		}
	}
	target := rec.Map{"label": rec.Fetch(label, "name"), "number": rec.Fetch(label, "number"), "block": rec.Fetch(block, "id"),
		"anchor": rec.Fetch(label, "anchor")}
	e.targets = append(e.targets, rec.Merge(target, row))
}

func (e *Evidence) blocksWhere(test func(rec.Map) bool) []rec.Map {
	result := []rec.Map{}
	for _, block := range e.blocks {
		if test(block) {
			result = append(result, block)
		}
	}
	return result
}

func hasSpan(block rec.Map, test func(rec.Map) bool) bool {
	for _, span := range content(block) {
		if test(span) {
			return true
		}
	}
	return false
}

func (e *Evidence) targetBlocks(label rec.Map) []rec.Map {
	if last := e.lastBibliographyItem(label); len(last) == 1 {
		return last
	}
	if formulaLabel(label) {
		formula := rec.Fetch(label, "formula")
		return e.continuedFormulaBlocks(label, e.blocksWhere(func(block rec.Map) bool {
			return hasSpan(block, func(span rec.Map) bool { return rec.Equal(span["formula"], formula) })
		}))
	}
	entity := e.floatCaptionEntity(label, e.footnoteEntity(label, rec.Fetch(label, "target_entity")))
	if !positive(entity) {
		return e.unownedLabelBlocks(label)
	}
	blocks := e.blocksWhere(func(block rec.Map) bool { return rec.Equal(rec.Fetch(block, "entity"), entity) })
	if anchored := e.sameAnchorBlocks(label, blocks); len(anchored) == 1 {
		return anchored
	}
	if len(blocks) == 0 {
		if caption := e.richCaptionBlocks(label); len(caption) > 0 {
			return caption
		}
		if boxed := e.boxedCaptionBlocks(label); len(boxed) > 0 {
			return boxed
		}
		// A caption context with no block of its own: its caption line is read in the float's
		// entity (papers5/1210 t:*, 1318's tables).
		if partner := e.captionPartner(entity); partner != nil && positive(partner) {
			if caption := e.captionLineBlocks(label, partner); len(caption) > 0 {
				return caption
			}
		}
		if heading := e.followingRunInHeadingBlocks(label); len(heading) == 1 {
			return heading
		}
		return e.theoremTargetBlocks(label)
	}
	if label["counter"] != "equation" {
		if !hasText(blocks) {
			if boxed := e.boxedCaptionBlocks(label); len(boxed) > 0 {
				return boxed
			}
		}
		if heading := e.printedNumberBlocks(label, blocks); len(heading) == 1 {
			return heading
		}
		if printed := e.printedFloatBlocks(label, blocks); len(printed) == 1 {
			return printed
		}
		return blocks
	}
	return e.equationBlocks(label, blocks)
}

func (e *Evidence) equationBlocks(label rec.Map, blocks []rec.Map) []rec.Map {
	equations := 0
	for _, record := range e.records {
		if record["code"] == "equation_context" && record["source"] == "latex-equation-hooks" &&
			rec.Equal(record["entity"], rec.Fetch(label, "target_entity")) && less(rec.Fetch(record, "seq"), rec.Fetch(label, "seq")) {
			equations++
		}
	}
	if equations != 1 {
		return []rec.Map{}
	}
	displays := []rec.Map{}
	pictures := []rec.Map{}
	for _, block := range blocks {
		if rec.Fetch(block, "type") == "display_math" {
			displays = append(displays, block)
		}
		if _, ok := block["content"]; ok && hasSpan(block, func(span rec.Map) bool { return span["type"] == "rendered_graphic" }) {
			pictures = append(pictures, block)
		}
	}
	// An eqnarray whose rows were set as several displays: the label names the display that prints its
	// number, as trailing text or in its formula's row label cell (a \label at a row's end, fourth cohort 1038).
	if tagged := e.printingDisplays(displays, printedLabelNumber(label)); len(displays) > 1 && len(tagged) == 1 {
		return tagged
	}
	if tag := e.unprintedDisplayTagBlocks(label, displays, blocks); len(tag) == 1 {
		return tag
	}
	// One numbered equation may span several display blocks; the label names the first.
	if len(displays) > 0 {
		return displays[:1]
	}
	// A class that sets an equation as text-mode math in a line-wide box (iopart's \equation:
	// \hbox to\linewidth{$\displaystyle ...$\@eqnnum}, third cohort 629) leaves one paragraph of
	// its formula and its printed tag: the label names that paragraph.
	if tagged := taggedDisplays(blocks, printedLabelNumber(label)); len(pictures) == 0 && len(tagged) == 1 &&
		hasSpan(tagged[0], func(span rec.Map) bool { return span["type"] == "math" }) {
		return tagged
	}
	// An equation that is only a picture (an xymatrix diagram) prints its number in a block of its
	// own beside the picture: the label names that block, or the picture when none prints it.
	if len(pictures) == 1 {
		if tag := tagBlocks(blocks, label); len(tag) == 1 {
			return tag
		}
		return pictures
	}
	return taggedRowBlocks(blocks, label)
}

// tagBlocks are the blocks of text only that print exactly "(number)", the label's equation tag.
func tagBlocks(blocks []rec.Map, label rec.Map) []rec.Map {
	number := evidence.LiteralNumber(rec.FetchString(label, "number"))
	if number == nil {
		return nil
	}
	tags := []rec.Map{}
	for _, block := range blocks {
		spans := content(block)
		allText := len(spans) > 0
		for _, span := range spans {
			allText = allText && span["type"] == "text"
		}
		if allText && rec.Strip(joinedText(spans)) == "("+*number+")" {
			tags = append(tags, block)
		}
	}
	return tags
}

// taggedDisplays are the displays whose last span is the text "(number)": the ones printing that
// equation number beside their formula.
func taggedDisplays(displays []rec.Map, number string) []rec.Map {
	if number == "" {
		return nil
	}
	tagged := []rec.Map{}
	for _, display := range displays {
		spans := content(display)
		if len(spans) == 0 {
			continue
		}
		last := spans[len(spans)-1]
		if last["type"] == "text" && strings.TrimSpace(toS(last["text"])) == "("+number+")" {
			tagged = append(tagged, display)
		}
	}
	return tagged
}

// subreference: \subref names subcaption's sub@X, which holds only the subfigure's letter:
// it links to X's target when the printed letter is the one X's number ends with ((a) for
// 1a or 1(a)).
func subreference(targets map[any]rec.Map, span rec.Map, printed string) (rec.Map, *string) {
	name := rec.FetchString(span, "target")
	if !strings.HasPrefix(name, "sub@") {
		return nil, nil
	}
	base := targets[strings.TrimPrefix(name, "sub@")]
	if base == nil {
		return nil, nil
	}
	number := evidence.LiteralNumber(rec.FetchString(base, "number"))
	letter := strings.NewReplacer("(", "", ")", "").Replace(printed)
	if number == nil || !subLetter.MatchString(letter) ||
		!strings.HasSuffix(strings.NewReplacer("(", "", ")", "").Replace(*number), letter) {
		return nil, nil
	}
	return base, &printed
}

func (e *Evidence) resolveReferences() {
	targets := map[any]rec.Map{}
	for _, target := range e.targets {
		targets[rec.Fetch(target, "label")] = target
	}
	split := e.resolveSplitUses(targets)
	linked := map[any]bool{}
	linkedSpans := map[any]rec.Map{}
	for _, block := range e.blocks {
		for _, span := range content(block) {
			if rec.Fetch(span, "type") == "reference" && !split[span["reference"]] {
				e.resolveReference(targets, span, false)
				reference := rec.Fetch(span, "reference")
				if _, seen := linkedSpans[reference]; !seen && !unlinked(span) {
					linkedSpans[reference] = span
				}
				linked[reference] = linked[reference] || !unlinked(span)
			}
		}
	}
	for _, entry := range e.mathReferences {
		for _, value := range rec.FetchList(entry, "content") {
			span := rec.AsMap(value)
			if split[span["reference"]] {
				continue
			}
			if !e.continuedLink(linked, span) {
				e.resolveReference(targets, span, true)
				e.linkMathNumber(targets, entry, span)
			} else if enclosing, ok := e.enclosingLinks[entry["occurrence"]]; ok && rec.Equal(enclosing.reference, span["reference"]) {
				linkFormula(enclosing.span, linkedSpans[rec.Fetch(span, "reference")])
			}
		}
	}
}

// referenceNumber is the text a use of the label prints: the literal number, or the glyphs of a
// number typeset as math (an equation tagged \tag{$A$}); nil when neither.
func referenceNumber(number string) *string {
	if literal := evidence.LiteralNumber(number); literal != nil {
		return literal
	}
	return evidence.MathNumber(number)
}

// linkMathNumber: a formula that is wholly the printed math number of a resolved reference
// (\eqref{A} prints the A of \tag{$A$} as math) is that reference's link, so the math keeps its
// MathML and links to the target.
func (e *Evidence) linkMathNumber(targets map[any]rec.Map, entry, span rec.Map) {
	target := targets[rec.Fetch(span, "target")]
	enclosing, ok := e.enclosingLinks[entry["occurrence"]]
	if target == nil || !ok || !rec.Equal(enclosing.reference, span["reference"]) || unlinked(span) {
		return
	}
	if evidence.LiteralNumber(rec.FetchString(target, "number")) == nil {
		linkFormula(enclosing.span, span)
	}
}

// linkFormula: a formula set wholly inside a \hyperref text whose words are linked ("$c$-packed")
// belongs to that link: its math span takes the link's label and target, and its MathML is kept.
func linkFormula(math, text rec.Map) {
	link := rec.Map{"target": rec.Fetch(text, "target")}
	for _, key := range []string{"target_block", "target_row", "target_page_total"} {
		if value, ok := text[key]; ok {
			link[key] = value
		}
	}
	math["link"] = link
}

// unlinked: the reference span was bound to no target.
func unlinked(span rec.Map) bool {
	_, block := span["target_block"]
	_, row := span["target_row"]
	_, total := span["target_page_total"]
	return !block && !row && !total
}

// continuedLink: a \hyperref whose text runs into a formula (the book's \HLinkPage prints
// "Theorem 1.2.8" then $_\text{p\pageref{..}}$) is one use: its text outside the formula is
// verified and linked, and the piece inside the formula ("p") stays unlinked text of that use.
func (e *Evidence) continuedLink(linked map[any]bool, span rec.Map) bool {
	definition, _ := e.references.Get(rec.Fetch(span, "reference"))
	return linked[rec.Fetch(span, "reference")] && rec.Fetch(definition, "selector") == `\hyperref`
}

// resolveReference: inMath marks a use printed inside a formula, where a number's hyphen
// (\ref of A-1.3 in \stackrel{\rm ...}) prints as the math minus sign.
func (e *Evidence) resolveReference(targets map[any]rec.Map, span rec.Map, inMath bool) {
	target := targets[rec.Fetch(span, "target")]
	definition, _ := e.references.Get(rec.Fetch(span, "reference"))
	selector := rec.Fetch(definition, "selector")
	if selector == pageSelector || selector == namedPageSelector {
		e.resolvePageReference(targets, span, selector == namedPageSelector)
		return
	}
	printed := rec.Strip(squeeze(rec.FetchString(span, "text"), " "))
	if inMath {
		printed = strings.ReplaceAll(printed, "−", "-")
	}
	var printedNumber *string
	if target == nil {
		target, printedNumber = subreference(targets, span, printed)
	}
	if printedNumber != nil {
		span["target"] = rec.Fetch(target, "label")
	}
	var expected *string
	if target != nil {
		expected = printedNumber
		if expected == nil {
			expected = referenceNumber(rec.FetchString(target, "number"))
		}
	}
	// A \hyperref to a label with no number (an unnumbered heading, a box titled with its number)
	// links whatever text the author gives it ("Section~II", "Focus Box 1": papers2/0385, papers5/1187).
	named := target != nil && selector == `\hyperref` && rec.FetchString(target, "number") == "" && printed != ""
	if !named && (target == nil || expected == nil || !printedUse(selector, printed, *expected)) {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "unresolved_reference_use", "reference": rec.Fetch(span, "id"),
			"target": rec.Fetch(span, "target")})
		return
	}
	if target["kind"] == "document_page_total" {
		span["target_page_total"] = rec.Fetch(target, "label")
	} else {
		span["target_block"] = rec.Fetch(target, "block")
	}
	if truthy(target["row"]) {
		span["target_row"] = rec.Fetch(target, "row")
	}
}

func joinedText(spans []rec.Map) string {
	var builder strings.Builder
	for _, span := range spans {
		builder.WriteString(rec.FetchString(span, "text"))
	}
	return builder.String()
}

// taggedRowBlocks: an alignment read without rows (its rows are pictures) is a run of blocks
// per row, each ending with its tag ("(17)"): the label names the first block of its tag's row.
func taggedRowBlocks(blocks []rec.Map, label rec.Map) []rec.Map {
	number := evidence.LiteralNumber(rec.FetchString(label, "number"))
	tags := []int{}
	for index, block := range blocks {
		spans := content(block)
		allText := true
		for _, span := range spans {
			allText = allText && span["type"] == "text"
		}
		if allText && parenthesized.MatchString(rec.Strip(joinedText(spans))) {
			tags = append(tags, index)
		}
	}
	tag := "()"
	if number != nil {
		tag = "(" + *number + ")"
	}
	own := -1
	for _, index := range tags {
		if rec.Strip(joinedText(content(blocks[index]))) == tag {
			own = index
			break
		}
	}
	if own < 0 {
		return []rec.Map{}
	}
	start := 0
	for _, index := range tags {
		if index < own && index+1 > start {
			start = index + 1
		}
	}
	return []rec.Map{blocks[start]}
}

// floatCaptionEntity: a subfigure label of the old subfigure package runs before its float's
// caption, so the caption last set (the previous float's) is its recorded target. A label whose
// target caption belongs to another float names the caption of its own float instead.
func (e *Evidence) floatCaptionEntity(label rec.Map, entity any) any {
	if !strings.HasPrefix(toS(label["counter"]), "sub") || !positive(entity) {
		return entity
	}
	for _, row := range e.records {
		if row["kind"] == "diagnostic" && row["code"] == "caption_context" && rec.Equal(row["entity"], entity) {
			if rec.Equal(row["float"], rec.Fetch(label, "entity")) {
				return entity
			}
			break
		}
	}
	for _, row := range e.records {
		if row["kind"] == "diagnostic" && row["code"] == "caption_context" && row["numbered"] == true &&
			rec.Equal(row["float"], rec.Fetch(label, "entity")) && less(rec.Fetch(label, "seq"), rec.Fetch(row, "seq")) {
			return rec.Fetch(row, "entity")
		}
	}
	return entity
}

// sectionAnchor is hyperref's anchor of a numbered sectioning heading.
var sectionAnchor = regexp.MustCompile(`\A(?:chapter|section|subsection|subsubsection)\.[0-9A-Z]`)

// sameAnchorBlocks: a sectioning label set after an unnumbered heading at the document's end
// (MNRAS's \label{lastpage} after the bibliography) records the last numbered section's number
// and hyperref anchor (section.4), which name that section, not the unnumbered heading it sits
// under. An earlier label with the same anchor and number names the heading printing it.
func (e *Evidence) sameAnchorBlocks(label rec.Map, blocks []rec.Map) []rec.Map {
	anchor, number := toS(label["anchor"]), toS(label["number"])
	if !sectionAnchor.MatchString(anchor) || number == "" {
		return nil
	}
	for _, block := range blocks {
		if printedNumber(leadingText(block), number) {
			return nil
		}
	}
	for _, row := range e.records {
		if row["kind"] != "label" || !less(rec.Fetch(row, "seq"), rec.Fetch(label, "seq")) ||
			toS(row["anchor"]) != anchor || toS(row["number"]) != number || rec.Equal(row["name"], label["name"]) {
			continue
		}
		found := e.targetBlocks(row)
		if len(found) == 1 && slices.Contains([]any{"heading", "section"}, rec.Fetch(found[0], "type")) && printedNumber(leadingText(found[0]), number) {
			return found
		}
	}
	return nil
}

// lastBibliographyItem: a class's \label{LastBibItem} (set after the bibliography) names the
// last bibliography entry, the block printing "[N]" or "N" for the label's number N.
func (e *Evidence) lastBibliographyItem(label rec.Map) []rec.Map {
	number := toS(label["number"])
	if label["name"] != "LastBibItem" || number == "" {
		return []rec.Map{}
	}
	printing := one(e.blocksWhere(func(block rec.Map) bool {
		text := strings.TrimLeft(leadingText(block), " \t\n\v\f\r")
		return strings.HasPrefix(text, "["+number+"]") || printedNumber(text, number)
	}))
	if len(printing) == 1 {
		return printing
	}
	return e.numberedEntryBlocks(number)
}

// numberedEntryBlocks: an author-year style prints no label number (REVTeX's rmp style, third
// cohort 599), so LastBibItem names the first block of the one bibliography entry recorded
// with that number.
func (e *Evidence) numberedEntryBlocks(number string) []rec.Map {
	entries := []any{}
	for _, record := range e.records {
		if record["code"] == "bibliography_anchor" && toS(record["number"]) == number {
			entries = append(entries, record["entry"])
		}
	}
	if len(entries) != 1 {
		return []rec.Map{}
	}
	blocks := e.blocksWhere(func(block rec.Map) bool {
		return hasSpan(block, func(span rec.Map) bool { return rec.Equal(span["bibliography_entry"], entries[0]) })
	})
	if len(blocks) == 0 {
		return blocks
	}
	return blocks[:1]
}

// hasText: some block holds a text span (a picture-only paragraph does not).
func hasText(blocks []rec.Map) bool {
	for _, block := range blocks {
		if hasSpan(block, func(span rec.Map) bool { return span["type"] == "text" }) {
			return true
		}
	}
	return false
}

// printedUse: whether the printed text is the use of the target number. \ref prints the number
// alone; \hyperref[label]{text} prints the author text, which links to the label's target as
// printed. A text that prints a number names the target's number: it must end with it as a word,
// bare or parenthesized, so a stale or foreign number stays unlinked text.
func printedUse(selector any, printed, expected string) bool {
	expected = rec.Strip(squeeze(expected, " "))
	switch selector {
	case `\@firstoffive`:
		return printed == expected
	case `\hyperref`:
		// The author's link text names the number, alone or after its words, bare or in the
		// parentheses of an equation ("Lemma 2.1", "Eq. (30.2)", "(30.2)").
		for _, number := range []string{expected, "(" + expected + ")"} {
			if printed == number || strings.HasSuffix(printed, " "+number) {
				return true
			}
		}
		// A text printing no digit is a name ("$c$-packed", "dense"): hyperref links it as is.
		return printed != "" && !strings.ContainsAny(printed, "0123456789")
	}
	return false
}

// resolveProjectedTargets: a table label read before its table is projected finds no block (a
// deluxetable's caption runs in the continued table's grid, papers5/1210 t:*, papers2/0372
// tab:nightlog); once the tables are projected, it names the one block that now opens with its
// caption line.
func (e *Evidence) resolveProjectedTargets() {
	resolved := map[any]bool{}
	for _, label := range e.unowned {
		blocks := e.targetBlocks(label)
		if len(blocks) != 1 {
			continue
		}
		resolved[rec.Fetch(label, "name")] = true
		e.resolveTarget(label, blocks[0])
	}
	if len(resolved) == 0 {
		return
	}
	kept := []rec.Map{}
	for _, diagnostic := range e.diagnostics {
		owner := diagnostic["code"] == "unresolved_label_owner" && resolved[diagnostic["label"]]
		use := diagnostic["code"] == "unresolved_reference_use" && resolved[diagnostic["target"]]
		if !owner && !use {
			kept = append(kept, diagnostic)
		}
	}
	e.diagnostics = kept
	// The uses of those labels were read before they had a target: they are read again.
	targets := map[any]rec.Map{}
	for _, target := range e.targets {
		targets[rec.Fetch(target, "label")] = target
	}
	for _, block := range e.blocks {
		for _, span := range content(block) {
			if span["type"] == "reference" && resolved[span["target"]] {
				e.resolveReference(targets, span, false)
			}
		}
	}
}

// footnoteAnchor is hyperref's anchor of a footnote (Hfootnote.4, or equation.11-footnote.4 where
// the footnote's anchor extends the current one): its number.
var footnoteAnchor = regexp.MustCompile(`(?:\A|[-.])H?footnote\.(\d+)\z`)

// footnoteEntity: a \label in a footnote's text names the footnote, though the counter the class
// leaves current there is the section's (papers2/0355 fraction: counter subsection, number 4,
// anchor equation.11-footnote.4); entity otherwise.
func (e *Evidence) footnoteEntity(label rec.Map, entity any) any {
	match := footnoteAnchor.FindStringSubmatch(toS(label["anchor"]))
	if match == nil || match[1] != toS(rec.Fetch(label, "number")) {
		return entity
	}
	own := rec.Fetch(label, "entity")
	for _, row := range e.records {
		if row["kind"] == "diagnostic" && row["code"] == "note_body" && rec.Equal(row["entity"], own) &&
			toS(row["number"]) == match[1] {
			return own
		}
	}
	return entity
}
