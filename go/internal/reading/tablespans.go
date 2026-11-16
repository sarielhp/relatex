package reading

import (
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

// Table spans: where a rich table's cell spans and their note calls belong.

// relocateCallsites: a rich table gathers its cells' spans into the block where it starts, after
// the notes resolved their call sites. A call in a cell (an author's \thanks in a title tabular)
// belongs to that block now (third cohort 565).
func relocateCallsites(report rec.Map, blocks []rec.Map) {
	owners := map[any]any{}
	for _, block := range blocks {
		content, _ := block["content"].([]any)
		for _, span := range inline.Deep(content) {
			if span["type"] == "note_reference" && span["id"] != nil {
				owners[span["id"]] = block["id"]
			}
		}
	}
	for _, note := range rec.FetchList(report, "notes") {
		for _, value := range rec.FetchList(rec.AsMap(note), "callsites") {
			callsite := rec.AsMap(value)
			if owner, found := owners[callsite["id"]]; found {
				callsite["block"] = owner
			}
		}
	}
}

func (e *Evidence) tableOrigin() rec.Map {
	if !truthy(e.activeTable) {
		return rec.Map{}
	}
	result := rec.Map{"table": e.activeTable, "copy": e.activeCopy}
	if truthy(e.activeCell) {
		result["cell"] = e.activeCell
	}
	return result
}

func (e *Evidence) sameTableOrigin(span rec.Map) bool {
	origin, ok := e.origins[rec.Identity(span)]
	if !ok {
		origin = rec.Map{}
	}
	return rec.Equal(origin, e.tableOrigin())
}

func (e *Evidence) rememberTableSpan(span rec.Map) rec.Map {
	if truthy(e.activeTable) {
		e.origins[rec.Identity(span)] = e.tableOrigin()
		e.registered = append(e.registered, span)
	}
	return span
}
