package htmlkit

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// runDocument is a document whose bibliography entries sit in a section after prose (the revtex
// shape), at the top level, or wherever children and entries place them.
func runDocument(children []any, parent string) rec.Map {
	text := []any{rec.Map{"type": "text", "text": "x"}}
	nodes := rec.Map{
		"root": rec.Map{"id": "root", "type": "document", "children": []any{"s1"}},
		"s1":   rec.Map{"id": "s1", "type": "section", "children": []any{"h1", "p1"}},
		"h1":   rec.Map{"id": "h1", "type": "heading", "content": text},
		"p1":   rec.Map{"id": "p1", "type": "paragraph", "content": text},
		"e1":   rec.Map{"id": "e1", "type": "paragraph", "content": text},
		"e2":   rec.Map{"id": "e2", "type": "paragraph", "content": text},
		"p2":   rec.Map{"id": "p2", "type": "paragraph", "content": text},
		"fl":   rec.Map{"id": "fl", "type": "figure_float", "children": []any{}},
		"s2":   rec.Map{"id": "s2", "type": "section", "children": []any{"h2", "p1"}},
		"h2":   rec.Map{"id": "h2", "type": "heading", "content": text},
	}
	rec.AsMap(nodes[parent])["children"] = children
	return rec.Map{
		"root": "root", "nodes": nodes,
		"citations": rec.Map{"entries": []any{rec.Map{"blocks": []any{"e1"}}, rec.Map{"blocks": []any{"e2"}}}},
	}
}

// The entries after prose in one section, or at the top level, form one run.
func TestBibliographyRunFindsTheEntriesRun(t *testing.T) {
	section := runDocument([]any{"h2", "p1", "e1", "e2"}, "s2")
	if parent, first, last, ok := BibliographyRun(section); !ok || parent != "s2" || first != 2 || last != 3 {
		t.Errorf("section run %q %d %d %v", parent, first, last, ok)
	}
	top := runDocument([]any{"p1", "e1", "e2"}, "root")
	if parent, first, last, ok := BibliographyRun(top); !ok || parent != "root" || first != 1 || last != 2 {
		t.Errorf("top-level run %q %d %d %v", parent, first, last, ok)
	}
	// A float among the entries does not break the run.
	floating := runDocument([]any{"p1", "e1", "fl", "e2"}, "root")
	if parent, first, last, ok := BibliographyRun(floating); !ok || parent != "root" || first != 1 || last != 3 {
		t.Errorf("float run %q %d %d %v", parent, first, last, ok)
	}
}

// Entries interleaved with prose, in two sections, or alone are no run.
func TestBibliographyRunRejectsBrokenRuns(t *testing.T) {
	interleaved := runDocument([]any{"e1", "p1", "e2"}, "root")
	if _, _, _, ok := BibliographyRun(interleaved); ok {
		t.Error("interleaved entries made a run")
	}
	two := runDocument([]any{"s1", "s2"}, "root")
	rec.AsMap(rec.FetchMap(two, "nodes")["s1"])["children"] = []any{"h1", "p1", "e1"}
	rec.AsMap(rec.FetchMap(two, "nodes")["s2"])["children"] = []any{"h2", "e2"}
	if _, _, _, ok := BibliographyRun(two); ok {
		t.Error("entries in two parents made a run")
	}
	alone := runDocument([]any{"p1", "e1"}, "root")
	alone["citations"] = rec.Map{"entries": []any{rec.Map{"blocks": []any{"e1"}}}}
	if _, _, _, ok := BibliographyRun(alone); ok {
		t.Error("one entry made a run")
	}
}

// When a section landmark applies, the run is not asked.
func TestBibliographyRunLeavesTheSectionLandmarkAlone(t *testing.T) {
	section := runDocument([]any{"h2", "e1", "e2"}, "s2")
	if BibliographySection(section) != "s2" {
		t.Fatal("the section landmark does not apply")
	}
	if _, _, _, ok := BibliographyRun(section); ok {
		t.Error("a run was found beside the section landmark")
	}
}
