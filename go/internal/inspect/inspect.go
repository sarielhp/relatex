// Package inspect is the Go port of Relatex::Stream#inspect_document: the reconciled view
// of a validated record stream, with its math, reading, semantic and citation evidence and
// every diagnostic the stream and the evidence raised.
//
// It is in the core layer (decision 102): the reconciliation stage (papersource) builds the
// inspection with it from the records; no output reads it.
package inspect

import (
	"fmt"
	"github.com/sarielhp/relatex/internal/command"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/reading"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

// Document is Stream#inspect_document; proof is the pdf_page_count proof (nil without one).
func Document(records []rec.Map, proof any) rec.Map {
	document, _ := Read(records, proof)
	return document
}

// Read is Document with the reading blocks by the flow items they hold (reading.SlotKey), which
// the tagged PDF binds glyphs by (decision 105).
func Read(records []rec.Map, proof any) (rec.Map, map[reading.SlotKey]string) {
	candidates := omap.New[any, rec.Map]()
	for _, record := range records {
		if rec.Fetch(record, "kind") == "candidate" {
			candidates.Set(rec.Fetch(record, "candidate"), record)
		}
	}
	// The reports below read the records and nothing else of each other: they run beside the
	// reconciliation, and a failure is raised in the order the reports are used.
	var retention rec.Map
	joinRetention := beside(func() { retention = evidence.MathReport(records) })
	// The reading's inventories need the records alone; its report waits for the retention.
	var observed *reading.Evidence
	joinObserved := beside(func() { observed = reading.New(records, nil, proof) })
	var citations rec.Map
	var citationFailure string
	joinCitations := beside(func() { citations, citationFailure = citationReport(records) })
	var semantics any
	joinSemantics := beside(func() { semantics = evidence.SemanticReport(records) })
	occurrences := reconcile(records, candidates)
	command.Trace("inspect: occurrences")
	retained := map[any]bool{}
	for _, occurrence := range occurrences {
		retained[rec.Fetch(occurrence, "candidate")] = true
	}
	discarded := []any{}
	for _, id := range candidates.Keys() {
		if !retained[id] {
			discarded = append(discarded, id)
		}
	}
	joinRetention()
	command.Trace("inspect: math retention")
	// Citation evidence that cannot be verified does not stop the reading: the export keeps
	// the citation text and reports why it is not linked.
	joinCitations()
	command.Trace("inspect: citations")
	joinObserved()
	report := observed.Retain(retention).Report()
	joinSemantics()
	command.Trace("inspect: reading")
	document := rec.Map{"schema_version": "0.6", "status": "experimental",
		"order_basis": "retained traversal; simple single-column profile",
		"blocks":      list(occurrences), "discarded_candidates": discarded,
		"labels": kinds(records, "label"), "math": kinds(records, "math"), "math_retention": retention,
		"reading": report, "semantics": semantics}
	issues := diagnostics(records, candidates, occurrences, retention)
	document["diagnostics"] = append(issues, rec.FetchList(report, "diagnostics")...)
	if rec.Fetch(citations, "status") != "unavailable" {
		document["citation_retention"] = citations
	}
	if citationFailure != "" {
		document["citation_degraded"] = citationFailure
	}
	if stream.Enabled(records[0], "external-links-0.1") {
		document["external_link_collection"] = "hyperref-execution-1"
	}
	return document, observed.SlotBlocks()
}

// beside runs work in its own goroutine; the returned function waits for it and raises its panic.
func beside(work func()) func() {
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		work()
	}()
	return func() {
		if failure := <-done; failure != nil {
			panic(failure)
		}
	}
}

// citationReport is CitationEvidence.new(records).report, an unavailable report with the
// failure's message when it fails (Relatex::Failure only: a lookup failure stops the stage).
func citationReport(records []rec.Map) (report rec.Map, failure string) {
	defer degraded(&report, &failure)
	return evidence.CitationReport(records), ""
}

// degraded is citationReport's rescue of Relatex::Failure.
func degraded(report *rec.Map, failure *string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	caught, ok := recovered.(rec.Failure)
	if !ok || caught.Lookup {
		panic(recovered)
	}
	*report, *failure = rec.Map{"status": "unavailable"}, caught.Message
}

func kinds(records []rec.Map, kind string) []any {
	result := []any{}
	for _, record := range records {
		if rec.Fetch(record, "kind") == kind {
			result = append(result, record)
		}
	}
	return result
}

func list(maps []rec.Map) []any {
	result := make([]any, len(maps))
	for index, item := range maps {
		result[index] = item
	}
	return result
}

func reconcile(records []rec.Map, candidates *omap.Map[any, rec.Map]) []rec.Map {
	result := []rec.Map{}
	for _, record := range records {
		if rec.Fetch(record, "kind") != "occurrence" {
			continue
		}
		candidate, ok := candidates.Get(rec.Fetch(record, "candidate"))
		if !ok {
			panic(rec.Failure{Message: fmt.Sprintf("key not found: %v", record["candidate"]), Lookup: true})
		}
		first, last := rec.FetchInt(record, "first"), rec.FetchInt(record, "last")
		if len(result) > 0 {
			previous := result[len(result)-1]
			if rec.Equal(rec.Fetch(previous, "candidate"), rec.Fetch(record, "candidate")) && rec.FetchInt(previous, "last") < first {
				previous["last"] = last
				previous["seen_glyphs"] = rec.FetchInt(previous, "seen_glyphs") + last - first + 1
				previous["pages"] = union(rec.FetchList(previous, "pages"), rec.Fetch(record, "page"))
				continue
			}
		}
		result = append(result, rec.Map{"id": fmt.Sprintf("o%d", len(result)+1), "candidate": rec.Fetch(candidate, "candidate"),
			"entity": rec.Fetch(candidate, "entity"), "type": rec.Fetch(candidate, "role"),
			"text": rec.Strip(rec.FetchString(candidate, "text")), "first": first, "last": last,
			"seen_glyphs": last - first + 1, "pages": []any{rec.Fetch(record, "page")}})
	}
	return result
}

func union(list []any, value any) []any {
	for _, item := range list {
		if rec.Equal(item, value) {
			return list
		}
	}
	return append(list, value)
}

func diagnostics(records []rec.Map, candidates *omap.Map[any, rec.Map], occurrences []rec.Map, retention rec.Map) []any {
	issues := []any{}
	math := false
	for _, record := range records {
		kind := rec.Fetch(record, "kind")
		math = math || kind == "math"
		if kind == "diagnostic" && rec.Fetch(record, "code") != "math_choice_discard" {
			issues = append(issues, record)
		}
	}
	for _, occurrence := range occurrences {
		candidate, _ := candidates.Get(rec.Fetch(occurrence, "candidate"))
		if !rec.Equal(rec.Fetch(occurrence, "seen_glyphs"), rec.Fetch(candidate, "glyphs")) {
			issues = append(issues, rec.Map{"code": "partial_or_interleaved_candidate", "occurrence": rec.Fetch(occurrence, "id")})
		}
	}
	if math && rec.Fetch(retention, "status") != "verified" {
		issues = append(issues, rec.Map{"code": "math_retention_unresolved", "reason": "Converted hook has no proven retained-occurrence bridge"})
	}
	return append(issues, rec.FetchList(retention, "diagnostics")...)
}
