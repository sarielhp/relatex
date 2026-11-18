package html

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/rec"
)

// openingMarks are the characters after which TeX never breaks a line when a formula follows
// ("($x$)", "“$x$”").
const openingMarks = "([{“‘`«¿¡"

// joinCSS: a formula is an atomic box to the browser, which may break a line on either side of it
// (a U+2060 word joiner does not stop that); TeX breaks there only at a space. The formula and the
// punctuation stuck to it sit in one span that never wraps. The span is no text of the page and holds
// no text of its own, so extraction, copy and paste and speech read the same words in the same order.
const joinCSS = "[data-join] { white-space: nowrap; }\n"

// joinSafeKeys are the fields of a text span that may be cut in two pieces at a space: the pieces
// render alike. Links, citations, notes and tags keep their span whole.
var joinSafeKeys = map[string]bool{"type": true, "text": true, "style": true, "color": true, "lang": true}

// joinedText is the text of a span a formula's punctuation may be cut from, else "" and false.
func joinedText(content []any, index int, kept func(rec.Map) bool) (string, bool) {
	if index < 0 || index >= len(content) {
		return "", false
	}
	span := rec.AsMap(content[index])
	if span["type"] != "text" || (kept != nil && kept(span)) {
		return "", false
	}
	for key := range span {
		if !joinSafeKeys[key] {
			return "", false
		}
	}
	text, _ := span["text"].(string)
	return text, text != ""
}

func isMath(content []any, index int) bool {
	return index >= 0 && index < len(content) && rec.AsMap(content[index])["type"] == "math"
}

// startsJoined: the text at index starts right after a formula with no space ("$S$. However").
func startsJoined(content []any, index int, kept func(rec.Map) bool) bool {
	text, ok := joinedText(content, index, kept)
	if !ok || !isMath(content, index-1) {
		return false
	}
	first, _ := utf8.DecodeRuneInString(text)
	return !unicode.IsSpace(first)
}

// endsJoined: the text at index ends with an opening mark right before a formula ("($S$").
func endsJoined(content []any, index int, kept func(rec.Map) bool) bool {
	text, ok := joinedText(content, index, kept)
	if !ok || !isMath(content, index+1) {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(text)
	return strings.ContainsRune(openingMarks, last)
}

// joinRun is how the loop over a paragraph's spans keeps each formula with its punctuation: the
// nowrap span is open between the first piece that belongs to it and the last.
type joinRun struct {
	open bool
	// kept reports a span that something finds by its identity (an external link, an index
	// entry); a cut copy would lose it, so such a span stays whole. nil keeps none.
	kept func(rec.Map) bool
}

// text renders the text span at index: whole when nothing joins to it, else cut at the space after
// its leading punctuation and at the space before its trailing opening mark, with the wrapper
// opened or closed around the pieces that belong to a formula. render draws one piece; every piece after
// the first of a span is a continuation (data-join-cont), part of the same styled run for the checks.
func (r *joinRun) text(content []any, index int, render func(piece string, cont bool) string) (string, bool) {
	after, before := startsJoined(content, index, r.kept), endsJoined(content, index, r.kept)
	if !after && !before {
		return "", false
	}
	text, _ := joinedText(content, index, r.kept)
	cutAfter, cutBefore := 0, len(text)
	if after {
		cutAfter = strings.IndexFunc(text, unicode.IsSpace)
		if cutAfter < 0 {
			cutAfter = len(text)
		}
	}
	if before {
		cutBefore = strings.LastIndexFunc(text, unicode.IsSpace) + 1
	}
	var out strings.Builder
	if cutBefore < cutAfter { // one unbroken word that both follows a formula and precedes one
		return render(text, false), true
	}
	pieces := 0
	draw := func(piece string) string {
		pieces++
		return render(piece, pieces > 1)
	}
	if after {
		out.WriteString(draw(text[:cutAfter]))
		if r.open {
			out.WriteString("</span>")
			r.open = false
		}
	}
	if cutAfter < cutBefore {
		out.WriteString(draw(text[cutAfter:cutBefore]))
	}
	if before {
		out.WriteString("<span data-join>" + draw(text[cutBefore:]))
		r.open = true
	}
	return out.String(), true
}

// formula opens the wrapper before a formula whose next text joins to it, and closes it after the
// formula when an opening mark opened it and nothing joins after.
func (r *joinRun) formula(content []any, index int, html string) string {
	next := startsJoined(content, index+1, r.kept)
	if !r.open && next {
		r.open = true
		return "<span data-join>" + html
	}
	if r.open && !next {
		r.open = false
		return html + "</span>"
	}
	return html
}

// identityKept: the span is one an external link, an index link or an index anchor finds by its
// identity.
func (p *Paper) identityKept(span rec.Map) bool {
	if !p.described {
		p.descriptors, p.described = extlinks.Descriptors(p.document, false), true
	}
	for _, link := range p.descriptors {
		if _, ok := link["block"]; ok && rec.Identity(extlinks.Resolve(p.document, link)) == rec.Identity(span) {
			return true
		}
	}
	id := rec.Identity(span)
	return len(p.index().links[id]) > 0 || len(p.index().anchors[id]) > 0
}
