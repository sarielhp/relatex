package html

import (
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Accessible names of links whose content gives none (WCAG 2.4.4, 4.1.2; plan accessibility
// §7.2.2): a formula without alttext or a space names nothing. The name is what the formula shows
// (WCAG 2.5.3 wants the visible label inside the name), else what the link leads to, so it stays
// mechanically true: an external link's target as written, a citation link's bibliography entry.

// citationName names a citation link made of a formula or a space: what the formula shows (a
// citation number set in math), else its bibliography entry.
func (p *Paper) citationName(content string, entry rec.Map) string {
	if shown := textContent(content); shown != "" {
		return shown
	}
	return p.entryName(entry)
}

// linkName is an external link's name: the text its formula shows (WCAG 2.5.3 wants the visible
// label inside the name), else its target as a reader would say it, an e-mail address without its
// mailto: scheme.
func linkName(content, href string) string {
	if shown := textContent(content); shown != "" {
		return shown
	}
	if address, ok := strings.CutPrefix(href, "mailto:"); ok {
		return address
	}
	return href
}

var (
	markup = regexp.MustCompile(`<[^>]*>`)
	// annotations are a formula's TeX annotations (decision 100), which do not show.
	annotations = regexp.MustCompile(`(?s)<annotation[ >].*?</annotation>`)
	// invisibleOperators are U+2061–U+2064 (function application, invisible times...).
	invisibleOperators = strings.NewReplacer("\u2061", "", "\u2062", "", "\u2063", "", "\u2064", "")
)

// textContent is the text an element's HTML holds, its textContent with spaces collapsed, without
// a formula's annotation. A link's name is this text, so that the name contains the page's text
// exactly (WCAG 2.5.3): the invisible operators between a formula's letters stay, as they do in a
// browser's textContent. Without them a checker comparing the name with the page's text takes the
// letters of "https" or "et al" for separate words and finds the name lacking the visible text.
func textContent(content string) string {
	shown := stdhtml.UnescapeString(markup.ReplaceAllString(annotations.ReplaceAllString(content, ""), ""))
	return strings.Join(strings.Fields(shown), " ")
}

// visibleText is the text an element's HTML shows: a formula's characters in reading order,
// without its annotation or invisible operators (a heading's or contents entry's name).
func visibleText(content string) string {
	return strings.Join(strings.Fields(invisibleOperators.Replace(textContent(content))), " ")
}

// nameless is whether a link made of this span would have no accessible name: a formula, or text
// that is only spaces.
func nameless(span rec.Map) bool {
	switch span["type"] {
	case "math":
		return true
	case "text":
		text, _ := span["text"].(string)
		return strings.TrimSpace(text) == ""
	}
	return false
}

// entryNameLength is the most characters of an entry's text a citation link's name repeats.
const entryNameLength = 80

// entryName names a citation link by its bibliography entry: "Reference: " and the entry's text,
// cut at a word boundary within entryNameLength characters; the entry's key when it has no text.
func (p *Paper) entryName(entry rec.Map) string {
	text := p.entryText(entry)
	if text == "" {
		return "Reference " + rec.ToS(entry["key"])
	}
	return "Reference: " + cutEntry(text)
}

// labelName names a citation link whose text is only its entry's printed label ("12", "DJ17"),
// which alone does not say what it leads to (WCAG 2.4.4; decision 168): "Reference <label>: " and
// the entry's text without its leading label, cut as entryName cuts it, or "Reference <label>"
// when the entry has no other text. The name starts with the visible label (WCAG 2.5.3). Text
// that is not the bare label (author-year text, a note), a glyph read by its name, and a document
// not in English (the word is English) get none.
func (p *Paper) labelName(span, link, entry rec.Map) string {
	if span["type"] != "text" || !p.english() {
		return ""
	}
	if _, glyph := span["glyph_name"]; glyph {
		return ""
	}
	label := strings.Join(strings.Fields(rec.FetchString(span, "text")), " ")
	number, _ := link["number"].(string)
	if label == "" || label != number {
		return ""
	}
	body := withoutLabel(p.entryText(entry), label)
	if body == "" {
		return "Reference " + label
	}
	return "Reference " + label + ": " + cutEntry(body)
}

// entryText is the text of an entry's first block, its spaces collapsed.
func (p *Paper) entryText(entry rec.Map) string {
	var builder strings.Builder
	for _, value := range rec.FetchList(p.node(rec.FetchList(entry, "blocks")[0]), "content") {
		if text, ok := rec.AsMap(value)["text"].(string); ok {
			builder.WriteString(text)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

// withoutLabel is an entry's text without the label it opens with: "[12] ", "(12) ", "12. " or
// "12 " (the name already says the label once).
func withoutLabel(text, label string) string {
	for _, form := range []string{"[" + label + "]", "(" + label + ")", label + ".", label} {
		rest, found := strings.CutPrefix(text, form)
		if found && (rest == "" || strings.HasPrefix(rest, " ")) {
			return strings.TrimPrefix(rest, " ")
		}
	}
	return text
}

// cutEntry is an entry's text cut at a word boundary within entryNameLength characters.
func cutEntry(entry string) string {
	text := []rune(entry)
	if len(text) <= entryNameLength {
		return entry
	}
	cut := string(text[:entryNameLength])
	if space := strings.LastIndex(cut, " "); space > 0 {
		cut = cut[:space]
	}
	return cut + "…"
}
