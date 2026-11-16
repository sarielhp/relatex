package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// pageEvidenceFixture: a theorem set on physical page 3 (printed "1", after two front-matter
// pages i and ii) and the book's \HLinkPage use: \hyperref (reference 1) printing "Theorem 1.2"
// and, in a formula, "p" with the \pageref (reference 2) printing the page.
func pageEvidenceFixture(pagePrinted string) (*Evidence, rec.Map, rec.Map, rec.Map) {
	references := omap.New[any, rec.Map]()
	references.Set(int64(1), rec.Map{"target": "theo:a", "selector": `\hyperref`})
	references.Set(int64(2), rec.Map{"target": "theo:a", "selector": pageSelector})
	records := []rec.Map{
		{"kind": "diagnostic", "code": "page_number", "page": int64(1), "printed": "i"},
		{"kind": "diagnostic", "code": "page_number", "page": int64(2), "printed": "ii"},
		{"kind": "diagnostic", "code": "page_number", "page": int64(3), "printed": "1"},
	}
	use := rec.Map{"type": "reference", "id": "r1", "reference": int64(1), "text": " Theorem 1.2", "target": "theo:a"}
	piece := rec.Map{"type": "reference", "id": "r2", "reference": int64(1), "text": "p", "target": "theo:a"}
	page := rec.Map{"type": "reference", "id": "r3", "reference": int64(2), "text": pagePrinted, "target": "theo:a"}
	e := &Evidence{
		references:   references,
		printedPages: PrintedPages(records),
		targets:      []rec.Map{{"label": "theo:a", "number": "1.2", "block": "b1", "anchor": "theorem.1.2"}},
		blocks: []rec.Map{
			{"id": "b1", "pages": []any{int64(3)}, "content": []any{rec.Map{"type": "text", "text": "Theorem 1.2. A statement."}}},
			{"id": "b2", "pages": []any{int64(1)}, "content": []any{use}},
		},
		mathReferences: []rec.Map{{"occurrence": "m1", "content": []any{piece, page}}},
	}
	return e, use, piece, page
}

// TestPageReferenceLinksToTheElementOnThePrintedPage: \pageref printing the printed number of
// the page its label's element is set on links to that element; the \hyperref text running into
// the formula is one use, linked by its piece outside the formula, its "p" left unlinked.
func TestPageReferenceLinksToTheElementOnThePrintedPage(t *testing.T) {
	e, use, piece, page := pageEvidenceFixture("1")
	e.resolveReferences()
	if len(e.diagnostics) != 0 {
		t.Fatalf("diagnostics %v", e.diagnostics)
	}
	if page["target_block"] != "b1" || use["target_block"] != "b1" {
		t.Errorf("page %v use %v", page, use)
	}
	if !unlinked(piece) {
		t.Errorf("formula piece %v", piece)
	}
}

// TestPageReferenceOnAnotherOrUnknownPageIsADiagnostic: a page that is not the element's, the
// physical page index instead of the printed number, or a stream without printed pages is no
// silent pass: the use is a diagnostic and prints unlinked.
func TestPageReferenceOnAnotherOrUnknownPageIsADiagnostic(t *testing.T) {
	for _, printed := range []string{"2", "3", "ii", ""} {
		e, _, _, page := pageEvidenceFixture(printed)
		e.resolveReferences()
		if !unlinked(page) || len(e.diagnostics) != 1 || e.diagnostics[0]["code"] != "unverified_page_reference" {
			t.Errorf("%q: page %v diagnostics %v", printed, page, e.diagnostics)
		}
	}
	e, _, _, page := pageEvidenceFixture("1")
	e.printedPages = nil
	e.resolveReferences()
	if !unlinked(page) || len(e.diagnostics) != 1 || e.diagnostics[0]["code"] != "unverified_page_reference" {
		t.Errorf("no printed pages: page %v diagnostics %v", page, e.diagnostics)
	}
}

// TestPrintedPagesRejectsARepeatedOrMalformedPage: one printed number per shipped page.
func TestPrintedPagesRejectsARepeatedOrMalformedPage(t *testing.T) {
	repeated := []rec.Map{
		{"kind": "diagnostic", "code": "page_number", "page": int64(1), "printed": "1"},
		{"kind": "diagnostic", "code": "page_number", "page": int64(1), "printed": "2"},
	}
	if PrintedPages(repeated) != nil {
		t.Error("a page printed twice")
	}
	if PrintedPages([]rec.Map{{"kind": "diagnostic", "code": "page_number", "page": int64(1)}}) != nil {
		t.Error("a page without its printed number")
	}
}

// TestNamedPageReferenceEndsWithThePage: \autopageref prints "page 7"; \pageref prints "7" alone.
func TestNamedPageReferenceEndsWithThePage(t *testing.T) {
	pages := []any{"7", "xii"}
	for _, test := range []struct {
		printed string
		named   bool
		want    bool
	}{{"page 7", true, true}, {"7", true, true}, {"page xii", true, true}, {"page 7", false, false},
		{"page 17", true, false}, {"page7", true, false}, {"page 8", true, false}, {"7", false, true}} {
		if got := printsPage(test.printed, pages, test.named); got != test.want {
			t.Errorf("%q named %v: %v, want %v", test.printed, test.named, got, test.want)
		}
	}
}
