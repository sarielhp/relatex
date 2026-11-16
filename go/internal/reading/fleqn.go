package reading

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// tagFormula is the MathML of a display that holds only an equation's printed number.
var tagFormula = regexp.MustCompile(`\A<math[^>]*><mtext>([^<]*)</mtext></math>\z`)

// attachFlushLeftNumbers joins the number of a flush-left equation to its formula. With
// amsmath's fleqn, the display is one line box: the formula is an inline formula of its own,
// and the number reads as the display's whole math (an mtext). The inline formula becomes the
// display block, the number a text span after it, and the number's formula is consumed with
// it (like a formula nested in another's reading).
//
// When the number does not fit beside the formula, amsmath sets the two as rows of a display
// alignment (\emdf@Ra, \emdf@La): no display math list exists, and the number is a text paragraph
// of the equation's entity below (or, tags left, above) the formula's (fifth cohort 1354).
func (e *Evidence) attachFlushLeftNumbers() {
	tags := e.tagFormulas()
	equations := e.equationEntities()
	result := make([]rec.Map, 0, len(e.blocks))
	for _, block := range e.blocks {
		last := len(result) - 1
		paired := last >= 0 && e.sameCell(result[last], block) && equations[rec.ToS(block["entity"])]
		if paired && e.isMarkedEquationNumber(block) && flushLeftFormula(result[last], block) {
			displayFormula(result[last])
			join(result[last], block, false)
			continue
		}
		if paired && e.isMarkedEquationNumber(result[last]) && flushLeftFormula(block, result[last]) {
			displayFormula(block)
			join(block, result[last], true)
			result[last] = block
			continue
		}
		span, tag := numberSpan(block, tags)
		if span != nil && paired && flushLeftFormula(result[last], block) {
			formula := result[last]
			displayFormula(formula)
			formula["content"] = append(withoutSpaces(formula), e.tagSpan(tag, span["formula"]))
			pages := rec.FetchList(formula, "pages")
			for _, page := range rec.FetchList(block, "pages") {
				pages = union(pages, page)
			}
			formula["pages"] = pages
			e.nestedMath = append(e.nestedMath, rec.Fetch(span, "occurrence"))
			continue
		}
		result = append(result, block)
	}
	e.blocks = result
	e.promoteUnnumberedDisplays()
}

// equationEntities are the entities of the displays the source sets (equation, align, gather, ...):
// only there is a formula and a parenthesized text beside it an equation and its number. A table
// head that prints $T_{\rm rot}$(X) over (K) (aastex \colhead, papers5/1210) is no equation.
func (e *Evidence) equationEntities() map[string]bool {
	entities := map[string]bool{}
	for _, record := range e.records {
		if record["code"] == "equation_context" {
			entities[rec.ToS(record["entity"])] = true
		}
	}
	return entities
}

// sameCell: a formula and its number are set in one place, both in the same table cell or both
// outside any table. Neighbouring cells are no equation: a lone formula in a table cell beside a
// cell that prints "(11,55)" stays an inline formula (papers2/1182).
func (e *Evidence) sameCell(formula, number rec.Map) bool {
	return rec.Equal(e.cellOf(formula), e.cellOf(number))
}

// cellOf is the table origin (table, copy, cell) of a block's first span set in a table, or nil.
func (e *Evidence) cellOf(block rec.Map) any {
	for _, value := range rec.FetchList(block, "content") {
		if origin, ok := e.origins[rec.Identity(rec.AsMap(value))]; ok {
			return origin
		}
	}
	return nil
}

// singleDisplays are the environments that set one formula as one display.
var singleDisplays = map[any]bool{"equation": true, "equation*": true, "displaymath": true}

// promoteUnnumberedDisplays: a flush-left display too wide for its line and given a tag that
// prints nothing (ntheorem's thmmarks end mark on an equation*) is set as a row of a display
// alignment like an equation whose number does not fit, with no number row: its formula is a
// prose block of the equation's entity, alone, and the display it is (the book 0003, 12 displays).
func (e *Evidence) promoteUnnumberedDisplays() {
	displays := map[string]bool{}
	for _, record := range e.records {
		if record["code"] == "equation_context" && record["source"] == "latex-equation-hooks" && singleDisplays[record["environment"]] {
			displays[rec.ToS(record["entity"])] = true
		}
	}
	for _, block := range e.blocks {
		if displays[rec.ToS(block["entity"])] && flushLeftFormula(block, block) {
			displayFormula(block)
		}
	}
}

// displayFormula makes a flush-left formula's paragraph its display block.
func displayFormula(formula rec.Map) {
	formula["type"] = "display_math"
	for _, value := range rec.FetchList(formula, "content") {
		if mathSpan := rec.AsMap(value); mathSpan["type"] == "math" {
			mathSpan["display"] = true
		}
	}
}

// tagFormulas are the formulas whose whole math is an equation tag, by formula id.
func (e *Evidence) tagFormulas() map[string]string {
	tags := map[string]string{}
	for _, record := range e.records {
		if rec.Fetch(record, "kind") != "math" {
			continue
		}
		match := tagFormula.FindStringSubmatch(rec.FetchString(record, "mathml"))
		if match != nil && equationTag.MatchString(strings.TrimSpace(match[1])) {
			tags[rec.ToS(rec.Fetch(record, "formula"))] = strings.TrimSpace(match[1])
		}
	}
	return tags
}

// numberSpan is the one math span of a display block whose formula is only a tag, and its tag.
func numberSpan(block rec.Map, tags map[string]string) (rec.Map, string) {
	if rec.Fetch(block, "type") != "display_math" {
		return nil, ""
	}
	content := withoutSpaces(block)
	if len(content) != 1 {
		return nil, ""
	}
	span := rec.AsMap(content[0])
	tag, ok := tags[rec.ToS(span["formula"])]
	if span["type"] != "math" || !ok {
		return nil, ""
	}
	return span, tag
}

// flushLeftFormula: a prose block in the number's own entity that holds one inline formula and
// nothing else but spaces.
func flushLeftFormula(formula, number rec.Map) bool {
	if rec.Fetch(formula, "type") != "paragraph" || rec.Fetch(formula, "entity") == nil ||
		!rec.Equal(rec.Fetch(formula, "entity"), rec.Fetch(number, "entity")) {
		return false
	}
	maths := 0
	for _, value := range rec.FetchList(formula, "content") {
		span := rec.AsMap(value)
		switch {
		case span["type"] == "math":
			maths++
		case span["type"] == "text" && strings.TrimSpace(rec.FetchString(span, "text")) == "":
		default:
			return false
		}
	}
	return maths == 1
}

func withoutSpaces(block rec.Map) []any {
	kept := []any{}
	for _, value := range rec.FetchList(block, "content") {
		span := rec.AsMap(value)
		if span["type"] == "text" && strings.TrimSpace(rec.FetchString(span, "text")) == "" {
			continue
		}
		kept = append(kept, value)
	}
	return kept
}

// tagSpan is an equation number printed beside its formula: a text span in the style its glyphs
// are set in (the collector's tag_style, \normalfont at the size around: a \small display's number
// is small); without that evidence, the document's body style (the most common style of its text).
func (e *Evidence) tagSpan(tag string, formula any) rec.Map {
	if style, known := e.tagStyles[rec.ToS(formula)]; known {
		return rec.Map{"type": "text", "text": " " + tag, "style": style}
	}
	counts := map[string]int{}
	styles := map[string]any{}
	for _, block := range e.blocks {
		for _, value := range rec.FetchList(block, "content") {
			span := rec.AsMap(value)
			if span["type"] != "text" || span["style"] == nil {
				continue
			}
			key := rec.ToS(span["style"])
			counts[key]++
			styles[key] = span["style"]
		}
	}
	var best string
	for key, count := range counts {
		if count > counts[best] || (count == counts[best] && key < best) {
			best = key
		}
	}
	return rec.Map{"type": "text", "text": " " + tag, "style": styles[best]}
}
