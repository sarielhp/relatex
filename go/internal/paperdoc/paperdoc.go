// Package paperdoc is the Go port of Relatex::PaperDocument: the general reading-copy IR
// built from an inspected stream (the inspection), with its validation
// (PaperDocumentValidation and DocumentValidation), floats and citation graph.
// Diagnostic status cannot qualify a conversion.
package paperdoc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/sarielhp/relatex/internal/bookindex"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/stylemodel"
	"github.com/sarielhp/relatex/internal/tablemodel"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/authorhints"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mathml"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
	"github.com/sarielhp/relatex/internal/sourcealt"
	"github.com/sarielhp/relatex/internal/tablehints"
)

// Root is the repository root, where the schemas are read; set it before use.
var Root string

// Gaps is PaperDocument::GAPS.
var Gaps = []any{"ordinary_fidelity", "independent_content", "independent_math", "metadata", "assets", "typed_lists",
	"citations", "text_style"}

// degradable: reader retention codes the export degrades instead of stopping on: the
// affected references print as unlinked text, an unmapped glyph as U+FFFD, an unmodeled
// table as its cells' plain text without painted rules. Every other unresolved code still
// stops export.
var degradable = map[string]string{"unresolved_label_owner": "label.unresolved_owner", "ambiguous_label": "label.ambiguous",
	"unresolved_math_row_target": "label.unresolved_math_row",
	"unresolved_reference_use":   "reference.unresolved", "unmapped_reading_glyph": "text.unmapped_glyph",
	"unsupported_table_layout": "table.unmodeled", "unsupported_reading_node": "content.rule_omitted",
	"unclassified_running_rule":         "content.rule_omitted",
	"interleaved_reading_paragraph":     "paragraph.interrupted",
	"incomplete_reading_paragraph":      "text.paragraph_incomplete",
	"reference_inside_math_unsupported": "reference.unlinked_in_math",
	"footnote_call_number_mismatch":     "note.number_differs",
	"footnote_body_number_mismatch":     "note.number_differs",
	"footnote_body_without_call":        "note.uncalled", "footnote_title_note": "note.title_mark",
	"unresolved_footnote_call": "note.unlinked_call",
	"upward_multirow":          "table.span_flattened",
	"multiply_defined_label":   "label.multiply_defined", "clamped_multirow": "table.span_flattened",
	"citation_inside_math": "citation.unlinked_in_math", "unread_math_nodes": "math.nodes_unread",
	"unsupported_qed_definition": "proof.qed_untyped", "unsupported_qed_context": "proof.qed_untyped",
	"unsupported_graphic_source":     "figure.source_unsupported",
	"unsupported_external_link":      "link.unsupported",
	"unclosed_structural_context":    "structure.context_unclosed",
	"heading_inside_block":           "structure.heading_in_block",
	"table_inside_formula":           "table.inside_formula",
	"unsupported_external_alignment": "math.alignment_unrowed",
	"unsupported_list_context":       "list.context_unsupported",
	"unsupported_graphic_transform":  "graphic.transform_unsupported",
	"unverified_page_reference":      "reference.page_unverified"}

const (
	unmapped     = "[unmapped]"
	painted      = "[rule]"
	mediaVersion = "media-1" // MediaAsset::VERSION
)

var (
	ruleFields  = []string{"type", "width_sp", "height_sp", "depth_sp"}
	ruleOptions = []string{"color", "margin_left_sp", "margin_right_sp"}
	// citationFields is PaperCitations::FIELDS.
	citationFields  = []string{"citation", "citation_link", "bibliography_entry"}
	citationSources = []any{"natbib-command-hooks", "latex-command-hooks", "biblatex-command-hooks"}
)

// mathControl is PaperDocument::MATH_CONTROLS.
func mathControl(character rune) bool {
	return character <= 0x08 || character == 0x0B || character == 0x0C || (character >= 0x0E && character <= 0x1F) ||
		(character >= 0x7F && character <= 0x9F)
}

// presentationForm: shaped Arabic glyphs map to presentation forms; text keeps the letters
// they show.
func presentationForm(character rune) bool {
	return (character >= 0xFB50 && character <= 0xFDFF) || (character >= 0xFE70 && character <= 0xFEFE)
}

var schemas sync.Map

// contract reads and caches schemas/NAME.
func contract(name string) rec.Map { return rootFile("schemas", name) }

// rootFile reads and caches the JSON file DIRECTORY/NAME under Root.
func rootFile(directory, name string) rec.Map {
	path := filepath.Join(Root, directory, name)
	if cached, ok := schemas.Load(path); ok {
		return cached.(rec.Map)
	}
	file, err := os.Open(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		rec.Fail("%v", err)
	}
	schemas.Store(path, value)
	return rec.AsMap(value)
}

// builder is one PaperDocument construction.
type builder struct {
	reading     rec.Map
	displayed   map[any]bool
	semantics   rec.Map
	diagnostics *diagnostics.Diagnostics
	citations   rec.Map
	assets      rec.Map
	identity    rec.Map
	nodes       rec.Map
	order       []string
	groupSeen   map[string]bool
	metadata    rec.Map
	referenced  []any
	labelsReady bool
	// alternatives are the authors' figure descriptions from the source (decision 096).
	alternatives *sourcealt.Evidence
	// itemMarkers are the drawn list markers of the reading blocks, by block id (decision 127).
	itemMarkers map[string]rec.Map
}

// New is PaperDocument.new(inspection, identity:, assets:, diagnostics:).data. hints are the
// author's alt hints (decision 122): an image of the figure their label names takes the author's
// text alternative over what the source says.
func New(inspection, identity, assetsData rec.Map, diags *diagnostics.Diagnostics, hints *authorhints.Hints) (data rec.Map) {
	defer incomplete("Incomplete paper reading: ")
	schema.Validate(inspection, contract("inspection.schema.json"), "$")
	b := &builder{displayed: map[any]bool{}, reading: rec.FetchMap(inspection, "reading"), diagnostics: diags, identity: identity,
		nodes: rec.Map{}, groupSeen: map[string]bool{}}
	b.semantics, _ = inspection["semantics"].(rec.Map)
	b.citations = b.citationEvidence(inspection)
	b.alternatives = sourcealt.Read(diagnosticRows(inspection))
	b.verifyRetention(inspection)
	b.assets = assetsData
	if b.assets == nil {
		b.assets = rec.Map{"status": "unverified"}
	}
	b.buildStructure(rec.FetchMap(b.reading, "structure"), "main", nil)
	b.reportDrawnTitles(diagnosticRows(inspection))
	if displaced, _ := rec.FetchMap(b.reading, "structure")["displaced"].([]any); len(displaced) > 0 {
		b.diagnostics.Add("structure.displaced_block", fmt.Sprintf("%d blocks are printed outside the section that owns them; "+
			"they stay where the PDF prints them", len(displaced)), nil)
	}
	for _, block := range rec.FetchList(b.reading, "blocks") {
		b.buildBlock(rec.AsMap(block))
	}
	for _, note := range b.notes() {
		b.buildNote(note)
	}
	b.buildMath(inspection)
	b.buildDocumentPageTotals()
	projectFloats(inspection, b)
	data = b.assemble()
	sourcealt.ApplyHints(data, hints)
	if unmatched := tablehints.Apply(data, hints); len(unmatched) > 0 {
		b.diagnostics.Add("table.hint_unmatched", fmt.Sprintf("table hints name no table of the document (a \\label or the number "+
			"a table's caption prints): %s", strings.Join(unmatched, ", ")), int64(len(unmatched)))
	}
	if proofs := b.inferProofs(toS(data["root"])); proofs > 0 {
		b.diagnostics.Add("structure.proof_inferred", fmt.Sprintf("%d proofs written without an environment (a styled Proof heading and a "+
			"QED mark at a paragraph end) are marked as proofs", proofs), int64(proofs))
	}
	if marked := InferDecorative(b.nodes); marked > 0 {
		b.diagnostics.Add("figure.decorative_inferred", fmt.Sprintf("%d images that show nothing to describe are marked decorative (provenance inferred)", marked),
			int64(marked))
	}
	if named := InferIconAlt(b.nodes); named > 0 {
		b.diagnostics.Add("figure.icon_named", fmt.Sprintf("%d small icons are named by their file names (ORCID iD, Email, URL; provenance inferred)", named),
			int64(named))
	}
	data["language"] = b.language(inspection, data)
	b.pruneUnlinkedCitations()
	if graph := projectCitations(b.citations, b.nodes, rec.FetchList(data, "block_order")); graph != nil {
		data["citations"] = graph
	}
	if links := extlinks.Project(inspection, data, b.uid); links != nil {
		data["external_links"], data["schema_version"], data["profile"] = links, "paper-0.2", "paper-reading-diagnostic-v2"
	}
	if graph, losses := bookindex.Project(inspection, b.uid); graph != nil {
		data["index"] = graph
		for _, loss := range losses {
			b.diagnostics.Add(loss.Code, loss.Message, nil)
		}
	}
	Validate(data)
	return data
}

// incomplete is the rescue of KeyError and TypeError around a construction: a missing field
// is incomplete evidence.
func incomplete(prefix string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(rec.Failure); ok && failure.Lookup {
		panic(rec.Failure{Message: prefix + failure.Message})
	}
	panic(recovered)
}

func (b *builder) assemble() rec.Map {
	mainOrder, noteOrder, blockOrder := []any{}, []any{}, []any{}
	for _, block := range rec.FetchList(b.reading, "blocks") {
		mainOrder = append(mainOrder, b.uid("block", rec.Fetch(rec.AsMap(block), "id")))
	}
	for _, note := range b.notes() {
		noteOrder = append(noteOrder, b.uid("note", rec.Fetch(note, "id")))
	}
	for _, block := range b.allBlocks() {
		blockOrder = append(blockOrder, b.uid("block", rec.Fetch(block, "id")))
	}
	labels := rec.Map{}
	for _, value := range rec.FetchList(b.reading, "targets") {
		target := rec.AsMap(value)
		labels[rec.FetchString(target, "label")] = rec.Map{"target": b.targetID(target), "number": rec.Fetch(target, "number"),
			"status": "verified"}
	}
	metadata := b.metadata
	if metadata == nil {
		metadata = rec.Map{"status": "unavailable"}
	}
	data := rec.Map{"schema_version": "paper-0.1", "profile": "paper-reading-diagnostic-v1",
		"status": "diagnostic", "qualified": false, "identity": b.identity,
		"root": b.uid("main", int64(0)), "nodes": b.nodes, "main_block_order": mainOrder, "note_order": noteOrder,
		"block_order": blockOrder, "labels": labels, "metadata": metadata, "assets": b.assets,
		"unverified_requirements": slices.Clone(Gaps)}
	if b.mediaAssets() || b.richGrids() {
		data["schema_version"], data["profile"] = "paper-0.2", "paper-reading-diagnostic-v2"
	}
	return data
}

func (b *builder) mediaAssets() bool {
	items, _ := b.assets["items"].(rec.Map)
	for _, asset := range items {
		if rec.AsMap(asset)["schema_version"] == mediaVersion {
			return true
		}
	}
	return false
}

func (b *builder) richGrids() bool {
	for _, id := range b.order {
		node := rec.AsMap(b.nodes[id])
		content, _ := node["content"].([]any)
		for _, span := range content {
			if grid, ok := rec.AsMap(span)["grid"].(rec.Map); ok && grid["schema_version"] == tablemodel.Version {
				return true
			}
		}
	}
	return false
}

// uid is PaperDocument#uid: 'n-' + SHA-256 of JSON [input_sha256, scope, key].
func (b *builder) uid(scope string, key any) string { return b.uidOf(scope, key) }

// uidOf is uid with a scope of any type (a note scope is the note's id).
func (b *builder) uidOf(scope, key any) string {
	bytes, err := canonjson.Marshal([]any{rec.Fetch(b.identity, "input_sha256"), scope, key})
	if err != nil {
		rec.Fail("%v", err)
	}
	sum := sha256.Sum256(bytes)
	return "n-" + hex.EncodeToString(sum[:])
}

func (b *builder) notes() []rec.Map {
	result := []rec.Map{}
	for _, note := range rec.FetchList(rec.FetchMap(b.reading, "notes"), "notes") {
		result = append(result, rec.AsMap(note))
	}
	return result
}

func (b *builder) allBlocks() []rec.Map {
	result := []rec.Map{}
	for _, block := range rec.FetchList(b.reading, "blocks") {
		result = append(result, rec.AsMap(block))
	}
	for _, note := range b.notes() {
		for _, block := range rec.FetchList(note, "blocks") {
			result = append(result, rec.AsMap(block))
		}
	}
	return result
}

func (b *builder) store(identifier, kind string, children []any, source string, fields rec.Map) rec.Map {
	if _, ok := b.nodes[identifier]; ok {
		rec.Fail("Duplicate paper node")
	}
	node := rec.Map{"id": identifier, "type": kind, "children": children,
		"provenance": rec.Map{"inspection_key": source, "source_hint": "unavailable"}}
	for key, value := range fields {
		node[key] = value
	}
	b.nodes[identifier] = node
	b.order = append(b.order, identifier)
	return node
}

func (b *builder) buildStructure(structure rec.Map, scope, root any) string {
	if rec.Fetch(structure, "status") != "verified" {
		rec.Fail("Unverified paper structure")
	}
	selected := map[any]rec.Map{}
	nodes := rec.FetchList(structure, "nodes")
	for _, value := range nodes {
		node := rec.AsMap(value)
		selected[rec.Fetch(node, "id")] = node
	}
	if len(selected) != len(nodes) {
		rec.Fail("Duplicate paper structure")
	}
	start := root
	if start == nil {
		start = int64(0)
	}
	return b.buildGroup(selected, start, scope, root, 0)
}

func (b *builder) buildGroup(groups map[any]rec.Map, identifier, scope, noteRoot any, depth int) string {
	key := toS(scope) + "\x00" + toS(identifier)
	if b.groupSeen[key] || depth > 64 {
		rec.Fail("Repeated or excessively deep paper structure")
	}
	b.groupSeen[key] = true
	node, ok := groups[identifier]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + toS(identifier), Lookup: true})
	}
	children := []any{}
	for _, value := range rec.FetchList(node, "content") {
		child := rec.AsMap(value)
		if block, isBlock := child["block"]; isBlock {
			children = append(children, b.uid("block", block))
			continue
		}
		children = append(children, b.buildGroup(groups, rec.Fetch(child, "structure"), scope, noteRoot, depth+1))
	}
	isRoot := noteRoot != nil && rec.Equal(identifier, noteRoot)
	ownID, kind := b.uidOf(scope, identifier), rec.FetchString(node, "role")
	if isRoot {
		ownID, kind = b.uidOf("note", scope), "note"
	}
	fields := b.semanticsFor(identifier, "heading", "theorem")
	if kind == "list_item" {
		fields = b.withItemMarker(fields, node)
	}
	b.store(ownID, kind, children, toS(scope)+"/structure:"+toS(identifier), fields)
	return ownID
}

// semanticsFor: the author's construct for an entity (a run-in heading's command, a
// display's environment), when its kind is one this node can carry.
func (b *builder) semanticsFor(entity any, kinds ...string) rec.Map {
	found, ok := b.semantics[toS(entity)].(rec.Map)
	if ok && slices.Contains(kinds, rec.FetchString(found, "kind")) {
		return rec.Map{"semantics": found}
	}
	return rec.Map{}
}

// withTitle adds a run-in heading's title to its paragraph's semantics (decision 088, amendment
// 1): title_spans, how many of the paragraph's first spans the collector saw set as the title.
// Only a run-in heading's own paragraph carries it (a paragraph of the title alone, all of them).
func withTitle(fields rec.Map, block rec.Map, spans int) rec.Map {
	semantics, _ := fields["semantics"].(rec.Map)
	count, counted := rec.Number(block["title_spans"])
	if semantics == nil || semantics["runin"] != true || block["type"] == "section" || !counted || int(count) < 1 || int(count) > spans {
		return fields
	}
	titled := maps.Clone(semantics)
	titled["title_spans"] = int64(count)
	return rec.Map{"semantics": titled}
}

func (b *builder) buildBlock(block rec.Map) {
	kind := rec.FetchString(block, "type")
	if kind == "section" {
		kind = "heading"
	}
	if kind == "theorem" || kind == "proof" || kind == "footnote" {
		kind = "paragraph"
	}
	content := []any{}
	for _, span := range rec.FetchList(block, "content") {
		content = append(content, b.buildSpan(rec.AsMap(span)))
	}
	fields := withTitle(b.semanticsFor(block["entity"], "heading", "equation"), block, len(content))
	fields["content"] = content
	b.store(b.uid("block", rec.Fetch(block, "id")), kind, mathChildren(content), toS(rec.Fetch(block, "id")), fields)
}

func mathChildren(content []any) []any {
	children := []any{}
	for _, span := range inline.Deep(content) {
		if span["type"] == "math" {
			children = append(children, span["node"])
		}
	}
	return children
}

func (b *builder) targetID(target rec.Map) string {
	if target["kind"] == "document_page_total" {
		return b.uid("document_page_total", rec.Fetch(target, "label"))
	}
	if row, ok := target["row"]; ok {
		return b.uid("row", row)
	}
	return b.uid("block", rec.Fetch(target, "block"))
}

func (b *builder) buildSpan(span rec.Map) rec.Map {
	result := b.buildUnstyledSpan(span)
	if style, ok := span["style"]; ok {
		result["style"] = stylemodel.Validate(style)
	}
	if color, ok := span["color"]; ok && result["type"] != "rule" {
		result["color"] = color
	}
	if lang, ok := span["lang"]; ok {
		result["lang"] = lang
	}
	if name, ok := span["glyph_name"]; ok && result["type"] == "text" {
		result["glyph_name"] = name
	}
	// Text-mode \underline: underlined text, not a formula (relatex-external-math, text_underline).
	if decoration, ok := span["decoration"]; ok && result["type"] == "text" {
		result["decoration"] = decoration
	}
	// The space between two lines of a heading (decision 126) breaks the line in the HTML.
	if span["line_break"] == true && result["type"] == "text" {
		result["line_break"] = true
	}
	if b.citations != nil {
		for _, field := range citationFields {
			if value, ok := span[field]; ok {
				result[field] = value
			}
		}
	}
	if link, ok := span["external_link"]; ok {
		result["external_link"] = b.uid("external_link", link)
	}
	return result
}

var replacements = strings.NewReplacer(unmapped, "�", painted, "")

func normalizedText(text string) string {
	text = replacements.Replace(text)
	if !strings.ContainsFunc(text, presentationForm) {
		return text
	}
	var builder strings.Builder
	for _, character := range text {
		if presentationForm(character) {
			builder.WriteString(norm.NFKC.String(string(character)))
		} else {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func (b *builder) buildUnstyledSpan(span rec.Map) rec.Map {
	switch rec.Fetch(span, "type") {
	case "text":
		return rec.Map{"type": "text", "text": normalizedText(rec.FetchString(span, "text"))}
	case "table_grid":
		return b.buildGrid(span)
	case "image":
		return b.buildImage(span)
	case "symbol":
		return b.buildSymbol(span)
	case "rule":
		return rec.Pick(span, append(slices.Clone(ruleFields), ruleOptions...)...)
	case "math":
		if span["display"] == true {
			b.displayed[rec.Fetch(span, "occurrence")] = true
		}
		math := rec.Map{"type": "math", "node": b.uid("math", rec.Fetch(span, "occurrence"))}
		// A formula wholly inside a \hyperref text is part of that link (decision 076).
		if link, ok := span["link"].(rec.Map); ok {
			math["link_label"], math["link_target"] = rec.Fetch(link, "target"), b.referenceTarget(link)
		}
		return math
	case "reference":
		if unresolvedReference(span) {
			return rec.Map{"type": "text", "text": rec.Fetch(span, "text")}
		}
		return rec.Map{"type": "reference", "text": rec.Fetch(span, "text"), "label": rec.Fetch(span, "target"),
			"target": b.referenceTarget(span)}
	case "note_reference":
		return rec.Map{"type": "note_reference", "text": rec.Fetch(span, "text"), "target": b.uid("note", rec.Fetch(span, "note")),
			"callsite": b.uid("callsite", rec.Fetch(span, "id"))}
	}
	rec.Fail("Unsupported paper inline content")
	return nil
}

func (b *builder) buildGrid(span rec.Map) rec.Map {
	grid := rec.FetchMap(span, "grid")
	if grid["schema_version"] == tablemodel.Version {
		cells := []any{}
		for _, value := range rec.FetchList(grid, "cells") {
			cell := rec.AsMap(value)
			content := []any{}
			for _, item := range rec.FetchList(cell, "content") {
				content = append(content, b.buildSpan(rec.AsMap(item)))
			}
			copied := rec.Merge(cell, rec.Map{"id": b.uid("table_cell", []any{rec.Fetch(span, "id"), rec.Fetch(cell, "id")}), "content": content})
			cells = append(cells, copied)
		}
		grid = rec.Merge(grid, rec.Map{"cells": cells})
	}
	return rec.Map{"type": "table_grid", "id": b.uid("table_grid", rec.Fetch(span, "id")), "grid": grid}
}

// EndOfProof is the name of a QED mark: the symbol span's aria-label and the alt text of an image mark.
const EndOfProof = "End of proof"

// buildSymbol: the pinned open square is its name; an author's QED mark (decision 176) keeps its glyphs' text or
// its image, which carries the mark's name like the glyphs do: the alt text "End of proof" (decision 176,
// amendment 1), provenance inferred, not decorative.
func (b *builder) buildSymbol(span rec.Map) rec.Map {
	symbol := rec.Pick(span, "type", "name")
	if text, ok := span["text"].(string); ok {
		symbol["text"] = normalizedText(text)
	}
	if mark, ok := span["image"].(rec.Map); ok {
		image := b.buildImage(mark)
		for _, key := range []string{"alt", "long_description", "description_details"} {
			delete(image, key)
		}
		image["alt"], image["alt_provenance"] = EndOfProof, Inferred
		symbol["image"] = image
	}
	return symbol
}

func (b *builder) buildImage(span rec.Map) rec.Map {
	placement := rec.Pick(span, "placement", "depth_sp")
	items := rec.FetchMap(b.assets, "items")
	asset, ok := items[rec.FetchString(span, "sha256")].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", span["sha256"]), Lookup: true})
	}
	if _, placed := span["placement"]; asset["schema_version"] != mediaVersion && placed {
		if !mediamodel.SimplePlacement(rec.FetchMap(span, "placement")) {
			rec.Fail("Legacy PNG adapter cannot erase media transforms")
		}
		placement = rec.Map{}
	}
	// The figure keeps where the author included it from: the export names its file after it.
	origin := rec.Map{}
	if source, has := span["filename"]; has {
		origin = rec.Map{"source": source}
		// And where the source includes it (decision 115), so the lint finds it through any macro.
		place, ok := b.alternatives.OriginOf(span["graphic_use"])
		if rendered, isRendered := span["rendered_from"].(rec.Map); isRendered && !ok {
			place, ok = b.alternatives.DrawingOrigin(rendered["graphic"])
		}
		if ok && mediamodel.SafeSourcePath(place.File) && strings.Contains(path.Base(place.File), ".") {
			origin["source_file"], origin["source_line"] = place.File, place.Line
		}
	}
	image := rec.Merge(rec.Map{"type": "image", "id": b.uid("image", rec.Fetch(span, "id")), "asset": rec.Fetch(span, "sha256"),
		"width_sp": rec.Fetch(span, "width_sp"), "height_sp": rec.Fetch(span, "height_sp")}, placement, origin)
	// What the author wrote about it in the source (decision 096): alt, artifact, \Description.
	if description, ok := b.alternatives.For(span["graphic_use"]); ok {
		sourcealt.Apply(image, description)
	} else if from, rendered := span["rendered_from"].(rec.Map); rendered {
		// A drawn picture (TikZ): the keys given to the tikzpicture (decision 111).
		if description, ok := b.alternatives.ForPicture(from["graphic"]); ok {
			sourcealt.Apply(image, description)
		}
	}
	return image
}

// unresolvedReference: a reference the reader could not bind keeps its printed text, unlinked.
func unresolvedReference(span rec.Map) bool {
	for _, key := range []string{"target_block", "target_row", "target_page_total"} {
		if _, ok := span[key]; ok {
			return false
		}
	}
	return true
}

func (b *builder) referenceTarget(span rec.Map) string {
	if total, ok := span["target_page_total"]; ok {
		return b.uid("document_page_total", total)
	}
	if row, ok := span["target_row"]; ok {
		return b.uid("row", row)
	}
	return b.uid("block", rec.Fetch(span, "target_block"))
}

// pruneUnlinkedCitations: the verified citation graph, or nil. Calls typeset inside a
// formula print there unlinked: they leave the citation graph, while the bibliography keeps
// its entries. A call that prints nowhere in the reading (a trial typesetting; or a call kept
// inside a formula's MathML) leaves the graph.
func (b *builder) pruneUnlinkedCitations() {
	if b.citations == nil {
		return
	}
	all := []any{}
	for _, id := range b.order {
		content, _ := rec.AsMap(b.nodes[id])["content"].([]any)
		all = append(all, content...)
	}
	calls, links := map[any]bool{}, map[any]bool{}
	for _, span := range inline.Deep(all) {
		if truthy(span["citation"]) {
			calls[span["citation"]] = true
		}
		if truthy(span["citation_link"]) {
			links[span["citation_link"]] = true
		}
	}
	keep := func(rows []any, seen map[any]bool) []any {
		result := []any{}
		for _, row := range rows {
			if seen[rec.Fetch(rec.AsMap(row), "id")] {
				result = append(result, row)
			}
		}
		return result
	}
	b.citations = rec.Merge(b.citations, rec.Map{"calls": keep(rec.FetchList(b.citations, "calls"), calls),
		"links": keep(rec.FetchList(b.citations, "links"), links)})
}

func truthy(value any) bool { return value != nil && value != false }

func (b *builder) citationEvidence(inspection rec.Map) rec.Map {
	retention, _ := inspection["citation_retention"].(rec.Map)
	events := false
	for _, row := range rec.FetchList(inspection, "diagnostics") {
		events = events || slices.Contains(citationSources, rec.AsMap(row)["source"])
	}
	reason, _ := inspection["citation_degraded"].(string)
	if reason == "" && retention != nil && retention["status"] != "verified" {
		reason = fmt.Sprintf("%d unresolved bibliography numbers", len(rec.FetchList(retention, "diagnostics")))
	}
	if reason == "" && events && retention == nil {
		reason = "citation evidence is incomplete"
	}
	if reason != "" {
		b.diagnostics.Add("citation.unverified", "Citations are plain text: "+reason, nil)
		return nil
	}
	if retention == nil {
		return nil
	}
	warnings, _ := retention["warnings"].([]any)
	repeated := map[string]bool{}
	for _, value := range warnings {
		warning := rec.AsMap(value)
		if rec.Fetch(warning, "code") == "bibliography_key_repeated" {
			key := toS(rec.Fetch(warning, "key"))
			if !repeated[key] {
				b.diagnostics.Add("citation.repeated_key", fmt.Sprintf("Bibliography key %q names more than one item; its citations link to the last, as LaTeX numbers them",
					key), nil)
			}
			repeated[key] = true
			continue
		}
		b.diagnostics.Add("citation.link_number_differs", fmt.Sprintf("Citation link to %s records number %s; the bibliography numbers it %s",
			toS(rec.Fetch(warning, "key")), toS(rec.Fetch(warning, "recorded")), toS(rec.Fetch(warning, "bibliography"))), nil)
	}
	return retention
}

// degradeInvisibleMath: formulas whose boxes the engine could not read into visible nodes (a
// paragraph inside the math box) no longer stop the export; they print from their MathML. It
// reports whether that was the only unresolved math retention code.
func (b *builder) degradeInvisibleMath(math rec.Map) bool {
	rows := rec.FetchList(math, "diagnostics")
	for _, row := range rows {
		if rec.AsMap(row)["code"] != "math_without_visible_nodes" {
			return false
		}
	}
	for _, row := range rows {
		formulas := rec.FetchList(rec.AsMap(row), "formulas")
		b.diagnostics.Add("math.nothing_visible", fmt.Sprintf("%d formulas have no visible nodes to verify; they print from their MathML (%s)",
			len(formulas), formulaList(formulas)), nil)
	}
	return true
}

func formulaList(formulas []any) string {
	parts := make([]string, len(formulas))
	for index, formula := range formulas {
		parts[index] = toS(formula)
	}
	return strings.Join(parts, ", ")
}

func (b *builder) verifyRetention(inspection rec.Map) {
	math := rec.FetchMap(inspection, "math_retention")
	rows := []rec.Map{}
	for _, row := range rec.FetchList(b.reading, "diagnostics") {
		rows = append(rows, rec.AsMap(row))
	}
	reading := rec.Fetch(b.reading, "status") == "verified"
	if !reading {
		reading = true
		for _, row := range rows {
			_, known := degradable[toS(row["code"])]
			reading = reading && known
		}
	}
	mathOK := b.degradeInvisibleMath(math) && rec.Fetch(math, "status") == "verified"
	if !reading || !mathOK {
		all := append(append([]any{}, rec.FetchList(b.reading, "diagnostics")...), rec.FetchList(math, "diagnostics")...)
		bytes, _ := canonjson.Marshal(all)
		rec.Fail("Paper retention unresolved: %s", bytes)
	}
	if rec.Fetch(rec.FetchMap(b.reading, "notes"), "status") != "verified" {
		detail, _ := canonjson.Marshal(rec.Fetch(rec.FetchMap(b.reading, "notes"), "diagnostics"))
		rec.Fail("Paper reading notes must verify before export: %s", detail)
	}
	for _, row := range rows {
		if !plainTableRule(row, rows) {
			b.degrade(row)
		}
	}
}

// plainTableRule: a table read as plain text loses its rules with it; its own diagnostic
// says so.
func plainTableRule(row rec.Map, rows []rec.Map) bool {
	code := rec.Fetch(row, "code")
	table, bound := row["table"]
	if (code != "unsupported_reading_node" && code != "unclassified_running_rule") || !bound {
		return false
	}
	for _, other := range rows {
		if rec.Fetch(other, "code") != "unsupported_table_layout" {
			continue
		}
		if otherTable, has := other["table"]; !has || rec.Equal(otherTable, table) {
			return true
		}
	}
	return false
}

func (b *builder) degrade(row rec.Map) {
	message, location := degradation(row)
	code := degradable[rec.FetchString(row, "code")]
	if code == "" {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", row["code"]), Lookup: true})
	}
	// A label nothing references (a class's FirstPage, LastBibItem) loses nothing visible.
	if code == "label.unresolved_owner" && !includes(b.referencedLabels(), row["label"]) {
		code = "label.unowned_unreferenced"
	}
	b.diagnostics.Add(code, message, nil, location...)
}

func includes(list []any, value any) bool {
	for _, item := range list {
		if rec.Equal(item, value) {
			return true
		}
	}
	return false
}

func (b *builder) referencedLabels() []any {
	if b.labelsReady {
		return b.referenced
	}
	spans := []any{}
	for _, block := range b.allBlocks() {
		spans = append(spans, rec.FetchList(block, "content")...)
	}
	// A reference typeset inside a formula (k_{\ref{c}}) refers to its label as a word's does.
	entries, _ := b.reading["math_references"].([]any)
	for _, entry := range entries {
		spans = append(spans, rec.FetchList(rec.AsMap(entry), "content")...)
	}
	b.referenced = []any{}
	for _, span := range inline.Deep(spans) {
		if span["type"] == "reference" {
			b.referenced = append(b.referenced, span["target"])
		}
	}
	b.labelsReady = true
	return b.referenced
}

func (b *builder) buildDocumentPageTotals() {
	for _, value := range rec.FetchList(b.reading, "targets") {
		target := rec.AsMap(value)
		if target["kind"] != "document_page_total" {
			continue
		}
		id := b.targetID(target)
		fields := rec.Pick(target, "count", "anchor", "pdf_sha256")
		fields["source_label"] = rec.Fetch(target, "label")
		if !hasReferenceLabel(b.nodes, rec.Fetch(target, "label")) {
			fields["content"] = []any{pageTotalReference(target, id)}
		}
		b.store(id, "document_page_total", []any{}, "document-page-total:TotPages", fields)
		root := rec.AsMap(b.nodes[b.uid("main", int64(0))])
		root["children"] = append(rec.FetchList(root, "children"), id)
		b.metadata = rec.Map{"status": "partial", "source_page_total": id}
	}
}

func (b *builder) buildNote(note rec.Map) {
	b.buildStructure(rec.FetchMap(note, "structure"), rec.Fetch(note, "id"), rec.Fetch(note, "entity"))
	for _, block := range rec.FetchList(note, "blocks") {
		b.buildBlock(rec.AsMap(block))
	}
	labels := []any{}
	for _, value := range rec.FetchList(note, "labels") {
		label := rec.AsMap(value)
		labels = append(labels, rec.Merge(label, rec.Map{"block": b.uid("block", rec.Fetch(label, "block"))}))
	}
	callsites := []any{}
	for _, value := range rec.FetchList(note, "callsites") {
		callsite := rec.AsMap(value)
		callsites = append(callsites, rec.Map{"id": b.uid("callsite", rec.Fetch(callsite, "id")),
			"block": b.uid("block", rec.Fetch(callsite, "block"))})
	}
	node, ok := b.nodes[b.uid("note", rec.Fetch(note, "id"))].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: "key not found: note", Lookup: true})
	}
	node["number"], node["source_parent"], node["labels"], node["callsites"] = rec.Fetch(note, "number"), rec.Fetch(note, "parent"),
		labels, callsites
}

// mappedMathML: a glyph from a font without a Unicode mapping (MnSymbol's slots) reaches
// MathML as a control character; it prints as U+FFFD instead.
func (b *builder) mappedMathML(mathml string) string {
	if !strings.ContainsFunc(mathml, mathControl) {
		return mathml
	}
	b.diagnostics.Add("math.unmapped_glyph", "a formula glyph without a Unicode mapping prints as U+FFFD", nil)
	return strings.Map(func(character rune) rune {
		if mathControl(character) {
			return '�'
		}
		return character
	}, mathml)
}

func (b *builder) buildMath(inspection rec.Map) {
	definitions := map[any][]any{}
	for _, value := range rec.FetchList(inspection, "math") {
		entry := rec.AsMap(value)
		formula := rec.Fetch(entry, "formula")
		payload := rec.Fetch(entry, "mathml")
		if !includes(definitions[formula], payload) {
			definitions[formula] = append(definitions[formula], payload)
		}
	}
	rows := map[any][]rec.Map{}
	for _, value := range rec.FetchList(b.reading, "math_rows") {
		row := rec.AsMap(value)
		rows[rec.Fetch(row, "occurrence")] = append(rows[rec.Fetch(row, "occurrence")], row)
	}
	references := map[any][]any{}
	referenceEntries, _ := b.reading["math_references"].([]any)
	for _, value := range referenceEntries {
		entry := rec.AsMap(value)
		references[rec.Fetch(entry, "occurrence")] = rec.FetchList(entry, "content")
	}
	nested, _ := b.reading["nested_math"].([]any)
	seenReferences := map[any]bool{}
	for _, value := range rec.FetchList(rec.FetchMap(inspection, "math_retention"), "occurrences") {
		occurrence := rec.AsMap(value)
		id := rec.Fetch(occurrence, "id")
		if includes(nested, id) {
			continue
		}
		payloads, ok := definitions[rec.Fetch(occurrence, "formula")]
		if !ok {
			panic(rec.Failure{Message: "key not found: " + toS(occurrence["formula"]), Lookup: true})
		}
		if len(payloads) != 1 {
			rec.Fail("Conflicting formula payloads")
		}
		children := []any{}
		for _, row := range rows[id] {
			rowID := b.uid("row", rec.Fetch(row, "id"))
			b.store(rowID, "math_row", []any{}, toS(rec.Fetch(row, "id")), rec.Pick(row, "index", "tag", "mathml_path"))
			children = append(children, rowID)
		}
		formula, _ := mathml.SplitEquationTag(rec.AsString(payloads[0]))
		if b.displayed[id] && !strings.Contains(formula, "display=") {
			formula = strings.Replace(formula, "<math", "<math display=\"block\"", 1)
		}
		fields := rec.Map{"mathml": b.mappedMathML(mathml.CollapseHbar(formula)), "tex": rec.Map{"status": "unavailable"}}
		if content := b.buildMathReferences(references[id], seenReferences); len(content) > 0 {
			fields["content"] = content
		}
		b.store(b.uid("math", id), "math", children, toS(id), fields)
	}
}

// buildMathReferences: the reference spans a formula prints. One source use is one span,
// even when a nested formula's inner and outer occurrences both read its glyphs; an
// unresolved use keeps its unlinked number and stays out of the document.
func (b *builder) buildMathReferences(spans []any, seen map[any]bool) []any {
	content := []any{}
	for _, value := range spans {
		span := rec.AsMap(value)
		reference := rec.Fetch(span, "reference")
		if seen[reference] {
			continue
		}
		seen[reference] = true
		built := b.buildSpan(span)
		if built["type"] != "reference" {
			continue
		}
		content = append(content, built)
	}
	return content
}

// toS is Ruby's to_s on a JSON value.
func toS(value any) string { return rec.ToS(value) }

var sha256Hex = regexp.MustCompile(`\A[0-9a-f]{64}\z`)
