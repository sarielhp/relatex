package html

import (
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/bookindex"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// The linked index (decision 084): an index anchor is an empty span at the start of the first
// text span of the block that issued its entry (the block's children stay its spans); a placed
// page number of the typeset index is a link to its term's first anchor on that page, its printed
// text unchanged.

// indexPlan is the document's index anchors and page-number links by span.
type indexPlan struct {
	anchors map[uintptr][]rec.Map
	links   map[uintptr][]rec.Map
}

func newIndexPlan(document rec.Map) *indexPlan {
	plan := &indexPlan{anchors: map[uintptr][]rec.Map{}, links: map[uintptr][]rec.Map{}}
	if _, present := document["index"]; !present {
		return plan
	}
	nodes := rec.FetchMap(document, "nodes")
	bySpan := func(target map[uintptr][]rec.Map, rows map[string][]rec.Map) {
		for block, items := range rows {
			content := rec.FetchList(rec.Node(nodes, block), "content")
			for _, item := range items {
				span := rec.AsMap(content[rec.FetchInt(item, "span")])
				target[rec.Identity(span)] = append(target[rec.Identity(span)], item)
			}
		}
	}
	bySpan(plan.anchors, bookindex.Anchors(document))
	bySpan(plan.links, bookindex.Links(document))
	return plan
}

func (p *Paper) index() *indexPlan {
	if p.indexes == nil {
		p.indexes = newIndexPlan(p.document)
	}
	return p.indexes
}

// withIndexAnchors opens a rendered span with the empty anchors it holds, after its opening tag
// (whose attribute values are escaped, so its first ">" ends it).
func (p *Paper) withIndexAnchors(html string, span rec.Map) string {
	anchors := p.index().anchors[rec.Identity(span)]
	if len(anchors) == 0 {
		return html
	}
	var builder strings.Builder
	for _, anchor := range anchors {
		builder.WriteString("<span id=\"" + htmlkit.Escape(rec.FetchString(anchor, "id")) + "\" data-index-anchor=\"" +
			htmlkit.Escape(rec.FetchString(anchor, "page")) + "\"></span>")
	}
	end := strings.IndexByte(html, '>') + 1
	return html[:end] + builder.String() + html[end:]
}

// indexLinkSpan is a text span with its page numbers linked, nil when it has none.
func (p *Paper) indexLinkSpan(span rec.Map) (string, bool) {
	links := p.index().links[rec.Identity(span)]
	if len(links) == 0 || span["type"] != "text" {
		return "", false
	}
	sorted := append([]rec.Map(nil), links...)
	sort.SliceStable(sorted, func(a, b int) bool { return rec.FetchInt(sorted[a], "start") < rec.FetchInt(sorted[b], "start") })
	text := []rune(rec.FetchString(span, "text"))
	cursor := 0
	var builder strings.Builder
	for _, link := range sorted {
		start := int(rec.FetchInt(link, "start"))
		printed := rec.FetchString(link, "text")
		builder.WriteString(htmlkit.Escape(string(text[cursor:start])))
		builder.WriteString("<a data-kind=\"index-page\" href=\"#" + htmlkit.Escape(rec.FetchString(link, "anchor")) + "\">" + htmlkit.Escape(printed) + "</a>")
		cursor = start + len([]rune(printed))
	}
	return "<span data-kind=\"text\">" + builder.String() + htmlkit.Escape(string(text[cursor:])) + "</span>", true
}
