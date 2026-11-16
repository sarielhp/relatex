package reading

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// equationTag is the printed number of an equation: its counter between parentheses or brackets.
var equationTag = regexp.MustCompile(`\A[(\[][^()\[\]\s]{1,16}[)\]]\z`)

// rulePlaceholder is the text the flow reads for a rule it cannot model (a tabular's \hline):
// bracketed like a tag, but no number (the book 0003: an algorithm's last formula above its rule).
const rulePlaceholder = "[rule]"

// attachEquationNumbers joins an equation's printed number to its formula. The number is set
// beside the formula, in the equation's own entity, but reads as a paragraph of its own; it
// becomes a trailing (or, set at the left, a leading) text span of the display block. isNumber tells the
// number's paragraph: by the collector's tag mark, or by its shape (isEquationNumber).
func attachEquationNumbers(blocks []rec.Map, isNumber func(rec.Map) bool) []rec.Map {
	result := make([]rec.Map, 0, len(blocks))
	for index := 0; index < len(blocks); index++ {
		block := blocks[index]
		if !isNumber(block) {
			result = append(result, block)
			continue
		}
		if last := len(result) - 1; last >= 0 && numbers(result[last], block) {
			join(result[last], block, false)
			continue
		}
		if index+1 < len(blocks) && numbers(blocks[index+1], block) {
			join(blocks[index+1], block, true)
			continue
		}
		result = append(result, block)
	}
	return result
}

// maxTagFormulas bounds the formulas of a tag typeset with math (\tag{$A$}, \tag{NLS$_a$},
// \tag{$R_1$-$R_2$}): a tag is a short number, not a run of formulas.
const maxTagFormulas = 3

// isEquationNumber: a paragraph of the equation's entity that prints only "(number)". A tag typeset
// with math (\tag{$A$}) reads as text and inline formulas, "(" 𝐴 ")"; each formula stands for one
// character of the number, so the shape is the same and no word space may separate its parts.
func isEquationNumber(block rec.Map) bool {
	if rec.Fetch(block, "type") != "paragraph" || rec.Fetch(block, "entity") == nil {
		return false
	}
	var text strings.Builder
	formulas := 0
	for _, value := range rec.FetchList(block, "content") {
		span := rec.AsMap(value)
		switch span["type"] {
		case "text":
			text.WriteString(rec.FetchString(span, "text"))
		case "math":
			formulas++
			text.WriteString("x")
		default:
			return false
		}
	}
	tag := rec.Strip(text.String())
	return tag != rulePlaceholder && formulas <= maxTagFormulas && equationTag.MatchString(tag)
}

func numbers(display, number rec.Map) bool {
	return rec.Fetch(display, "type") == "display_math" && rec.Equal(rec.Fetch(display, "entity"), rec.Fetch(number, "entity"))
}

func join(display, number rec.Map, before bool) {
	content := rec.FetchList(display, "content")
	if before {
		display["content"] = append(append([]any{}, rec.FetchList(number, "content")...), content...)
	} else {
		display["content"] = append(content, rec.FetchList(number, "content")...)
	}
	pages := rec.FetchList(display, "pages")
	for _, page := range rec.FetchList(number, "pages") {
		pages = union(pages, page)
	}
	display["pages"] = pages
}
