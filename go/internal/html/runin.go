package html

import (
	"strconv"

	"github.com/sarielhp/relatex/internal/rec"
)

// A run-in heading (\paragraph, a class's run-in \subsubsection) is set at the start of its
// paragraph, so its title cannot be a heading element: an h4 inside a p ends the p. Its title
// spans are a span with role="heading" and aria-level (decision 088, amendment 1): a real heading
// for assistive technology, in the paragraph's line, so the page looks as before.

// runinTitle is the title a paragraph's content opens with: its first spans and their level.
type runinTitle struct {
	spans int
	level int
}

// edge is what the content list writes before its span at index: the title's opening before the
// first span and its closing after the last.
func (t runinTitle) edge(index int) string {
	switch {
	case t.spans == 0:
		return ""
	case index == 0:
		return `<span class="runin-heading" role="heading" aria-level="` + strconv.Itoa(t.level) + `">`
	case index == t.spans:
		return "</span>"
	}
	return ""
}

// RuninSpans is how many of a paragraph's first spans are a run-in heading's title, 0 for none.
func RuninSpans(node rec.Map) int {
	semantics, _ := node["semantics"].(rec.Map)
	count, ok := rec.Number(semantics["title_spans"])
	if !ok || semantics["runin"] != true || int(count) > len(rec.FetchList(node, "content")) {
		return 0
	}
	return int(count)
}

// runinTitleOf is a paragraph's run-in title: one level below the heading of the sections around
// it (h(2 + their number), at most 6), so the levels never skip.
func (p *Paper) runinTitleOf(identifier string, node rec.Map) runinTitle {
	spans := RuninSpans(node)
	if spans == 0 {
		return runinTitle{}
	}
	p.headingTag(identifier)
	return runinTitle{spans: spans, level: min(max(2+p.headingDepths[identifier], 2), 6)}
}
