package papersource

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestPageListMarksTheBlockEachPageBeginsIn(t *testing.T) {
	reading := rec.Map{"blocks": []any{
		rec.Map{"id": "b1", "pages": []any{int64(1)}}, rec.Map{"id": "b2", "pages": []any{int64(2), int64(3), int64(4)}},
		rec.Map{"id": "b3", "pages": []any{int64(4)}}, rec.Map{"id": "b4", "pages": []any{int64(6)}},
		rec.Map{"id": "float", "pages": []any{int64(2)}}, rec.Map{"id": "b5", "pages": []any{}},
	}}
	nodes := rec.Map{}
	order := []any{}
	for _, key := range []string{"b1", "b2", "b3", "float", "b4", "b5"} {
		nodes["n-"+key] = rec.Map{"type": "paragraph", "provenance": rec.Map{"inspection_key": key}}
		order = append(order, "n-"+key)
	}
	document := rec.Map{"main_block_order": order, "nodes": nodes}
	records := []rec.Map{}
	for page, printed := range []string{"i", "1", "2", "3", "4", " 5 "} {
		records = append(records, rec.Map{"kind": "diagnostic", "code": "page_number", "page": int64(page + 1), "printed": printed})
	}
	source := &Source{pageSets: blockPageSets(reading), pageLabels: printedLabels(records)}
	got := source.PageList(document)
	// Page 5 holds no block: its mark goes to the block after it. The float set on page 2 but
	// read after page 4 does not move page 2 back.
	want := rec.Map{"schema_version": PageListVersion, "labels": "printed", "pages": []any{
		rec.Map{"page": int64(1), "label": "i", "node": "n-b1"}, rec.Map{"page": int64(2), "label": "1", "node": "n-b2"},
		rec.Map{"page": int64(3), "label": "2", "node": "n-b2"}, rec.Map{"page": int64(4), "label": "3", "node": "n-b2"},
		rec.Map{"page": int64(5), "label": "4", "node": "n-b4"}, rec.Map{"page": int64(6), "label": "5", "node": "n-b4"}}}
	if !rec.Equal(got, want) {
		t.Fatalf("page list %v, want %v", got, want)
	}
	// Pages after the last block (a page of floats at the end) begin at the end of the text: every
	// page of the PDF is listed (decision 130).
	source.pageCountProof = rec.Map{"pages": int64(8)}
	got = source.PageList(document)
	pages := rec.FetchList(got, "pages")
	if len(pages) != 8 || !rec.Equal(pages[6], rec.Map{"page": int64(7), "label": "7", "node": ""}) ||
		!rec.Equal(pages[5], rec.Map{"page": int64(6), "label": "5", "node": "n-b4"}) {
		t.Errorf("trailing pages %v", pages)
	}
	source.pageCountProof = nil
	// A page recorded twice proves no printed number: the physical ones label the pages.
	records = append(records, rec.Map{"kind": "diagnostic", "code": "page_number", "page": int64(1), "printed": "x"})
	source.pageLabels = printedLabels(records)
	got = source.PageList(document)
	if got["labels"] != "physical" || rec.AsMap(rec.FetchList(got, "pages")[0])["label"] != "1" {
		t.Errorf("physical labels %v", got)
	}
}
