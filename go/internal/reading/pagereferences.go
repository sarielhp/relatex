package reading

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// pageSelector is the \@setref selector of \pageref: the label's page, not its number;
// namedPageSelector records hyperref's \autopageref, which prints the page after its name ("page 7").
const (
	pageSelector      = `\@secondoffive`
	namedPageSelector = `\autopageref`
)

// PrintedPages maps each shipped page (its physical index) to its printed number, \thepage as
// the page shipped (the collector's page_number records). A stream without them, or with a
// page recorded twice or malformed, proves no printed page: page references stay unverified.
func PrintedPages(records []rec.Map) map[int64]string {
	pages := map[int64]string{}
	for _, record := range records {
		if record["kind"] != "diagnostic" || record["code"] != "page_number" {
			continue
		}
		page, isPage := record["page"].(int64)
		printed, isText := record["printed"].(string)
		if _, seen := pages[page]; !isPage || !isText || seen || page < 1 {
			return nil
		}
		pages[page] = printed
	}
	return pages
}

// resolvePageReference: \pageref prints the page its label was set on. The use links to the
// label's element, as other references do, when its printed text is the printed number of a
// page that element is set on; an unknown page or another number is a diagnostic, and the use
// prints as unlinked text. \autopageref prints the page as the last word of its text.
func (e *Evidence) resolvePageReference(targets map[any]rec.Map, span rec.Map, named bool) {
	target := targets[rec.Fetch(span, "target")]
	if target == nil || target["kind"] == "document_page_total" {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "unresolved_reference_use", "reference": rec.Fetch(span, "id"),
			"target": rec.Fetch(span, "target")})
		return
	}
	printed := rec.Strip(squeeze(rec.FetchString(span, "text"), " "))
	pages := e.targetPages(target)
	if !printsPage(printed, pages, named) {
		e.diagnostics = append(e.diagnostics, rec.Map{"code": "unverified_page_reference", "reference": rec.Fetch(span, "id"),
			"target": rec.Fetch(span, "target"), "printed": printed, "pages": pages})
		return
	}
	span["target_block"] = rec.Fetch(target, "block")
	if truthy(target["row"]) {
		span["target_row"] = rec.Fetch(target, "row")
	}
}

// printsPage: the printed text is one of the pages, alone or, for a named use, as its last word.
func printsPage(printed string, pages []any, named bool) bool {
	for _, page := range pages {
		number := toS(page)
		if number != "" && (printed == number || named && strings.HasSuffix(printed, " "+number)) {
			return true
		}
	}
	return false
}

// targetPages are the printed numbers of the pages the target's element is set on.
func (e *Evidence) targetPages(target rec.Map) []any {
	if e.printedPages == nil {
		return []any{}
	}
	if e.blockIndex == nil {
		e.blockIndex = map[any]rec.Map{}
		for _, block := range e.blocks {
			e.blockIndex[rec.Fetch(block, "id")] = block
		}
	}
	printed := []any{}
	shipped, _ := e.blockIndex[target["block"]]["pages"].([]any)
	for _, page := range shipped {
		index, _ := page.(int64)
		number, known := e.printedPages[index]
		if known && !slices.Contains(printed, any(number)) {
			printed = append(printed, number)
		}
	}
	return printed
}
