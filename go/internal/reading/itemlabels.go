package reading

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/evidence"
	"github.com/sarielhp/relatex/internal/rec"
)

// unobservedItemBlocks: a \label in an item of a list the collector does not observe (an
// enumitem \newlist list, such as the book's compactenumA with label=(\Alph*)) has no item
// context, only hyperref's Item.N anchor. \item sets its marker before the label runs, and the
// item's text opens the next paragraph of the label's context: the label names that paragraph
// when it opens with the label's printed marker ("(i) For any ...", "3: if ..."). An item of an
// inline list (enumitem's enumerate*, paralist's inparaenum) sets its marker in a box inside the
// running paragraph: the label names that paragraph when the box set just before the label
// prints the marker and the paragraph prints that box's text as a word of its own.
func (e *Evidence) unobservedItemBlocks(label rec.Map) []rec.Map {
	marker := evidence.LiteralNumber(toS(rec.Fetch(label, "number")))
	custom := customItemLabel(label)
	if (!strings.HasPrefix(toS(label["anchor"]), "Item.") && !custom) || marker == nil || *marker == "" {
		return []rec.Map{}
	}
	var owner any
	box := ""
	for _, row := range e.records {
		if row["kind"] != "flow_candidate" || !rec.Equal(row["entity"], label["entity"]) {
			continue
		}
		if less(rec.Fetch(row, "seq"), rec.Fetch(label, "seq")) {
			box = markerBoxText(row)
			continue
		}
		if row["group"] == "paragraph" {
			owner = row["owner"]
			break
		}
	}
	if owner == nil {
		return []rec.Map{}
	}
	inline := printsMarker(box, *marker)
	return one(e.blocksWhere(func(block rec.Map) bool {
		return rec.Equal(block["owner"], owner) &&
			(opensWithMarker(leadingText(block), *marker) || custom && opensWithDelimitedMarker(leadingText(block), *marker) ||
				inline && holdsWord(printedText(block), box))
	}))
}

// printedText: the text of the block's spans that print text (its words and references; a
// formula or picture prints none).
func printedText(block rec.Map) string {
	return textOf(block, func(span rec.Map) bool {
		_, printed := span["text"].(string)
		return printed
	})
}

// markerBoxText: the text a box candidate prints ("(ii)"), or "" for another candidate.
func markerBoxText(row rec.Map) string {
	if row["group"] != "hbox" {
		return ""
	}
	var builder strings.Builder
	items, _ := row["items"].([]any)
	for _, value := range items {
		item := rec.AsMap(value)
		if item["type"] != "text" {
			return ""
		}
		builder.WriteString(rec.FetchString(item, "space") + rec.FetchString(item, "text"))
	}
	return strings.TrimSpace(builder.String())
}

// printsMarker: the box prints the item's number as a list marker: bare, in parentheses, or
// followed by a period, parenthesis or colon ("ii", "(ii)", "2.", "b)", "3:").
func printsMarker(text, marker string) bool {
	if text == "" {
		return false
	}
	for _, form := range []string{marker, "(" + marker + ")", marker + ".", marker + ")", marker + ":"} {
		if text == form {
			return true
		}
	}
	return false
}

// holdsWord: the text prints word as a word of its own, at its start or after a space, and
// before a space, a comma, semicolon, colon or period, or its end ("(ii)" in "and (ii) the",
// not in "x(ii)").
func holdsWord(text, word string) bool {
	for start := 0; start <= len(text); {
		index := strings.Index(text[start:], word)
		if index < 0 {
			return false
		}
		index += start
		end := index + len(word)
		previous, _ := utf8.DecodeLastRuneInString(text[:index])
		next, _ := utf8.DecodeRuneInString(text[end:])
		before := index == 0 || unicode.IsSpace(previous)
		after := end == len(text) || unicode.IsSpace(next) || strings.ContainsRune(",;:.", next)
		if before && after {
			return true
		}
		start = index + 1
	}
	return false
}

// opensWithMarker: the text begins with the marker as a word of its own.
func opensWithMarker(text, marker string) bool {
	rest, found := strings.CutPrefix(strings.TrimLeft(text, " \t\n\v\f\r"), marker)
	return found && (rest == "" || strings.ContainsAny(rest[:1], " \t\n\v\f\r"))
}

// customItemLabel: a class's own item counter (papers5/1242's \algitem: \refstepcounter{MYitem} and a
// bold "3)") records an anchor of that counter ending in the item's number (MYitem.1.3), not
// hyperref's Item.N.
func customItemLabel(label rec.Map) bool {
	counter, number := toS(label["counter"]), toS(rec.Fetch(label, "number"))
	return counter != "" && number != "" && !standardCounters[counter] &&
		strings.HasPrefix(toS(label["anchor"]), counter+".") && strings.HasSuffix(toS(label["anchor"]), "."+number)
}

// standardCounters are LaTeX's own counters, whose labels name their own elements.
var standardCounters = map[string]bool{"part": true, "chapter": true, "section": true, "subsection": true,
	"subsubsection": true, "paragraph": true, "subparagraph": true, "equation": true, "figure": true, "table": true,
	"footnote": true, "mpfootnote": true, "enumi": true, "enumii": true, "enumiii": true, "enumiv": true, "page": true}

// algorithmicLineNumber is the number algorithmic prints before a numbered line ("15: ").
var algorithmicLineNumber = regexp.MustCompile(`\A` + rubySpace + `*\d+:` + rubySpace + `+`)

// opensWithDelimitedMarker: the text begins with the number closed by ")" or "." as a word of
// its own ("3) Based on"), also after the line number of an algorithmic line ("15: 5) Update").
func opensWithDelimitedMarker(text, marker string) bool {
	if number := algorithmicLineNumber.FindString(text); number != "" && opensWithDelimitedMarker(text[len(number):], marker) {
		return true
	}
	for _, closing := range []string{")", "."} {
		if opensWithMarker(text, marker+closing) {
			return true
		}
	}
	return false
}
