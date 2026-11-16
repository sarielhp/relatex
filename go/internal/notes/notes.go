// Package notes is the Go port of Relatex::NoteEvidence: native footnote links resolved and
// retained content partitioned into the main text and the notes, without sorting it.
package notes

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
	"github.com/sarielhp/relatex/internal/structure"
)

const source = "latex-lab-footnote-sockets"

var unmarkedNumbers = []string{"", `\@thefnmark`, `\relax`}

// degradable: the note is complete and linked; only its printed mark or its call is unverified.
var degradable = []string{"footnote_call_number_mismatch", "footnote_body_number_mismatch", "footnote_body_without_call",
	"footnote_title_note", "unresolved_footnote_call"}

var fields = map[string][]string{
	"note_mark": {"seq", "kind", "code", "source", "mark", "number", "label"},
	"note_body": {"seq", "kind", "code", "source", "note", "entity", "parent", "number", "marks"},
}

// view is one note body's projection: the map the report carries and its blocks.
type view struct {
	data      rec.Map
	blocks    []rec.Map
	labels    []any
	callsites []any
}

// Notes is NoteEvidence.
type Notes struct {
	records     []rec.Map
	contexts    map[int64]rec.Map
	depth       map[int64]int64
	marks       *omap.Map[int64, rec.Map]
	bodies      *omap.Map[int64, rec.Map]
	entities    map[int64]int64
	links       map[int64]int64
	diagnostics []any
	callCount   int
	owners      map[any]rec.Map
	views       *omap.Map[int64, *view]
	mainBlocks  []rec.Map
	formulaText map[any]string
}

// New is NoteEvidence.new(records).
func New(records []rec.Map) *Notes {
	if !stream.Enabled(records[0], "notes-0.1") {
		rec.Fail("Notes require their declared extension")
	}
	n := &Notes{records: records, contexts: map[int64]rec.Map{0: {"role": "document", "parent": int64(0)}},
		depth: map[int64]int64{0: 0}, marks: omap.New[int64, rec.Map](), bodies: omap.New[int64, rec.Map](),
		entities: map[int64]int64{}, links: map[int64]int64{}, diagnostics: []any{}, owners: map[any]rec.Map{}}
	for _, record := range records {
		if rec.Fetch(record, "kind") == "context" {
			n.context(record)
		}
	}
	for _, record := range records {
		if rec.Fetch(record, "kind") == "flow_candidate" {
			n.owners[rec.Fetch(record, "owner")] = record
		}
	}
	n.inventory()
	return n
}

// NoteForEntity is note_for_entity: the note body whose footnote context holds entity, or nil.
func (n *Notes) NoteForEntity(entity any) any {
	current := rec.AsInt(entity)
	for current != 0 {
		context, ok := n.contexts[current]
		if !ok {
			rec.Fail("Unknown note content context")
		}
		if rec.Fetch(context, "role") == "footnote" {
			note, found := n.entities[current]
			if !found {
				rec.Fail("Undeclared footnote context")
			}
			return note
		}
		current = rec.FetchInt(context, "parent")
	}
	return nil
}

// MainRecords is main_records: the records without the contexts inside notes.
func (n *Notes) MainRecords() []rec.Map {
	result := []rec.Map{}
	for _, record := range n.records {
		if rec.Fetch(record, "kind") == "context" && n.NoteForEntity(rec.Fetch(record, "entity")) != nil {
			continue
		}
		result = append(result, record)
	}
	return result
}

// MainBlocks is main_blocks: the blocks outside notes, after Project.
func (n *Notes) MainBlocks() []rec.Map { return n.mainBlocks }

// Project is project(blocks): the note report. Note-number spans leave their blocks, call
// sites are linked to their notes in place.
func (n *Notes) Project(blocks []rec.Map) rec.Map {
	n.views = omap.New[int64, *view]()
	n.bodies.Each(func(id int64, body rec.Map) {
		data := rec.Map{"id": id}
		for _, key := range []string{"entity", "parent", "number"} {
			if value, ok := body[key]; ok {
				data[key] = value
			}
		}
		n.views.Set(id, &view{data: data, blocks: []rec.Map{}, labels: []any{}, callsites: []any{}})
	})
	n.partitionBlocks(blocks)
	for _, block := range blocks {
		n.resolveCallsites(block)
	}
	retained := []*view{}
	n.views.Each(func(_ int64, v *view) {
		v.data["blocks"] = asList(v.blocks)
		v.data["labels"] = v.labels
		v.data["callsites"] = v.callsites
		if len(v.blocks) > 0 {
			retained = append(retained, v)
		}
	})
	used := []int64{}
	retainedIDs := []int64{}
	notesList := []any{}
	for _, v := range retained {
		n.validateBody(v)
		printNumber(v.data)
		retainedIDs = append(retainedIDs, rec.FetchInt(v.data, "id"))
		notesList = append(notesList, v.data)
		for _, call := range v.callsites {
			used = append(used, rec.FetchInt(rec.AsMap(call), "mark"))
		}
	}
	verified := true
	for _, row := range n.diagnostics {
		verified = verified && slices.Contains(degradable, rec.FetchString(rec.AsMap(row), "code"))
	}
	status := "unresolved"
	if verified {
		status = "verified"
	}
	return rec.Map{"status": status, "notes": notesList, "discarded_notes": without(n.bodies.Keys(), retainedIDs),
		"discarded_marks": without(n.marks.Keys(), used), "diagnostics": n.diagnostics}
}

func asList(blocks []rec.Map) []any {
	result := make([]any, len(blocks))
	for index, block := range blocks {
		result[index] = block
	}
	return result
}

// without is Ruby's Array#-.
func without(keys, removed []int64) []any {
	result := []any{}
	for _, key := range keys {
		if !slices.Contains(removed, key) {
			result = append(result, key)
		}
	}
	return result
}

func (n *Notes) partitionBlocks(blocks []rec.Map) {
	n.mainBlocks = []rec.Map{}
	for _, block := range blocks {
		note := n.NoteForEntity(rec.Fetch(block, "entity"))
		if note != nil {
			v := n.view(note)
			v.blocks = append(v.blocks, block)
		} else {
			n.mainBlocks = append(n.mainBlocks, block)
		}
		n.collectLabels(block, note)
	}
}

func (n *Notes) view(note any) *view {
	id, _ := note.(int64)
	v, ok := n.views.Get(id)
	if !ok {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %v", note), Lookup: true})
	}
	return v
}

func (n *Notes) body(note any) rec.Map {
	id, _ := note.(int64)
	body, ok := n.bodies.Get(id)
	if !ok {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %v", note), Lookup: true})
	}
	return body
}

func (n *Notes) owner(block rec.Map) rec.Map {
	id := rec.Fetch(block, "owner")
	owner, ok := n.owners[id]
	if !ok {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %v", id), Lookup: true})
	}
	return owner
}

func (n *Notes) context(record rec.Map) {
	id := rec.AsInt(record["entity"])
	parent, isInt := record["parent"].(int64)
	_, seen := n.contexts[id]
	parentContext, known := n.contexts[parent]
	if !(id > 0 && !seen && isInt && known && rec.Fetch(parentContext, "role") != "paragraph") {
		rec.Fail("Invalid note context ancestry")
	}
	n.depth[id] = n.depth[parent] + 1
	if n.depth[id] > 64 {
		rec.Fail("Note context depth limit")
	}
	n.contexts[id] = record
}

func (n *Notes) inventory() {
	expectedSource := source
	if n.records[0]["adapter"] == "external" {
		expectedSource = "ordinary-footnote-hooks"
	}
	for _, record := range n.records {
		code, _ := record["code"].(string)
		if _, known := fields[code]; !known || rec.Fetch(record, "kind") != "diagnostic" {
			continue
		}
		// A body set in \maketitle says so (title: true).
		title := code == "note_body" && record["title"] == true
		keys := rec.Map{}
		for key, value := range record {
			if !(title && key == "title") {
				keys[key] = value
			}
		}
		_, isString := record["number"].(string)
		if !rec.SameKeys(keys, fields[code]) || rec.Fetch(record, "source") != expectedSource || !isString {
			rec.Fail("Invalid native footnote provenance")
		}
		if code == "note_mark" {
			n.mark(record)
		} else {
			n.addBody(record)
		}
	}
	n.bodies.Each(func(id int64, body rec.Map) {
		for _, value := range rec.FetchList(body, "marks") {
			mark := value.(int64)
			markRecord, ok := n.marks.Get(mark)
			_, linked := n.links[mark]
			if !ok || rec.FetchInt(markRecord, "seq") >= rec.FetchInt(body, "seq") || linked {
				rec.Fail("Dangling, premature, or multiply owned native footnote mark")
			}
			n.links[mark] = id
		}
	})
}

func (n *Notes) mark(record rec.Map) {
	id, isInt := record["mark"].(int64)
	if !isInt || id <= 0 || n.marks.Has(id) {
		rec.Fail("Invalid or duplicate footnote mark identity")
	}
	if _, ok := record["label"].(string); !ok {
		rec.Fail("Invalid footnote mark label")
	}
	n.marks.Set(id, record)
}

func (n *Notes) addBody(record rec.Map) {
	id, isInt := record["note"].(int64)
	entity, entityInt := record["entity"].(int64)
	context, known := n.contexts[entity]
	_, seenEntity := n.entities[entity]
	valid := isInt && id > 0 && !n.bodies.Has(id) && !(entityInt && seenEntity) && entityInt && known &&
		rec.Fetch(context, "role") == "footnote" && rec.Equal(rec.Fetch(context, "parent"), record["parent"]) &&
		rec.FetchInt(context, "seq") < rec.FetchInt(record, "seq")
	// lualibs encodes an empty Lua sequence as {}: an unmarked body, such as a \thanks note.
	if marks, ok := record["marks"].(rec.Map); ok && len(marks) == 0 {
		merged := rec.Map{}
		for key, value := range record {
			merged[key] = value
		}
		merged["marks"] = []any{}
		record = merged
	}
	marks, isList := record["marks"].([]any)
	valid = valid && isList && validMarks(marks)
	if !valid {
		rec.Fail("Invalid or duplicate native footnote body")
	}
	n.bodies.Set(id, record)
	n.entities[entity] = id
}

func validMarks(marks []any) bool {
	seen := map[int64]bool{}
	for _, value := range marks {
		mark, ok := value.(int64)
		if !ok || mark <= 0 || seen[mark] {
			return false
		}
		seen[mark] = true
	}
	return true
}

func (n *Notes) collectLabels(block rec.Map, note any) {
	content := rec.FetchList(block, "content")
	kept := content[:0:0]
	for _, value := range content {
		span := rec.AsMap(value)
		if rec.Fetch(span, "type") != "note_number" {
			kept = append(kept, span)
			continue
		}
		valid := rec.Equal(note, rec.Fetch(span, "note")) &&
			rec.FetchInt(n.body(note), "seq") < rec.FetchInt(n.owner(block), "seq")
		if !valid {
			rec.Fail("Foreign or premature footnote body label")
		}
		label := rec.Map{"block": rec.Fetch(block, "id")}
		for _, key := range []string{"text", "style"} {
			if value, ok := span[key]; ok {
				label[key] = value
			}
		}
		v := n.view(note)
		v.labels = append(v.labels, label)
	}
	block["content"] = kept
}

func (n *Notes) resolveCallsites(block rec.Map) {
	calls := []rec.Map{}
	for _, value := range rec.FetchList(block, "content") {
		if span := rec.AsMap(value); rec.Fetch(span, "type") == "note_reference" {
			calls = append(calls, span)
		}
	}
	for _, span := range calls {
		mark, ok := n.markOf(rec.Fetch(span, "mark"))
		if !ok || rec.FetchInt(mark, "seq") >= rec.FetchInt(n.owner(block), "seq") {
			rec.Fail("Dangling or premature footnote call site")
		}
		n.callCount++
		span["id"] = fmt.Sprintf("fn%d", n.callCount)
		note, expected := n.target(mark)
		if note == nil || len(n.view(note).blocks) == 0 {
			// A call whose note text never comes as a footnote (REVTeX footinbib sets it in
			// the bibliography) prints its mark, unlinked.
			n.diagnostics = append(n.diagnostics, rec.Map{"code": "unresolved_footnote_call", "mark": rec.Fetch(span, "mark")})
			replaceWithText(span)
			continue
		}
		v := n.view(note)
		if !evidence.SameMark(rec.Strip(rec.FetchString(span, "text")), evidence.LiteralNumber(rec.Strip(expected))) {
			n.diagnostics = append(n.diagnostics, rec.Map{"code": "footnote_call_number_mismatch", "mark": rec.Fetch(span, "mark")})
		}
		span["note"] = note
		span["target_block"] = rec.Fetch(v.blocks[0], "id")
		v.callsites = append(v.callsites, rec.Map{"id": span["id"], "mark": rec.Fetch(span, "mark"), "block": rec.Fetch(block, "id")})
	}
}

func (n *Notes) markOf(value any) (rec.Map, bool) {
	id, ok := value.(int64)
	if !ok {
		return nil, false
	}
	return n.marks.Get(id)
}

// replaceWithText is span.replace(span.slice(...).merge('type' => 'text')): the same map,
// so the block keeps it.
func replaceWithText(span rec.Map) {
	kept := rec.Map{"type": "text"}
	for _, key := range []string{"text", "style", "citation", "citation_link", "bibliography_entry"} {
		if value, ok := span[key]; ok {
			kept[key] = value
		}
	}
	for key := range span {
		delete(span, key)
	}
	for key, value := range kept {
		span[key] = value
	}
}

func (n *Notes) target(mark rec.Map) (any, string) {
	id := rec.FetchInt(mark, "mark")
	if rec.FetchString(mark, "label") == "" {
		if note, ok := n.links[id]; ok {
			return note, rec.FetchString(mark, "number")
		}
		return nil, rec.FetchString(mark, "number")
	}
	labels := []rec.Map{}
	for _, record := range n.records {
		if rec.Fetch(record, "kind") == "label" && rec.Equal(rec.Fetch(record, "name"), rec.Fetch(mark, "label")) {
			labels = append(labels, record)
		}
	}
	if len(labels) != 1 {
		return nil, ""
	}
	label := labels[0]
	note := n.NoteForEntity(rec.Fetch(label, "entity"))
	if linked, ok := n.links[id]; ok && !rec.Equal(linked, note) {
		rec.Fail("Conflicting native footnote target")
	}
	return note, rec.AsString(rec.Fetch(label, "number"))
}

// unmarked: a class title note (\thanks in amsart, IEEEtran, ...) prints no mark, no number
// and no call: no mark executed, no label retained, no call site, and an empty or unexpanded
// mark number. A mark executed with a blank number (\thanks under ICASSP's \name) prints
// nothing either.
func unmarked(v *view) bool {
	return len(v.labels) == 0 && len(v.callsites) == 0 && blankMark(rec.FetchString(v.data, "number"))
}

// blankMark: a mark that prints nothing (A&A's \protect $\relax $ for an offprint note) has
// no call: nothing is left once \protect, \relax, dollars, braces and spaces are removed.
func blankMark(number string) bool {
	if slices.Contains(unmarkedNumbers, rec.Strip(number)) {
		return true
	}
	for index := 0; index < len(number); index++ {
		if skip := silentCommand(number[index:]); skip > 0 {
			index += skip - 1
			continue
		}
		if !strings.ContainsRune("$ \t\n\v\f\r{}", rune(number[index])) {
			return false
		}
	}
	return true
}

// silentCommand is the length of \protect or \relax (not followed by a letter) at the start.
func silentCommand(text string) int {
	for _, name := range []string{`\protect`, `\relax`} {
		if !strings.HasPrefix(text, name) {
			continue
		}
		if len(text) == len(name) || !isLetter(text[len(name)]) {
			return len(name)
		}
	}
	return 0
}

func isLetter(c byte) bool { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') }

// printedMark: a body mark the class prints without the footnote hooks (\thanks in
// \maketitle) is not a tagged label, but the body still begins with exactly that mark.
func (n *Notes) printedMark(v *view, expected *string) bool {
	if len(v.labels) != 0 || expected == nil || *expected == "" {
		return false
	}
	var text strings.Builder
	if len(v.blocks) > 0 {
		for _, value := range rec.FetchList(v.blocks[0], "content") {
			span := rec.AsMap(value)
			if kind := rec.Fetch(span, "type"); kind != "text" && kind != "math" {
				break
			}
			text.WriteString(n.spanText(span))
		}
	}
	printed := strings.ReplaceAll(rec.LStrip(text.String()), "∗", "*")
	return strings.HasPrefix(printed, strings.ReplaceAll(*expected, "∗", "*"))
}

// spanText: a mark the class sets in math ($^{*}$, article's \maketitle under some fonts)
// reads as its formula's characters.
func (n *Notes) spanText(span rec.Map) string {
	if rec.Fetch(span, "type") != "math" {
		return rec.FetchString(span, "text")
	}
	if n.formulaText == nil {
		n.formulaText = map[any]string{}
		for _, record := range n.records {
			if rec.Fetch(record, "kind") == "math" {
				n.formulaText[rec.Fetch(record, "formula")] = RemoveSpace(MathMLText(rec.FetchString(record, "mathml")))
			}
		}
	}
	return n.formulaText[rec.Fetch(span, "formula")]
}

func (n *Notes) validateBody(v *view) {
	id := rec.FetchInt(v.data, "id")
	isUnmarked := unmarked(v)
	expected := evidence.LiteralNumber(rec.Strip(rec.FetchString(v.data, "number")))
	labelled := len(v.labels) == 1 && evidence.SameMark(rec.Strip(rec.FetchString(rec.AsMap(v.labels[0]), "text")), expected)
	if !isUnmarked && !labelled && !n.printedMark(v, expected) {
		// Two labels mean repeated body content, not a misread number.
		code := "footnote_body_number_mismatch"
		if len(v.labels) > 1 {
			code = "footnote_body_repeated_label"
		}
		n.diagnostics = append(n.diagnostics, rec.Map{"code": code, "note": id})
	}
	if len(v.callsites) == 0 && !isUnmarked {
		// A title note's mark is printed by the class itself, in the author line (\author[a,1]).
		code := "footnote_body_without_call"
		if title := n.body(id)["title"]; title != nil && title != false {
			code = "footnote_title_note"
		}
		n.diagnostics = append(n.diagnostics, rec.Map{"code": code, "note": id})
	}
	v.data["structure"] = structure.Report(n.bodyRecords(v, id), v.blocks)
}

// bodyRecords: the records a note's structure reads, without the contexts of other notes
// and of the main text outside the note's ancestry.
func (n *Notes) bodyRecords(v *view, id int64) []rec.Map {
	contexts := map[int64]bool{}
	starts := []int64{}
	for _, block := range v.blocks {
		starts = append(starts, rec.FetchInt(block, "entity"))
	}
	starts = append(starts, rec.FetchInt(v.data, "entity"))
	for _, entity := range starts {
		contexts[entity] = true
	}
	for _, entity := range starts {
		for entity != 0 {
			context, ok := n.contexts[entity]
			if !ok {
				rec.Fail("key not found: %d", entity)
			}
			entity = rec.FetchInt(context, "parent")
			contexts[entity] = true
		}
	}
	records := []rec.Map{}
	for _, record := range n.records {
		if rec.Fetch(record, "kind") == "context" {
			entity := rec.Fetch(record, "entity")
			if number, ok := entity.(int64); !(ok && contexts[number]) && !rec.Equal(n.NoteForEntity(entity), id) {
				continue
			}
		}
		records = append(records, record)
	}
	return records
}

// printNumber: once the body is validated against the recorded number, the reported number is
// the mark it prints, not the TeX the collector recorded (MNRAS's \thanks star
// \protect \mbox {{$…{^{\star }}…$}} prints ⋆; an amsart title note's \@thefnmark prints nothing).
func printNumber(data rec.Map) {
	if number, ok := data["number"].(string); ok {
		data["number"] = evidence.PrintedMark(number)
	}
}
