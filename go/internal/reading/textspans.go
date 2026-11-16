package reading

import (
	"fmt"

	"github.com/sarielhp/relatex/internal/language"
	"github.com/sarielhp/relatex/internal/rec"
)

// Text spans: their paint, their language and how a glyph run joins its block's last span.

// recordTextPaint is one shipout record of a glyph painted in a non-black fill. A slot shipped
// in two different colors has no single color and stays uncolored.
func (e *Evidence) recordTextPaint(row rec.Map) {
	name := ruleKey{row["owner"], row["slot"]}
	color := toS(row["color"])
	if known, seen := e.textPaint[name]; seen && known != color {
		color = ""
	}
	e.textPaint[name] = color
}

// textColor is the fill color of the glyph at the active slot, "" for black.
func (e *Evidence) textColor(owner rec.Map) string {
	return e.textPaint[ruleKey{rec.Fetch(owner, "owner"), e.activeSlot}]
}

// spanPaint is what the shipout recorded for a text item's slot beyond its font: its fill color
// ("" for black) and its language tag ("" for the document's language).
type spanPaint struct{ color, lang string }

// recordTextLanguage is one shipout record of a glyph set in another language than the
// document's. A slot shipped in two languages, or in one the table cannot map, carries none.
func (e *Evidence) recordTextLanguage(row rec.Map) {
	name := ruleKey{row["owner"], row["slot"]}
	tag := language.SpanTag(toS(row["language"]), e.mainTag)
	if known, seen := e.textLanguage[name]; seen && known != tag {
		tag = ""
	}
	e.textLanguage[name] = tag
}

// spanLanguage is the language tag of the glyph at the active slot, "" for the document's.
func (e *Evidence) spanLanguage(owner rec.Map) string {
	return e.textLanguage[ruleKey{rec.Fetch(owner, "owner"), e.activeSlot}]
}

// colorValue is a span's color field: absent for black.
func colorValue(color string) any {
	if color == "" {
		return nil
	}
	return color
}

func (e *Evidence) appendText(block, item rec.Map, paint spanPaint, fallbackStyle rec.Map) {
	if space, rest, ok := foreignSpace(item); ok {
		e.appendText(block, space, paint, fallbackStyle)
		item = rest
	}
	if space, rest, ok := undecoratedSpace(item, lastSpan(block)); ok {
		e.appendText(block, space, paint, fallbackStyle)
		item = rest
	}
	color := paint.color
	text := rec.FetchString(item, "space") + rec.FetchString(item, "text")
	reference := item["reference"]
	citation := slice(item, citationFields...)
	itemStyle := e.textStyles.ItemStyle(item, "Undeclared text font")
	if itemStyle == nil {
		itemStyle = fallbackStyle
	}
	style := styleValue(itemStyle)
	last := lastSpan(block)
	if last != nil && last["glyph_name"] == nil && item["glyph_name"] == nil && last["line_break"] == nil && !titleEdge(block, item) {
		kind := rec.Fetch(last, "type")
		if (kind == "text" || kind == "reference") && rec.Equal(last["reference"], reference) && rec.Equal(last["style"], style) && rec.Equal(last["color"], colorValue(color)) && rec.Equal(last["lang"], colorValue(paint.lang)) &&
			rec.Equal(slice(last, citationFields...), citation) && e.sameTableOrigin(last) &&
			rec.Equal(last["external_link"], item["external_link"]) && rec.Equal(last["decoration"], item["decoration"]) {
			last["text"] = rec.FetchString(last, "text") + text
			e.markTagSpan(last, item)
			return
		}
	}
	// A text underline (relatex-external-math, text_underline) keeps its decoration on the span.
	span := rec.Merge(rec.Map{"type": "text", "text": text}, citation, slice(item, "external_link", "decoration"))
	if style != nil {
		span["style"] = style
	}
	if color != "" {
		span["color"] = color
	}
	if paint.lang != "" {
		span["lang"] = paint.lang
	}
	if name, named := item["glyph_name"]; named {
		span["glyph_name"] = name
	}
	if truthy(reference) {
		definition, ok := e.references.Get(reference)
		if !ok {
			rec.Fail("Dangling reference execution")
		}
		e.referenceCnt++
		span["type"], span["id"], span["reference"] = "reference", fmt.Sprintf("r%d", e.referenceCnt), reference
		span["target"] = rec.Fetch(definition, "target")
	}
	span = e.rememberTableSpan(span)
	e.markTagSpan(span, item)
	push(block, span)
}

// foreignSpace splits a word space set in another font than the glyphs on both sides of it
// (\texttt{repeat} \texttt{repeat}: a roman space between two typewriter words, papers4/0919;
// decided 2026-11-09; the collector's space_style) into a run of its own in that font, so the two
// runs stay apart. A reference's text stays one span.
// undecoratedSpace: the space before an underlined run (" \underline{word}") is set outside the
// underline box, so it is no part of the decoration; it joins the text before the run. After a
// formula (\underline{Procedure $P$ here}) the space may be inside the box: it keeps the decoration.
func undecoratedSpace(item, last rec.Map) (space, rest rec.Map, ok bool) {
	if item["decoration"] == nil || rec.FetchString(item, "space") == "" {
		return nil, item, false
	}
	if last != nil && (rec.Equal(last["decoration"], item["decoration"]) || (last["type"] != "text" && last["type"] != "reference")) {
		return nil, item, false
	}
	if truthy(item["reference"]) {
		return nil, item, false
	}
	space = rec.Merge(item, rec.Map{"text": rec.FetchString(item, "space"), "space": ""})
	delete(space, "decoration")
	delete(space, "glyph_name")
	// The space before a link's underlined text (\href{..}{\underline{debunked}}) is outside the link
	// too; inside a link (\href{..}{a \underline{b}}) it stays in it.
	if last == nil || !rec.Equal(last["external_link"], item["external_link"]) {
		delete(space, "external_link")
	}
	return space, rec.Merge(item, rec.Map{"space": ""}), true
}

func foreignSpace(item rec.Map) (space, rest rec.Map, ok bool) {
	style, has := item["space_style"]
	if !has || item["space"] != " " || truthy(item["reference"]) {
		return nil, item, false
	}
	space = rec.Merge(item, rec.Map{"text": " ", "space": "", "style": style})
	rest = rec.Merge(item, rec.Map{"space": ""})
	for _, key := range []string{"glyph_name", "space_style"} {
		delete(space, key)
		delete(rest, key)
	}
	return space, rest, true
}
