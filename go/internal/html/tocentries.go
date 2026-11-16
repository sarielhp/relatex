package html

import (
	"regexp"
	"strings"
	"unicode"
)

// contentsTitles are the titles of a printed contents page, as text.
var contentsTitles = map[string]bool{"contents": true, "table of contents": true}

// printedPage is the page number a contents entry ends with: digits or a roman numeral after a space.
var printedPage = regexp.MustCompile(`\S\s+(\d{1,4}|[ivxlcdm]{1,7})$`)

// contentsEntries are the ids of the paragraphs that make a printed contents list: the run of
// short paragraphs, each ending in a page number, right after a paragraph titled "Contents". The
// PDF sets them on consecutive lines, so they read as one tight list, not as separate paragraphs
// with the page's paragraph spacing. Found once.
func (p *Paper) contentsEntries() map[string]bool {
	if p.tocEntries != nil {
		return p.tocEntries
	}
	p.tocEntries = map[string]bool{}
	var run []string
	titled := false
	flush := func() {
		if len(run) >= 2 {
			for _, id := range run {
				p.tocEntries[id] = true
			}
		}
		run, titled = nil, false
	}
	order, _ := p.document["main_block_order"].([]any)
	for _, value := range order {
		id, _ := value.(string)
		node := p.node(id)
		if node["type"] != "paragraph" {
			flush()
			continue
		}
		text := strings.TrimSpace(paragraphText(node))
		switch {
		case contentsTitles[strings.ToLower(strings.TrimRightFunc(text, unicode.IsPunct))]:
			flush()
			titled = true
		case titled && len(text) < 200 && printedPage.MatchString(text):
			run = append(run, id)
		default:
			flush()
		}
	}
	flush()
	return p.tocEntries
}
