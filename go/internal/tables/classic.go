// Package tables is the Go port of the table evidence of the reading: TableEvidence (a
// classic tabular, with TableGrid and TableGridShape) and RichTableEvidence (array cells,
// with RichTable and the checks of RichTableLayout). Only a complete, independently
// attributed table replaces the flat rule and text slots of the reading; a table that
// fails reads as text, with a diagnostic, as in the Ruby reference.
package tables

import (
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

const classicSource = "native-classic-alignment"

var (
	snapshotFields = []string{"seq", "kind", "code", "source", "page", "snapshot"}
	presenceFields = []string{"seq", "kind", "code", "source", "page", "table", "glyphs", "rules"}
)

// runningSize is the size TeX gives a running rule dimension.
const runningSize = -1_073_741_824

// classic is TableEvidence.
type classic struct {
	records     []rec.Map
	snapshots   []rec.Map
	presences   []rec.Map
	diagnostics []rec.Map
}

// plan is one snapshot bound to the reading block it replaces.
type plan struct {
	block  rec.Map
	record rec.Map
	glyphs int64
	span   rec.Map
}

// Project is TableEvidence.new(records).project!(blocks, diagnostics): each complete classic
// table replaces its block's content with a table_grid span; any failure leaves the blocks
// as they are and adds an unsupported_table_layout diagnostic.
func Project(records []rec.Map, blocks, diagnostics []rec.Map) (newBlocks, newDiagnostics []rec.Map) {
	e := &classic{records: records, diagnostics: diagnostics}
	for _, row := range records {
		if row["kind"] == "diagnostic" && row["code"] == "table_snapshot" {
			e.snapshots = append(e.snapshots, row)
		}
		if row["kind"] == "diagnostic" && row["code"] == "table_presence" {
			e.presences = append(e.presences, row)
		}
	}
	if message, failed := rescue(func() { e.project(blocks) }); failed {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "unsupported_table_layout", "reason": message})
	}
	return blocks, e.diagnostics
}

func (e *classic) project(blocks []rec.Map) {
	if len(e.snapshots) == 0 && len(e.presences) == 0 {
		return
	}
	if !stream.Enabled(e.records[0], "tables-0.1") {
		rec.Fail("Tables require their declared extension")
	}
	e.unbuilt()
	if len(e.snapshots) == 0 && len(e.presences) == 0 {
		return
	}
	used := []any{}
	plans := make([]plan, 0, len(e.snapshots))
	for _, record := range e.snapshots {
		plans = append(plans, e.plan(record, blocks, &used))
	}
	e.verifyPresence(plans)
	rules := e.verifyRunningRules(plans)
	horizontal := e.horizontalDiagnostics(plans)
	for _, entry := range plans {
		entry.block["content"] = []any{entry.span}
	}
	kept := []rec.Map{}
	for _, row := range e.diagnostics {
		if !containsMap(rules, row) && !containsMap(horizontal, row) {
			kept = append(kept, row)
		}
	}
	e.diagnostics = kept
}

// unbuilt: a package that replaces \@array (arydshln, mdwtab) builds tables the hooks never
// see: no preamble is recorded. Each such table reads as text on its own.
func (e *classic) unbuilt() {
	ids := []any{}
	for _, record := range e.snapshots {
		snapshot := dig(record, "snapshot")
		if !truthy(dig(snapshot, "preamble")) && !containsEql(ids, dig(snapshot, "id")) {
			ids = append(ids, dig(snapshot, "id"))
		}
	}
	for _, id := range ids {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "unsupported_table_layout", "table": id,
			"reason": `a package replacing \@array (arydshln, mdwtab) built it`})
	}
	snapshots := []rec.Map{}
	for _, record := range e.snapshots {
		if !contains(ids, dig(dig(record, "snapshot"), "id")) {
			snapshots = append(snapshots, record)
		}
	}
	presences := []rec.Map{}
	for _, record := range e.presences {
		if !contains(ids, record["table"]) {
			presences = append(presences, record)
		}
	}
	e.snapshots, e.presences = snapshots, presences
}

func provenance(record rec.Map, fields []string) {
	if rec.SameKeys(record, fields) && record["source"] == classicSource && rec.Positive(record["page"]) {
		return
	}
	rec.Fail("Invalid native table provenance")
}

func (e *classic) plan(record rec.Map, blocks []rec.Map, used *[]any) plan {
	provenance(record, snapshotFields)
	reader := newGrid(fetchMap(record, "snapshot"), e.records)
	grid := tablemodel.ValidateShape(reader.decode())
	var owner rec.Map
	for _, row := range e.records {
		if row["kind"] == "flow_candidate" && rec.Equal(row["owner"], reader.owner) {
			owner = row
			break
		}
	}
	if owner == nil || !less(rec.Fetch(owner, "seq"), rec.Fetch(record, "seq")) {
		rec.Fail("Premature table snapshot")
	}
	candidates := []rec.Map{}
	for _, candidate := range blocks {
		if rec.Equal(rec.Fetch(candidate, "owner"), reader.owner) && rec.Equal(rec.Fetch(candidate, "pages"), []any{rec.Fetch(record, "page")}) {
			candidates = append(candidates, candidate)
		}
	}
	var block rec.Map
	for _, candidate := range candidates {
		if !contains(*used, rec.Fetch(candidate, "id")) {
			block = candidate
			break
		}
	}
	if block == nil || !rec.Equal(rec.Fetch(block, "content"), anyList(reader.flatContent())) {
		rec.Fail("Missing or changed table reading body")
	}
	*used = append(*used, rec.Fetch(block, "id"))
	return plan{block: block, record: record, glyphs: reader.slotCount,
		span: rec.Map{"type": "table_grid", "id": "t" + tablemodel.Inspect(int64(len(*used))), "grid": grid}}
}

// groups is the plans by [page, table].
func groups(plans []plan) map[string][]plan {
	result := map[string][]plan{}
	for _, entry := range plans {
		name := key(rec.Fetch(entry.record, "page"), rec.Fetch(fetchMap(entry.record, "snapshot"), "id"))
		result[name] = append(result[name], entry)
	}
	return result
}

func (e *classic) verifyPresence(plans []plan) {
	expected := map[string]string{}
	for name, entries := range groups(plans) {
		var glyphs, rules int64
		for _, entry := range entries {
			glyphs += entry.glyphs
			grid := fetchMap(entry.span, "grid")
			rules += int64(len(tablemodel.FetchList(grid, "horizontal_borders")))
			for _, row := range tablemodel.FetchList(grid, "rows") {
				for _, cell := range rec.AsList(row) {
					rules += int64(len(tablemodel.FetchList(rec.AsMap(cell), "borders")))
				}
			}
		}
		expected[name] = key(glyphs, rules)
	}
	observed := map[string]string{}
	for _, record := range e.presences {
		provenance(record, presenceFields)
		name := key(record["page"], record["table"])
		_, seen := observed[name]
		glyphs, glyphsInt := record["glyphs"].(int64)
		rules, rulesInt := record["rules"].(int64)
		if seen || !isInt(record["table"]) || !rec.Positive(record["table"]) || !glyphsInt || !rulesInt || glyphs < 0 || rules < 0 {
			rec.Fail("Ambiguous native table presence")
		}
		observed[name] = key(glyphs, rules)
	}
	if !sameStrings(observed, expected) {
		rec.Fail("Uncovered native table nodes")
	}
}

func sameStrings(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for name, value := range left {
		if other, ok := right[name]; !ok || other != value {
			return false
		}
	}
	return true
}

func (e *classic) verifyRunningRules(plans []plan) []rec.Map {
	selected := []rec.Map{}
	for _, row := range e.diagnostics {
		if _, ok := row["table"]; ok && row["code"] == "unclassified_running_rule" {
			selected = append(selected, row)
		}
	}
	observed := map[string][]any{}
	for _, row := range selected {
		if row["box_kind"] != "hlist" || !rec.Equal(row["height"], int64(runningSize)) || !rec.Equal(row["depth"], int64(runningSize)) {
			rec.Fail("Unsupported running table border")
		}
		name := key(row["page"], row["table"])
		observed[name] = append(observed[name], values(row, "width", "box_height", "box_depth"))
	}
	expected := map[string][]any{}
	for name, entries := range groups(plans) {
		borders := []any{}
		for _, entry := range entries {
			for _, row := range tablemodel.FetchList(fetchMap(entry.span, "grid"), "rows") {
				for _, cell := range rec.AsList(row) {
					for _, border := range tablemodel.FetchList(rec.AsMap(cell), "borders") {
						borders = append(borders, values(rec.AsMap(border), "width", "height", "depth"))
					}
				}
			}
		}
		expected[name] = borders
	}
	if !sameGroups(observed, expected) {
		rec.Fail("Uncovered running table borders")
	}
	return selected
}

// sameGroups is Ruby's == on two hashes of sorted value lists.
func sameGroups(left, right map[string][]any) bool {
	if len(left) != len(right) {
		return false
	}
	for name, list := range left {
		other, ok := right[name]
		if !ok || !rec.Equal(tablemodel.SortValues(list), tablemodel.SortValues(other)) {
			return false
		}
	}
	return true
}

func (e *classic) horizontalDiagnostics(plans []plan) []rec.Map {
	result := []rec.Map{}
	for _, entry := range plans {
		id := rec.Fetch(entry.block, "id")
		selected := []rec.Map{}
		for _, row := range e.diagnostics {
			if row["code"] == "unsupported_reading_node" && rec.Equal(row["block"], id) {
				selected = append(selected, row)
			}
		}
		if len(selected) != len(tablemodel.FetchList(fetchMap(entry.span, "grid"), "horizontal_borders")) {
			rec.Fail("Uncovered horizontal table borders")
		}
		result = append(result, selected...)
	}
	return result
}

// dig is Hash#dig on one key: nil for a missing key or a nil receiver.
func dig(value any, name string) any {
	if value == nil {
		return nil
	}
	m, ok := value.(rec.Map)
	if !ok {
		rec.Fail("%T does not have #dig method", value)
	}
	return m[name]
}

func containsMap(list []rec.Map, value rec.Map) bool {
	for _, item := range list {
		if rec.Equal(item, value) {
			return true
		}
	}
	return false
}

// containsEql is include? with eql? (Array#uniq's equality): 1 and 1.0 differ.
func containsEql(list []any, value any) bool {
	for _, item := range list {
		if key(item) == key(value) {
			return true
		}
	}
	return false
}
