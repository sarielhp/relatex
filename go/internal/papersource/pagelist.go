package papersource

import (
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/reading"
	"github.com/sarielhp/relatex/internal/rec"
)

// PageListFile is the reconcile stage's record of where each page of the print PDF begins in the
// reading order (plan accessibility A8 and B1): the EPUB package marks each page there and lists
// the marks in its page-list navigation.
const PageListFile = "page-list.json"

// PageListVersion is the schema of PageListFile.
const PageListVersion = "page-list-1"

// printedLabelLimit bounds a printed page number: \thepage of a real document is a few characters.
const printedLabelLimit = 40

// blockPageSets are the pages of each reading block that names its pages, by block id.
func blockPageSets(evidence rec.Map) map[any][]int64 {
	sets := map[any][]int64{}
	blocks, _ := evidence["blocks"].([]any)
	for _, value := range blocks {
		block, _ := value.(rec.Map)
		list, _ := block["pages"].([]any)
		for _, page := range list {
			if number, ok := page.(int64); ok && number > 0 {
				sets[block["id"]] = append(sets[block["id"]], number)
			}
		}
	}
	return sets
}

// printedLabels are the printed page numbers (\thepage as each page shipped, the collector's
// page_number records, as \pageref reads them) by physical page; nil when the stream records
// none, or a page twice.
func printedLabels(records []rec.Map) map[int64]string {
	labels := reading.PrintedPages(records)
	if len(labels) == 0 {
		return nil
	}
	return labels
}

// PageList is PageListFile's content: for each page of the PDF in order, its label (the printed
// number when the stream records every page's, else the physical one) and the main-text block the
// page begins in: the first block after the previous page's that is set on the page, else the
// first that begins after it (a page of floats or a page the previous block fills). A page after
// the last block (floats, an empty page) begins at the end of the text: its node is "", so every
// page of the PDF is listed (decision 130). labels is "printed" or "physical".
func (s *Source) PageList(document rec.Map) rec.Map {
	nodes, _ := document["nodes"].(rec.Map)
	order, _ := document["main_block_order"].([]any)
	type block struct {
		node  any
		pages []int64
	}
	blocks := []block{}
	last := int64(0)
	for _, id := range order {
		node, _ := nodes[rec.AsString(id)].(rec.Map)
		provenance, _ := node["provenance"].(rec.Map)
		if pages := s.pageSets[provenance["inspection_key"]]; len(pages) > 0 {
			blocks = append(blocks, block{id, pages})
			last = max(last, slices.Max(pages))
		}
	}
	labels, kind := s.pageLabels, "printed"
	if labels == nil {
		kind = "physical"
	}
	entries := []any{}
	cursor := 0
	for page := int64(1); page <= max(last, s.pdfPages()); page++ {
		if page > last {
			entries = append(entries, rec.Map{"page": page, "label": pageLabel(labels, page), "node": ""})
			continue
		}
		at := slices.IndexFunc(blocks[cursor:], func(b block) bool { return slices.Contains(b.pages, page) })
		if at < 0 {
			at = slices.IndexFunc(blocks[cursor:], func(b block) bool { return slices.Min(b.pages) > page })
		}
		if at < 0 {
			continue
		}
		cursor += at
		entries = append(entries, rec.Map{"page": page, "label": pageLabel(labels, page), "node": blocks[cursor].node})
	}
	return rec.Map{"schema_version": PageListVersion, "labels": kind, "pages": entries}
}

// pdfPages is the PDF's physical page count (its page-count proof), 0 when it was not counted.
func (s *Source) pdfPages() int64 {
	proof, _ := s.pageCountProof.(rec.Map)
	pages, _ := proof["pages"].(int64)
	return pages
}

// pageLabel is the page's printed number, white space collapsed; the physical number when the
// page printed none or something too long to be a page number.
func pageLabel(labels map[int64]string, page int64) string {
	label := strings.Join(strings.Fields(labels[page]), " ")
	if label == "" || len(label) > printedLabelLimit {
		return strconv.FormatInt(page, 10)
	}
	return label
}
