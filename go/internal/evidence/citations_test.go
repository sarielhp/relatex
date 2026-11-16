package evidence

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func citationEvent(code string, fields rec.Map) rec.Map {
	row := rec.Map{"kind": "diagnostic", "code": code, "source": "latex-command-hooks"}
	for key, value := range fields {
		row[key] = value
	}
	return row
}

func citationOwner(id int64, metadata rec.Map) rec.Map {
	item := rec.Map{"type": "text", "text": "1", "space": ""}
	for key, value := range metadata {
		item[key] = value
	}
	return rec.Map{"kind": "flow_candidate", "owner": id, "entity": int64(0), "role": "paragraph", "group": "paragraph",
		"items": []any{item}}
}

func citationOccurrence(id int64) rec.Map {
	return rec.Map{"kind": "flow_occurrence", "owner": id, "first": int64(1), "last": int64(1), "page": int64(1)}
}

// Two \bibitem{alpha}: LaTeX numbers alpha's citations with the last item, 2. Each item keeps
// its own number, and the citation targets the last one (third cohort 563, 841).
func TestRepeatedBibliographyKeyCitesTheLastItem(t *testing.T) {
	records := []rec.Map{
		{"kind": "header", "protocol": "0.6", "collector": "0.6.1", "attempt": "unit", "pass": int64(2), "input": "unit",
			"engine": "unit", "adapter": "external", "extensions": []any{"external-math-0.1", "citations-0.1"}},
		citationEvent("bibliography_begin", rec.Map{"bibliography": int64(1)}),
		citationEvent("bibliography_entry", rec.Map{"bibliography": int64(1), "entry": int64(1), "key": "alpha"}),
		citationEvent("bibliography_anchor", rec.Map{"entry": int64(1), "key": "alpha", "number": "1"}),
		citationEvent("bibliography_entry_end", rec.Map{"entry": int64(1)}),
		citationEvent("bibliography_entry", rec.Map{"bibliography": int64(1), "entry": int64(2), "key": "alpha"}),
		citationEvent("bibliography_anchor", rec.Map{"entry": int64(2), "key": "alpha", "number": "2"}),
		citationEvent("bibliography_entry_end", rec.Map{"entry": int64(2)}),
		citationEvent("bibliography_end", rec.Map{"bibliography": int64(1)}),
		citationEvent("citation_call", rec.Map{"citation": int64(1), "requested": "alpha"}),
		citationEvent("citation_sorted", rec.Map{"citation": int64(1), "keys": "alpha"}),
		citationEvent("citation_link", rec.Map{"citation": int64(1), "citation_link": int64(1), "key": "alpha", "number": "2"}),
		citationEvent("citation_link_end", rec.Map{"citation_link": int64(1)}),
		citationEvent("citation_end", rec.Map{"citation": int64(1)}),
		citationOwner(1, rec.Map{"citation": int64(1), "citation_link": int64(1)}),
		citationOwner(2, rec.Map{"bibliography_entry": int64(1)}),
		citationOwner(3, rec.Map{"bibliography_entry": int64(2)}),
		citationOccurrence(1), citationOccurrence(2), citationOccurrence(3),
		citationEvent("citation_retained", rec.Map{"citation": int64(1), "citation_link": int64(1), "owner": int64(1),
			"first": int64(1), "last": int64(1), "page": int64(1)}),
		citationEvent("bibliography_retained", rec.Map{"entry": int64(1), "owner": int64(2), "first": int64(1), "last": int64(1), "page": int64(1)}),
		citationEvent("bibliography_retained", rec.Map{"entry": int64(2), "owner": int64(3), "first": int64(1), "last": int64(1), "page": int64(1)}),
	}
	for index, row := range records {
		row["seq"] = int64(index + 1)
	}
	report := CitationReport(records)
	if report["status"] != "verified" {
		t.Fatalf("status = %v, diagnostics %v", report["status"], report["diagnostics"])
	}
	entries := rec.FetchList(report, "entries")
	if len(entries) != 2 || rec.Fetch(rec.AsMap(entries[0]), "number") != "1" || rec.Fetch(rec.AsMap(entries[1]), "number") != "2" {
		t.Fatalf("entries = %v", entries)
	}
	call := rec.AsMap(rec.FetchList(report, "calls")[0])
	if !rec.Equal(rec.FetchList(call, "targets"), []any{int64(2)}) {
		t.Fatalf("targets = %v", call["targets"])
	}
	warnings := rec.FetchList(report, "warnings")
	if len(warnings) != 1 || rec.Fetch(rec.AsMap(warnings[0]), "code") != "bibliography_key_repeated" {
		t.Fatalf("warnings = %v", warnings)
	}
}

// alpha's \etalchar{+} records a superscript formula in the label: FHM{$^{+}$}20 prints FHM+20
// (third cohort 660, 853).
func TestPrintedLabelReadsSuperscriptFormula(t *testing.T) {
	cases := map[string]string{"FHM{$^{+}$}20": "FHM+20", "Sz{é}20": "Szé20", "AB$^*$01": "AB*01", "12": "12"}
	for recorded, want := range cases {
		if got := printedLabel(recorded); got != want {
			t.Errorf("printedLabel(%q) = %q, want %q", recorded, got, want)
		}
	}
}
