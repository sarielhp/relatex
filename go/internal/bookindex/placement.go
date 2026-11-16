package bookindex

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/rec"
)

// segment is one text span of an entry's paragraphs: its block's node, its index in the
// block's content and its first rune in the entry's joined text.
type segment struct {
	block       string
	span, start int
	length      int
}

// breakRune stands for content that is no text (a formula): no page number spans it.
const breakRune = '￼'

// entry is one index entry: its term, its paragraph and its locators, each page placed in the
// paragraph's text (span, start) from the end, as the index prints them last, and linked to the
// anchors of the entry's term on that page.
func (p *projection) entry(entry Entry, path string, owners []int64, name string) rec.Map {
	text, segments := p.joined(owners)
	result := rec.Map{"level": int64(entry.Level), "term": entry.Term, "locators": []any{}}
	if len(segments) > 0 {
		result["block"] = segments[0].block
	}
	cursor := len(text)
	locators := make([]any, len(entry.Locators))
	for index := len(entry.Locators) - 1; index >= 0; index-- {
		locator := entry.Locators[index]
		if locator.Kind != "page" {
			locators[index] = rec.Map{"kind": locator.Kind, "target": locator.Target}
			continue
		}
		row := rec.Map{"kind": "page", "text": locator.Text, "page": locator.Page, "anchors": p.anchorsOn(path, locator.Page, name)}
		locators[index] = row
		if len(rec.AsList(row["anchors"])) == 0 {
			p.diagnostics = append(p.diagnostics, Diagnostic{"index.page_unmatched", fmt.Sprintf(
				"the index entry %q lists page %s, where no anchor of it was issued; the page number prints unlinked", entry.Term, locator.Page)})
		}
		start := lastStandalone(text[:cursor], []rune(locator.Text))
		where := place(segments, start, len([]rune(locator.Text)))
		// A range whose dash is a span of its own links its start page (it leads to the start).
		if where == nil && locator.Page != locator.Text {
			if where = place(segments, start, len([]rune(locator.Page))); where != nil {
				row["text"] = locator.Page
			}
		}
		if where == nil {
			p.diagnostics = append(p.diagnostics, Diagnostic{"index.locator_unplaced", fmt.Sprintf(
				"page %s of the index entry %q is not in its printed text; it prints unlinked", locator.Text, entry.Term)})
			continue
		}
		cursor = start
		for key, value := range where {
			row[key] = value
		}
	}
	result["locators"] = locators
	return result
}

// joined is the text of an entry's paragraphs, a breakRune for each span that is no text.
func (p *projection) joined(owners []int64) ([]rune, []segment) {
	text := []rune{}
	segments := []segment{}
	for _, owner := range owners {
		for _, block := range p.blocks[owner] {
			id := p.uid("block", rec.Fetch(block, "id"))
			for index, value := range rec.FetchList(block, "content") {
				span := rec.AsMap(value)
				if span["type"] != "text" {
					text = append(text, breakRune)
					continue
				}
				runes := []rune(rec.ToS(span["text"]))
				segments = append(segments, segment{block: id, span: index, start: len(text), length: len(runes)})
				text = append(text, runes...)
			}
			text = append(text, breakRune)
		}
	}
	return text, segments
}

// lastStandalone is the start of the last occurrence of needle in text that no letter or digit
// continues on either side, -1 without one.
func lastStandalone(text, needle []rune) int {
	for start := len(text) - len(needle); start >= 0; start-- {
		if string(text[start:start+len(needle)]) != string(needle) {
			continue
		}
		before := start == 0 || !alphanumeric(text[start-1])
		after := start+len(needle) == len(text) || !alphanumeric(text[start+len(needle)])
		if before && after {
			return start
		}
	}
	return -1
}

func alphanumeric(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// place is the span holding runes [start, start+length) of the joined text, nil when they
// are not within one text span.
func place(segments []segment, start, length int) rec.Map {
	if start < 0 {
		return nil
	}
	for _, s := range segments {
		if start >= s.start && start+length <= s.start+s.length {
			return rec.Map{"block": s.block, "span": int64(s.span), "start": int64(start - s.start)}
		}
	}
	return nil
}

// anchorsOn are the anchors of a term issued on a printed page, in issue order; an anchor of
// another index (memoir's and imakeidx's named indexes) is not the term's.
func (p *projection) anchorsOn(path, page, name string) []any {
	result := []any{}
	for _, a := range p.byPath[path] {
		if a.page == page && (a.name == "" || a.name == name || strings.TrimSuffix(a.name, ".idx") == name) {
			result = append(result, a.id)
		}
	}
	return result
}
