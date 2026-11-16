package reading

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// The collector marks the material a display's tag sets (tag: true on its flow items: amsmath's
// \maketag@@@, the kernel's \@eqnnum; approved 2026-11-10). The reader records which text spans and
// formulas came from marked items, and knows an equation's number by that mark; the shape rule
// (isEquationNumber: a paragraph printing "(number)") stays the fallback where nothing is marked.
const (
	tagMarked   = 1
	tagUnmarked = 2
)

// markTagSpan records whether the item merged into (or opening) a text span was tag material.
func (e *Evidence) markTagSpan(span, item rec.Map) {
	if e.tagSpans == nil {
		e.tagSpans = map[uintptr]int{}
	}
	state := tagUnmarked
	if truthy(item["tag"]) {
		state = tagMarked
	}
	e.tagSpans[rec.Identity(span)] |= state
}

// markTagFormula records a formula read from marked items (the math of \tag{$A$}).
func (e *Evidence) markTagFormula(item rec.Map) {
	if !truthy(item["tag"]) {
		return
	}
	if e.tagMath == nil {
		e.tagMath = map[string]bool{}
	}
	e.tagMath[rec.ToS(item["formula"])] = true
	e.noteTagStyle(item)
}

// noteTagStyle records the text style of a formula's first tag glyph that names one (tag_style:
// the font \maketag@@@ sets the number in, a declared text font).
func (e *Evidence) noteTagStyle(item rec.Map) {
	font, named := item["tag_style"].(int64)
	if !named || !e.textStyles.Has(font) {
		return
	}
	if e.tagStyles == nil {
		e.tagStyles = map[string]rec.Map{}
	}
	key := rec.ToS(item["formula"])
	if _, seen := e.tagStyles[key]; !seen {
		e.tagStyles[key] = e.textStyles.Style(font, "Undeclared tag font")
	}
}

// spanTag is a span's mark: tagMarked, tagUnmarked, or 0 for a span that says nothing (white space,
// a span of another kind).
func (e *Evidence) spanTag(span rec.Map) int {
	switch span["type"] {
	case "text":
		if strings.TrimSpace(rec.FetchString(span, "text")) == "" {
			return 0
		}
		return e.tagSpans[rec.Identity(span)]
	case "math":
		if e.tagMath[rec.ToS(span["formula"])] {
			return tagMarked
		}
		return tagUnmarked
	}
	return tagUnmarked
}

// markedTag: whether the spans carry the mark at all (decided), and whether every one that says
// anything is tag material (marked).
func (e *Evidence) markedTag(spans []any) (marked, decided bool) {
	all, anyMarked := true, false
	for _, value := range spans {
		switch e.spanTag(rec.AsMap(value)) {
		case tagMarked:
			anyMarked = true
		case 0:
		default:
			all = false
		}
	}
	return anyMarked && all, anyMarked
}

// isMarkedEquationNumber is isEquationNumber by the collector's mark: a paragraph of the equation's
// entity holding only tag material. Without any mark, the shape rule decides.
func (e *Evidence) isMarkedEquationNumber(block rec.Map) bool {
	marked, decided := e.markedTag(rec.FetchList(block, "content"))
	if !decided {
		return isEquationNumber(block)
	}
	return marked && rec.Fetch(block, "type") == "paragraph" && rec.Fetch(block, "entity") != nil
}
