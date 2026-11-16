package papersource

import "github.com/sarielhp/relatex/internal/rec"

// ChapterPagesFile is the reconcile stage's record of the PDF page each part and chapter of the
// document begins on (decision 087): the bundle cuts the layout into files at these pages.
const ChapterPagesFile = "chapter-pages.json"

// ChapterPagesVersion is the schema of ChapterPagesFile.
const ChapterPagesVersion = "chapter-pages-1"

// firstPages is the first page of each reading block that names its pages, by block id.
func firstPages(reading rec.Map) map[any]int64 {
	pages := map[any]int64{}
	blocks, _ := reading["blocks"].([]any)
	for _, value := range blocks {
		block, _ := value.(rec.Map)
		list, _ := block["pages"].([]any)
		if len(list) == 0 {
			continue
		}
		if page, ok := list[0].(int64); ok {
			pages[block["id"]] = page
		}
	}
	return pages
}

// ChapterPages is ChapterPagesFile's content: each heading of the main text whose counter is part
// or chapter, in reading order, with the page its reading block begins on. A heading whose block
// names no page is left out; a paper has none.
func (s *Source) ChapterPages(document rec.Map) rec.Map {
	headings := []any{}
	nodes, _ := document["nodes"].(rec.Map)
	order, _ := document["main_block_order"].([]any)
	for _, id := range order {
		node, _ := nodes[rec.AsString(id)].(rec.Map)
		semantics, _ := node["semantics"].(rec.Map)
		counter, _ := semantics["counter"].(string)
		if node["type"] != "heading" || (counter != "part" && counter != "chapter") {
			continue
		}
		provenance, _ := node["provenance"].(rec.Map)
		if page, ok := s.blockPages[provenance["inspection_key"]]; ok {
			headings = append(headings, rec.Map{"node": id, "counter": counter, "page": page})
		}
	}
	return rec.Map{"schema_version": ChapterPagesVersion, "headings": headings}
}
