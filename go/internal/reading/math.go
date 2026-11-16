package reading

import (
	"fmt"

	"github.com/sarielhp/relatex/internal/mathml"
	"github.com/sarielhp/relatex/internal/notes"
	"github.com/sarielhp/relatex/internal/rec"
)

// nextNode is the retained math nodes' Enumerator#next; peekNode is #peek. Running out is
// the StopIteration observe_math! reads as unexpected mathematics.
func (e *Evidence) nextNode() nodeKey {
	node := e.peekNode()
	e.mathNext++
	return node
}

func (e *Evidence) peekNode() nodeKey {
	if e.mathNext >= len(e.mathNodes) {
		rec.Fail("Unexpected mathematics in reading stream")
	}
	return e.mathNodes[e.mathNext]
}

func sameNode(left, right nodeKey) bool {
	return rec.Equal(left[0], right[0]) && rec.Equal(left[1], right[1]) && rec.Equal(left[2], right[2])
}

func (e *Evidence) tableInsideFormula(item rec.Map) {
	table := e.activeTable
	if !truthy(table) {
		table = e.pending.table
	}
	if table == nil {
		return
	}
	for _, row := range e.diagnostics {
		if row["code"] == "table_inside_formula" && rec.Equal(row["table"], table) {
			return
		}
	}
	e.diagnostics = append(e.diagnostics, rec.Map{"code": "table_inside_formula", "table": table, "formula": rec.Fetch(item, "formula")})
}

func (e *Evidence) observeMath(item, owner rec.Map, page any) {
	actual := nodeKey{item["formula"], item["fragment"], item["position"]}
	skipped := e.skipUnreadMathNodes(actual)
	e.markTagFormula(item)
	if expected := e.nextNode(); !sameNode(expected, actual) {
		rec.Fail("Reading math differs from retained math nodes: read formula %v fragment %v position %v, retained %v %v %v",
			actual[0], actual[1], actual[2], expected[0], expected[1], expected[2])
	}
	if e.pending != nil {
		e.pending.noteEnclosing(item)
	}
	if e.pending != nil && !rec.Equal(e.pending.formula, rec.Fetch(item, "formula")) {
		// A reference printed in a formula set inside the open one (a tabular in $..$ whose cell
		// cites \ref) is a use of the open formula: its span belongs to the open formula's block.
		if truthy(item["reference"]) {
			e.recordMathReference(item, e.pending)
		}
		e.absorbNestedMath(item)
		return
	}
	if e.pending == nil {
		e.startMath(item, owner, page)
		e.pending.noteEnclosing(item)
	}
	pending := e.pending
	if truthy(item["reference"]) {
		e.recordMathReference(item, pending)
	}
	// Citations typeset inside a formula (a table set in math) print in its MathML, unlinked.
	if !rec.Equal(pending.citation, slice(item, citationFields...)) && !pending.citationsUnlinked {
		pending.citationsUnlinked = true
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "citation_inside_math", "formula": rec.Fetch(item, "formula")})
	}
	// A formula cannot cross the cells of a table outside it: a table it crosses is set
	// inside it (a tabular in an equation) and belongs to its MathML, not to the grid.
	if !rec.Equal(pending.cell, e.activeCell) {
		e.tableInsideFormula(item)
	}
	if !rec.Equal(pending.externalLink, item["external_link"]) {
		rec.Fail("External link changes within native formula")
	}
	pending.block["pages"] = union(rec.FetchList(pending.block, "pages"), page)
	pending.remaining -= 1 + int64(skipped)
	if pending.remaining != 0 {
		return
	}
	e.mathIndex += 1 + len(pending.nested) + pending.foreign
	if style, known := e.tagStyles[rec.ToS(pending.formula)]; known && pending.tag != nil {
		pending.tag["style"] = style
	}
	if len(pending.marker) > 0 {
		e.markerMath()
	}
	if len(pending.references) > 0 {
		e.mathReferences = append(e.mathReferences, rec.Map{"occurrence": pending.occurrence, "content": maps(pending.references)})
		if pending.enclosing != nil {
			if e.enclosingLinks == nil {
				e.enclosingLinks = map[any]enclosingLink{}
			}
			e.enclosingLinks[pending.occurrence] = enclosingLink{reference: pending.enclosing, span: pending.span}
		}
	}
	e.pending = nil
}

// noteEnclosing follows the reference each node of the open formula prints in: the formula lies
// wholly inside a reference's text while all of its nodes print in that one reference.
func (pending *pendingMath) noteEnclosing(item rec.Map) {
	reference := item["reference"]
	if !truthy(reference) || pending.read && !rec.Equal(pending.enclosing, reference) {
		reference = nil
	}
	pending.enclosing, pending.read = reference, true
}

// recordMathReference: a reference printed inside a formula (an \eqref in its \text{}) is a
// reference span of that formula. The glyphs it covers carry their printed text, which is
// verified against the label's number like an ordinary reference. The inner and outer
// occurrences of a nested formula both read the same glyphs; one source use yields one span.
func (e *Evidence) recordMathReference(item rec.Map, pending *pendingMath) {
	reference := item["reference"]
	printed, ok := item["text"].(string)
	if !ok || printed == "" {
		rec.Fail("Unsupported reference inside a formula without printed text")
	}
	if position, seen := pending.referenceAt[reference]; seen {
		span := pending.references[position]
		span["text"] = rec.FetchString(span, "text") + printed
		return
	}
	definition, ok := e.references.Get(reference)
	if !ok {
		rec.Fail("Dangling reference execution")
	}
	e.referenceCnt++
	span := rec.Map{"type": "reference", "id": fmt.Sprintf("r%d", e.referenceCnt), "reference": reference,
		"text": printed, "target": rec.Fetch(definition, "target")}
	if pending.referenceAt == nil {
		pending.referenceAt = map[any]int{}
	}
	pending.referenceAt[reference] = len(pending.references)
	pending.references = append(pending.references, span)
}

// markerMath: a footnote marker typeset as math (\fnsymbol's \ensuremath{\ast}) is the
// marker: its formula's text replaces the formula, whose occurrence is then not a formula
// of its own.
func (e *Evidence) markerMath() {
	pending := e.pending
	items := rec.FetchList(pending.block, "content")
	index := -1
	for position := len(items) - 1; position >= 0; position-- {
		if span, ok := items[position].(rec.Map); ok && rec.Identity(span) == rec.Identity(pending.span) {
			index = position
			break
		}
	}
	if index < 0 || index != len(items)-1 {
		rec.Fail("Footnote marker formula moved")
	}
	pending.block["content"] = items[:len(items)-1]
	text := notes.RemoveSpace(notes.MathMLText(e.formulaMathMLFor(pending.formula)))
	e.appendNoteMarker(pending.block, rec.Merge(pending.marker, rec.Map{"text": text, "space": ""}))
	e.nestedMath = append(e.nestedMath, pending.occurrence)
}

func (e *Evidence) formulaMathMLFor(formula any) string {
	if e.formulaMathML == nil {
		e.formulaMathML = map[any]string{}
		for _, record := range e.records {
			if rec.Fetch(record, "kind") == "math" {
				e.formulaMathML[rec.Fetch(record, "formula")] = rec.FetchString(record, "mathml")
			}
		}
	}
	mathml, ok := e.formulaMathML[formula]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + toS(formula), Lookup: true})
	}
	return mathml
}

func (e *Evidence) occurrence(index int) rec.Map {
	return rec.AsMap(rec.Index(rec.FetchList(e.retention, "occurrences"), index))
}

func (e *Evidence) formulaNodes(occurrence rec.Map) int64 {
	total := int64(0)
	for _, id := range rec.FetchList(occurrence, "fragments") {
		fragment, ok := e.fragments.Get(id)
		if !ok {
			panic(rec.Failure{Message: "key not found: " + toS(id), Lookup: true})
		}
		total += rec.FetchInt(fragment, "nodes")
	}
	return total
}

// absorbNestedMath: a formula printed inside an open formula (math inside its \text{}) is
// part of the outer formula's reading. Its occurrence started after the outer one, so it
// follows it in the retained order and is consumed with it.
func (e *Evidence) absorbNestedMath(item rec.Map) {
	nested := e.pending.nested
	formula := rec.Fetch(item, "formula")
	if _, ok := nested[formula]; !ok {
		occurrence := e.occurrence(e.mathIndex + 1 + len(nested) + e.pending.foreign)
		if !rec.Equal(rec.Fetch(occurrence, "formula"), formula) {
			rec.Fail("Reading formula identity mismatch")
		}
		nested[formula] = e.formulaNodes(occurrence)
		e.nestedMath = append(e.nestedMath, rec.Fetch(occurrence, "id"))
	}
	nested[formula]--
	if nested[formula] < 0 {
		rec.Fail("Nested formula exceeds its retained nodes")
	}
}

// skipUnreadMathNodes: a retained formula node the flow did not read (a glyph of a table set
// in math) is still in the formula's MathML; the reading continues at the next node of the
// same fragment.
func (e *Evidence) skipUnreadMathNodes(actual nodeKey) int {
	skipped := 0
	for {
		expected := e.peekNode()
		if sameNode(expected, actual) || !rec.Equal(expected[0], actual[0]) || !rec.Equal(expected[1], actual[1]) ||
			!less(expected[2], actual[2]) {
			break
		}
		e.nextNode()
		skipped++
	}
	if skipped > 0 {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "unread_math_nodes", "formula": actual[0], "nodes": int64(skipped)})
	}
	return skipped
}

func (e *Evidence) startMath(item, owner rec.Map, page any) {
	occurrence := e.occurrence(e.mathIndex)
	if !rec.Equal(rec.Fetch(occurrence, "formula"), rec.Fetch(item, "formula")) {
		rec.Fail("Reading formula identity mismatch")
	}
	definition, ok := e.formulas.Get(rec.Fetch(item, "formula"))
	if !ok {
		panic(rec.Failure{Message: "key not found: " + toS(item["formula"]), Lookup: true})
	}
	display := truthy(rec.Fetch(definition, "display"))
	var block rec.Map
	if display {
		block = e.newBlock("display_math", owner, page)
	} else {
		block = e.inlineMathBlock(owner, page)
	}
	citation := slice(item, citationFields...)
	if space := rec.FetchString(item, "space"); !display && space != "" {
		e.appendText(block, rec.Merge(citation, rec.Map{"text": space, "space": ""}), spanPaint{}, e.ownerStyles[rec.Fetch(owner, "owner")])
	}
	span := rec.Merge(rec.Map{"type": "math", "occurrence": rec.Fetch(occurrence, "id"), "formula": rec.Fetch(item, "formula")},
		citation, slice(item, "external_link"))
	push(block, e.rememberTableSpan(span))
	e.pending = &pendingMath{block: block, remaining: e.formulaNodes(occurrence), citation: citation, cell: e.activeCell,
		table: e.activeTable, externalLink: item["external_link"], formula: rec.Fetch(item, "formula"), nested: map[any]int64{},
		span: lastSpan(block), occurrence: rec.Fetch(occurrence, "id"), marker: slice(item, "note_mark", "note_number")}
	if tag := e.displayTag(rec.Fetch(item, "formula")); display && tag != "" {
		e.pending.tag = e.tagSpan(tag, rec.Fetch(item, "formula"))
		push(block, e.pending.tag)
	}
}

// inlineMathBlock: a formula an hbox holds (a one-line caption typeset in a box) is observed
// as an owner of its own between two runs of the hbox: it belongs to the hbox's open block,
// which the hbox resumes after it.
func (e *Evidence) inlineMathBlock(owner rec.Map, page any) rec.Map {
	part := e.readingPartition(owner)
	last, key := e.lastBlocks[part], e.lastKeys[part]
	if last == nil || rec.Fetch(owner, "group") != "math" || rec.Fetch(last, "type") == "display_math" ||
		!rec.Equal(rec.Fetch(last, "entity"), rec.Fetch(owner, "entity")) {
		return e.proseBlock(owner, page)
	}
	host, found := e.owners.Get(key.owner)
	progress, _ := e.ownerProgress.Get(key.owner)
	if !found || rec.Fetch(host, "group") != "hbox" || progress >= int64(len(rec.FetchList(host, "items"))) {
		return e.proseBlock(owner, page)
	}
	last["pages"] = union(rec.FetchList(last, "pages"), page)
	return last
}

// displayTag is the printed number a flush-left display carries inside its formula, which reads
// as a text span after the formula (mathml.SplitEquationTag).
func (e *Evidence) displayTag(formula any) string {
	if e.payloads == nil {
		e.payloads = map[string]string{}
		for _, record := range e.records {
			if rec.Fetch(record, "kind") == "math" {
				if _, seen := e.payloads[rec.ToS(rec.Fetch(record, "formula"))]; !seen {
					e.payloads[rec.ToS(rec.Fetch(record, "formula"))] = rec.FetchString(record, "mathml")
				}
			}
		}
	}
	_, tag := mathml.SplitEquationTag(e.payloads[rec.ToS(formula)])
	return tag
}

func (e *Evidence) finishCoverage() {
	if e.mathIndex != len(rec.FetchList(e.retention, "occurrences")) || e.pending != nil {
		rec.Fail("Missing mathematics in reading stream")
	}
	e.ownerProgress.Each(func(id any, last int64) {
		owner, _ := e.owners.Get(id)
		size := int64(len(rec.FetchList(owner, "items")))
		if rec.Fetch(owner, "group") != "paragraph" || last == size || e.neverShipped(owner, last+1, size) {
			return
		}
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "incomplete_reading_paragraph", "owner": id})
	})
}
