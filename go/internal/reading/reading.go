package reading

import (
	"slices"

	"github.com/sarielhp/relatex/internal/language"
	"github.com/sarielhp/relatex/internal/links"
	"github.com/sarielhp/relatex/internal/media"
	"github.com/sarielhp/relatex/internal/notes"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
	"github.com/sarielhp/relatex/internal/structure"
	"github.com/sarielhp/relatex/internal/tables"
	"github.com/sarielhp/relatex/internal/textstyle"
)

var citationFields = []string{"citation", "citation_link", "bibliography_entry"}

// runningSize is a rule dimension TeX left running (the box's own).
const runningSize = -1_073_741_824

var keptDiagnostics = []string{"unclassified_flow_glyph", "unsupported_qed_definition", "unsupported_qed_context",
	"unsupported_caption_counter", "unclassified_running_rule", "unsupported_graphic_source",
	"unsupported_graphic_transform", "unsupported_table_layout", "unsupported_document_page_total",
	"unsupported_list_context", "unsupported_citation_retention",
	"unsupported_citation_hook_context", "unsupported_external_alignment",
	"unclosed_structural_context", "heading_inside_block"}

type ruleKey struct{ owner, slot any }

type nodeKey [3]any

type blockKey struct {
	owner any
	uses  int64
}

// pendingMath is a formula whose retained nodes the reading is still consuming.
type pendingMath struct {
	block             rec.Map
	remaining         int64
	citation          rec.Map
	cell, table       any
	externalLink      any
	formula           any
	nested            map[any]int64
	span              rec.Map
	occurrence        any
	marker            rec.Map
	foreign           int
	citationsUnlinked bool
	// references are the reference spans printed inside this formula, one per source use.
	references  []rec.Map
	referenceAt map[any]int
	// enclosing is the one reference every node of the formula prints in (a formula set wholly
	// inside a \hyperref text), nil when a node prints outside it or in another one.
	enclosing any
	read      bool
	// tag is the display's number span, given its glyphs' style when the formula is read (tagSpan).
	tag rec.Map
}

// enclosingLink: the reference a formula is set wholly inside, and the formula's math span.
type enclosingLink struct {
	reference any
	span      rec.Map
}

// Evidence is ReadingEvidence.
type Evidence struct {
	// tagSpans and tagMath: the text spans and formulas read from tag-marked items (tagmarks.go).
	tagSpans map[uintptr]int
	tagMath  map[string]bool
	// tagStyles: the text style a formula's tag glyphs are set in (item tag_style), by formula id.
	tagStyles   map[string]rec.Map
	records     []rec.Map
	textStyles  textstyle.Catalog
	ownerStyles map[any]rec.Map
	cellStyles  map[string]rec.Map
	// fontStyles: the style of a direct TeX font its items name by ID (item.font).
	fontStyles    map[int64]rec.Map
	pages         *pageEvidence
	retention     rec.Map
	scopes        *scopes
	graphics      *media.Graphics
	rendered      *media.Rendered
	externalLinks map[int64]rec.Map
	droppedLinks  []any
	owners        *omap.Map[any, rec.Map]
	references    *omap.Map[any, rec.Map]
	formulas      *omap.Map[any, rec.Map]
	fragments     *omap.Map[any, rec.Map]
	symbols       *omap.Map[any, rec.Map]
	symbolShapes  *omap.Map[any, []any]
	parents       map[any]any
	rulePaint     map[ruleKey][][]any
	// textPaint is the non-black fill color the shipout recorded for a glyph's flow slot.
	textPaint map[ruleKey]string
	// textLanguage is the BCP 47 tag of a glyph set in a language other than the document's.
	textLanguage map[ruleKey]string
	// mathSpace is the word space ahead of a formula an enclosing box reads again (code math_space).
	mathSpace map[ruleKey]string
	// rowIndents is the leading width of the row of a box that a flow slot opens (rowlines.go).
	rowIndents  map[ruleKey]int64
	mainTag     string
	symbolCount int
	origins     map[uintptr]rec.Map
	// registered keeps every span in origins reachable, so no other map takes its address.
	registered  []rec.Map
	blocks      []rec.Map
	diagnostics []rec.Map

	mathNodes  []nodeKey
	mathNext   int
	mathIndex  int
	pending    *pendingMath
	nestedMath []any
	// mathReferences holds the reference spans each occurrence prints inside its formula.
	mathReferences []rec.Map
	// enclosingLinks: by occurrence, the formulas set wholly inside one reference's text, with the
	// reference and the formula's math span.
	enclosingLinks map[any]enclosingLink
	payloads       map[string]string
	ownerProgress  *omap.Map[any, int64]
	ownerUses      map[any]int64
	referenceCnt   int
	retainedRefs   []any
	targets        []rec.Map
	mathRows       *mathRows
	structure      rec.Map
	notes          *notes.Notes
	noteReport     rec.Map
	lastOwners     map[partition]any
	lastBlocks     map[partition]rec.Map
	lastKeys       map[partition]blockKey
	shipped        map[any][]rec.Map
	formulaMathML  map[any]string
	// printedPages is each shipped page's printed number (\thepage); blockIndex finds a block by id.
	printedPages map[int64]string
	blockIndex   map[any]rec.Map
	// slotBlocks: the block each shipped flow item went into (slots.go).
	slotBlocks map[SlotKey]rec.Map
	// titleSlots: the flow items of run-in titles (runin.go).
	titleSlots map[SlotKey]bool
	// furniture: the candidates set as page decoration (labels.go), read once.
	furniture map[int64]bool
	// headingCounters: the counter of each heading context (labels.go), read once.
	headingCounters map[any]string
	// unowned: the labels the reading found no block for before the tables were projected (targets.go).
	unowned []rec.Map

	activeTable, activeCell any
	activeSlot              int64
	activeCopy              int64
}

// Retain sets the math retention a report is read against (an evidence built with a nil retention waits for it).
func (e *Evidence) Retain(retention rec.Map) *Evidence {
	e.retention = retention
	return e
}

// New is ReadingEvidence.new(records, math_retention, pdf_page_count: proof).
func New(records []rec.Map, retention rec.Map, proof any) *Evidence {
	e := &Evidence{records: records, retention: retention}
	e.textStyles = textstyle.NewCatalog(records)
	e.ownerStyles = unobservedOwnerStyles(records, e.textStyles)
	e.cellStyles = unobservedCellStyles(records, e.ownerStyles)
	e.fontStyles = unobservedFontStyles(records, e.textStyles)
	e.pages = &pageEvidence{records: records, proof: proof}
	e.printedPages = PrintedPages(records)
	e.scopes = newScopes(records)
	e.graphics = media.NewGraphics(records)
	e.rendered = media.NewRendered(records)
	e.externalLinks = links.Catalog(records)
	e.owners = e.inventory("flow_candidate", "owner")
	e.references = e.inventory("reference", "reference")
	e.formulas = e.inventory("math_candidate", "formula")
	e.fragments = e.inventory("math_fragment", "fragment")
	e.symbols = e.inventory("symbol_candidate", "symbol")
	e.symbolShapes = omap.New[any, []any]()
	e.parents = map[any]any{}
	e.rulePaint = map[ruleKey][][]any{}
	e.textPaint = map[ruleKey]string{}
	e.textLanguage = map[ruleKey]string{}
	e.mathSpace = map[ruleKey]string{}
	e.rowIndents = map[ruleKey]int64{}
	e.mainTag = rec.AsString(language.Read(records).Main["tag"])
	for _, row := range records {
		if row["kind"] == "context" {
			e.parents[rec.Fetch(row, "entity")] = rec.Fetch(row, "parent")
		}
		if row["kind"] == "flow_row" {
			e.rowIndents[ruleKey{row["owner"], row["slot"]}] = rec.AsInt(row["indent"])
		}
		if row["code"] == "text_paint" {
			e.recordTextPaint(row)
		}
		if row["code"] == "math_space" {
			e.mathSpace[ruleKey{row["owner"], row["slot"]}] = toS(row["space"])
		}
		if row["code"] == "text_language" {
			e.recordTextLanguage(row)
		}
		if row["code"] == "rule_paint" {
			name := ruleKey{row["owner"], row["slot"]}
			e.rulePaint[name] = append(e.rulePaint[name],
				[]any{row["width"], row["height"], row["depth"], row["color"], row["margin_left"], row["margin_right"]})
		}
	}
	e.origins = map[uintptr]rec.Map{}
	e.blocks = []rec.Map{}
	e.diagnostics = []rec.Map{}
	for _, record := range records {
		if rec.Fetch(record, "kind") == "diagnostic" && slices.Contains(keptDiagnostics, toS(rec.Fetch(record, "code"))) {
			e.diagnostics = append(e.diagnostics, record)
		}
	}
	e.ownerProgress = omap.New[any, int64]()
	e.ownerUses = map[any]int64{}
	e.targets = []rec.Map{}
	e.structure = rec.Map{"status": "unavailable", "nodes": []any{}, "discarded_entities": []any{}}
	if stream.Enabled(records[0], "notes-0.1") {
		e.notes = notes.New(records)
	}
	e.lastOwners = map[partition]any{}
	e.lastBlocks = map[partition]rec.Map{}
	e.lastKeys = map[partition]blockKey{}
	return e
}

// Report is ReadingEvidence#report.
func (e *Evidence) Report() rec.Map {
	header := e.records[0]
	if !stream.Retained(header) {
		return e.result("unavailable")
	}
	protocol := rec.Fetch(header, "protocol")
	if !slices.Contains([]any{"0.3", "0.4", "0.5", "0.6"}, protocol) {
		return e.result("unavailable")
	}
	if rec.Fetch(e.retention, "status") != "verified" {
		return e.result("unresolved")
	}
	e.mathNodes = e.retainedMathNodes()
	for _, record := range e.records {
		if rec.Fetch(record, "kind") == "flow_occurrence" {
			e.observe(record)
		}
	}
	e.finishCoverage()
	e.blocks = attachEquationNumbers(e.blocks, e.isMarkedEquationNumber)
	e.attachFlushLeftNumbers()
	if e.notes == nil {
		e.structure = structure.Report(e.records, e.blocks)
	}
	if protocol == "0.5" || protocol == "0.6" {
		e.mathRows = newMathRows(e.records, rec.FetchList(e.retention, "occurrences"))
	}
	e.resolveTargets()
	if target := e.pages.target(); target != nil {
		e.targets = append(e.targets, target)
	}
	e.diagnostics = append(e.diagnostics, e.pages.diagnostics...)
	e.resolveReferences()
	if e.notes != nil {
		e.noteReport = e.notes.Project(e.blocks)
		e.blocks = e.notes.MainBlocks()
		for _, diagnostic := range rec.FetchList(e.noteReport, "diagnostics") {
			e.diagnostics = append(e.diagnostics, rec.AsMap(diagnostic))
		}
		e.structure = structure.Report(e.notes.MainRecords(), e.blocks)
	}
	e.blocks, e.diagnostics = tables.Project(e.records, e.blocks, e.diagnostics)
	var rich bool
	var dropped []any
	e.blocks, e.diagnostics, e.targets, dropped, rich = tables.ProjectRich(e.records, e.origins, e.blocks, e.diagnostics, e.targets)
	e.nestedMath = append(e.nestedMath, dropped...)
	e.resolveProjectedTargets()
	if rich {
		source := e.records
		if e.notes != nil {
			source = e.notes.MainRecords()
			relocateCallsites(e.noteReport, e.blocks)
		}
		e.structure = structure.Report(source, e.blocks)
	}
	kept := []rec.Map{}
	for _, diagnostic := range e.diagnostics {
		if !e.rendered.Resolved(diagnostic) {
			kept = append(kept, diagnostic)
		}
	}
	e.diagnostics = kept
	if len(e.diagnostics) == 0 {
		return e.result("verified")
	}
	return e.result("unresolved")
}

func (e *Evidence) inventory(kind, key string) *omap.Map[any, rec.Map] {
	found := omap.New[any, rec.Map]()
	for _, record := range e.records {
		if rec.Fetch(record, "kind") != kind {
			continue
		}
		id := rec.Fetch(record, key)
		if found.Has(id) {
			rec.Fail("Duplicate %s identity", kind)
		}
		found.Set(id, record)
	}
	return found
}

func maps(list []rec.Map) []any {
	result := make([]any, len(list))
	for index, item := range list {
		result[index] = item
	}
	return result
}

func (e *Evidence) result(status string) rec.Map {
	rows := []any{}
	if e.mathRows != nil {
		rows = e.mathRows.rowList()
	}
	discardedReferences := []any{}
	for _, id := range e.references.Keys() {
		if !slices.Contains(e.retainedRefs, id) {
			discardedReferences = append(discardedReferences, id)
		}
	}
	discardedSymbols := []any{}
	for _, id := range e.symbols.Keys() {
		if !e.symbolShapes.Has(id) {
			discardedSymbols = append(discardedSymbols, id)
		}
	}
	result := rec.Map{"status": status, "blocks": maps(e.blocks), "targets": maps(e.targets), "math_rows": rows,
		"structure": e.structure, "discarded_references": discardedReferences, "discarded_symbols": discardedSymbols,
		"diagnostics": maps(e.diagnostics)}
	// Declared notes always appear: an early unresolved reading (such as unverified math) keeps an
	// empty unresolved notes report, so consumers fail on the real reason, not a missing key.
	if e.notes != nil {
		result["notes"] = e.noteReport
		if e.noteReport == nil {
			result["notes"] = rec.Map{"status": "unresolved", "notes": []any{}, "discarded_notes": []any{},
				"discarded_marks": []any{}, "diagnostics": []any{}}
		}
	}
	// Formulas printed inside another formula; the outer formula's MathML holds them.
	if len(e.nestedMath) > 0 {
		result["nested_math"] = e.nestedMath
	}
	// References printed inside a formula; its MathML still holds their printed numbers.
	if len(e.mathReferences) > 0 {
		result["math_references"] = maps(e.mathReferences)
	}
	return result
}

func (e *Evidence) retainedMathNodes() []nodeKey {
	nodes := []nodeKey{}
	for _, record := range e.records {
		if rec.Fetch(record, "kind") != "math_occurrence" {
			continue
		}
		for position := rec.FetchInt(record, "first"); position <= rec.FetchInt(record, "last"); position++ {
			nodes = append(nodes, nodeKey{rec.Fetch(record, "formula"), rec.Fetch(record, "fragment"), position})
		}
	}
	return nodes
}

func (e *Evidence) observe(record rec.Map) {
	owner, ok := e.owners.Get(rec.Fetch(record, "owner"))
	if !ok || !less(rec.Fetch(owner, "seq"), rec.Fetch(record, "seq")) {
		rec.Fail("Dangling or premature flow owner")
	}
	first, last := rec.FetchInt(record, "first"), rec.FetchInt(record, "last")
	if !(first > 0 && first <= last && last <= int64(len(rec.FetchList(owner, "items")))) {
		rec.Fail("Invalid retained flow range")
	}
	if e.paintedIntoBlock(owner, first, last) {
		return
	}
	e.trackOwner(owner, first, last)
	if e.foreignToOpenFormula(owner) {
		e.besideOpenFormula(func() { e.observeItems(owner, first, last, record) })
		return
	}
	e.observeItems(owner, first, last, record)
}

func (e *Evidence) observeItems(owner rec.Map, first, last int64, record rec.Map) {
	items := rec.FetchList(owner, "items")[first-1 : last]
	for index, value := range items {
		item := rec.AsMap(value)
		e.activeTable = item["table"]
		e.activeCell = item["table_cell"]
		e.activeSlot = first + int64(index)
		e.activeCopy = e.ownerUses[rec.Fetch(owner, "owner")]
		e.breakRow(owner, e.activeSlot)
		e.observeTitled(item, owner, rec.Fetch(record, "page"))
		e.recordSlot(owner, rec.Fetch(record, "page"))
	}
}

// foreignToOpenFormula: a formula broken across a page or column break stays open while the
// page ships a float set at its top: that flow, in a context outside the formula's, is no
// \text{} of the formula.
func (e *Evidence) foreignToOpenFormula(owner rec.Map) bool {
	if e.pending == nil {
		return false
	}
	formulaEntity := rec.FetchInt(e.pending.block, "entity")
	entity := rec.FetchInt(owner, "entity")
	// Every context descends from the document's: there, a flow with a context of its own is foreign.
	if formulaEntity == 0 {
		return entity != formulaEntity
	}
	for entity > 0 && entity != formulaEntity {
		parent, ok := e.parents[entity]
		if !ok {
			parent = int64(0)
		}
		entity = rec.AsInt(parent)
	}
	return entity != formulaEntity
}

// besideOpenFormula: the foreign flow reads on its own, its formulas after the open
// formula's own occurrences; the open formula then resumes, counting the occurrences the
// foreign flow used.
func (e *Evidence) besideOpenFormula(read func()) {
	suspended := e.pending
	base := e.mathIndex
	first := base + 1 + len(suspended.nested) + suspended.foreign
	e.pending = nil
	e.mathIndex = first
	read()
	if e.pending != nil {
		rec.Fail("Formula left open inside a flow beside an open formula")
	}
	suspended.foreign += e.mathIndex - first
	e.mathIndex = base
	e.pending = suspended
}

// paintedIntoBlock: a box holding only rules (a frame piece copied into each line of a
// listing) paints into the block it ships in; it does not interrupt that paragraph. A paragraph that
// prints such rules between the lines of material nested in it (a listing with a background colour
// inside a box: the box's paragraph holds one rule per listing line) reads them the same way: they
// are painted into the line before, and the paragraph goes on after them without a block of its own.
func (e *Evidence) paintedIntoBlock(owner rec.Map, first, last int64) bool {
	block := e.lastBlocks[e.readingPartition(owner)]
	if block == nil || (rec.Fetch(owner, "group") == "paragraph" && !e.interruptedAt(owner, first)) {
		return false
	}
	items := rec.FetchList(owner, "items")
	sizes := [][]any{}
	all := true
	for slot := first; slot <= last; slot++ {
		item := rec.AsMap(rec.Index(items, int(slot-1)))
		e.activeTable = item["table"]
		e.activeSlot = slot
		size := e.ruleSize(item, owner)
		sizes = append(sizes, size)
		all = all && size != nil
	}
	if !all {
		return false
	}
	for index, size := range sizes {
		e.appendRule(block, rec.AsMap(rec.Index(items, int(first)+index-1)), size)
	}
	if rec.Fetch(owner, "group") == "paragraph" {
		e.ownerProgress.Set(rec.Fetch(owner, "owner"), last)
	}
	return true
}

// interruptedAt: the paragraph owner has read up to slot first-1 and the block read last belongs
// to other material, printed inside it.
func (e *Evidence) interruptedAt(owner rec.Map, first int64) bool {
	id := rec.Fetch(owner, "owner")
	progress, ok := e.ownerProgress.Get(id)
	previous, read := e.lastKeys[e.readingPartition(owner)]
	return ok && read && progress == first-1 && progress < int64(len(rec.FetchList(owner, "items"))) && !rec.Equal(previous.owner, id)
}

// neverShipped: slots no page ever ships (a trial typesetting, as IEEEtran measures a
// biography) are absent, not reordered: the paragraph reads on, reported incomplete.
func (e *Evidence) neverShipped(owner rec.Map, from, to int64) bool {
	if from > to {
		return false
	}
	if e.shipped == nil {
		e.shipped = map[any][]rec.Map{}
		for _, row := range e.records {
			if row["kind"] == "flow_occurrence" {
				id := rec.Fetch(row, "owner")
				e.shipped[id] = append(e.shipped[id], row)
			}
		}
	}
	for _, row := range e.shipped[rec.Fetch(owner, "owner")] {
		if rec.FetchInt(row, "first") <= to && rec.FetchInt(row, "last") >= from {
			return false
		}
	}
	return true
}

func (e *Evidence) readingPartition(owner rec.Map) partition {
	if e.notes != nil {
		if note := e.notes.NoteForEntity(rec.Fetch(owner, "entity")); truthy(note) {
			return partition{"note", note}
		}
	}
	scope, _ := e.scopes.partition(owner)
	return scope
}

func (e *Evidence) trackOwner(owner rec.Map, first, last int64) {
	id := rec.Fetch(owner, "owner")
	part := e.readingPartition(owner)
	if previousID, ok := e.lastOwners[part]; ok && !rec.Equal(previousID, id) {
		previousOwner, _ := e.owners.Get(previousID)
		progress, _ := e.ownerProgress.Get(previousID)
		size := int64(len(rec.FetchList(previousOwner, "items")))
		if rec.Fetch(previousOwner, "group") == "paragraph" && progress != size && !e.neverShipped(previousOwner, progress+1, size) &&
			!e.strayCopy(owner) && !e.strayCopy(previousOwner) {
			e.diagnostics = append(e.diagnostics, rec.Map{"code": "interleaved_reading_paragraph", "owner": previousID})
		}
	}
	e.lastOwners[part] = id
	previous, _ := e.ownerProgress.Get(id)
	if previous == int64(len(rec.FetchList(owner, "items"))) {
		previous = 0
		e.ownerUses[id]++
	}
	if first != previous+1 && rec.Fetch(owner, "group") == "paragraph" && !e.neverShipped(owner, previous+1, first-1) {
		rec.Fail("Missing, reversed, or overlapping paragraph slots")
	}
	e.ownerProgress.Set(id, last)
}
