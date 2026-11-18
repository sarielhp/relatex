package html

import (
	"maps"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/rec"
)

// spans writes a content list, each span knowing the author's fill behind it (contrast.Behind,
// decision 099): text on a fill takes its colour against the fill, not against the theme's page.
// A table's cells are lists of their own: innermost first, on their own colortbl fill
// (contrast.CellFill, set in cellFill by cellContent), else the fill of the cell around their
// table, else the page's canvas (decision 107). A fill rule is marked (data-fill-box) so the
// stylesheet paints it with its theme's counterpart (fillInksCSS).
func (p *Paper) spans(content []any) string {
	outer := p.fill
	defer func() { p.fill = outer }()
	outerPrevious := p.previous
	defer func() { p.previous = outerPrevious }()
	base := p.cellFill
	if base == "" {
		base = contrast.Canvas(p.document)
	}
	fills, rules := contrast.BehindOn(content, base, p.pdfBackdrops()), contrast.Behind(content)
	title := p.runin
	p.runin = runinTitle{}
	var builder strings.Builder
	previous := ""
	joins := joinRun{kept: p.identityKept}
	for index, value := range content {
		builder.WriteString(title.edge(index))
		span := rec.AsMap(value)
		p.fill = fills[index]
		p.previous = previous
		if span["type"] == "table_grid" {
			p.fill = base // a table starts on the list's base, not on a fill rule before it (099)
		}
		html, split := joins.text(content, index, func(piece string, cont bool) string {
			html := p.renderSpan(rec.Merge(span, rec.Map{"text": piece}))
			if cont {
				html = strings.Replace(html, "data-kind=", "data-join-cont data-kind=", 1)
			}
			return html
		})
		if !split {
			html = p.renderSpan(span)
		}
		if span["type"] == "math" {
			html = joins.formula(content, index, html)
		}
		if span["type"] == "text" {
			previous, _ = span["text"].(string)
		} else {
			previous = ""
		}
		if fill := contrast.FillOf(span); fill != "" {
			html = strings.Replace(html, `data-kind="rule"`, `data-kind="rule" data-fill-box="`+strings.TrimPrefix(fill, "#")+`"`, 1)
		}
		if p.fill != base && rules[index] == "" && span["type"] != "table_grid" {
			// on a fill the PDF paints in its picture, not in a rule of the list: the span paints it (113)
			mark := "data-fill-box=\"" + strings.TrimPrefix(p.fill, "#") + "\" data-kind="
			if split {
				html = strings.ReplaceAll(html, "data-kind=", mark) // each piece paints on the fill
			} else {
				html = strings.Replace(html, "data-kind=", mark, 1)
			}
		}
		builder.WriteString(p.withIndexAnchors(html, span))
	}
	builder.WriteString(title.edge(len(content)))
	return builder.String()
}

// pdfBackdrops are the document's backdrops, read once.
func (p *Paper) pdfBackdrops() contrast.Backdrops {
	if p.backdrops == nil {
		backdrops := contrast.DocumentBackdrops(p.document)
		p.backdrops = &backdrops
	}
	return *p.backdrops
}

// textMarks marks a text span: the author hid it (hiddenText), or, uncoloured on a fill, names
// that fill (data-fill), whose stylesheet rule gives it the fill's black: the dark theme's ink
// would otherwise be a light one on the author's light fill. The mark is no inline colour, which
// the browser inventory would read as the span's own.
func (p *Paper) textMarks(html, color string) string {
	if color != "" || p.fill == "" {
		return p.hiddenText(html, color)
	}
	return strings.Replace(html, "data-kind=", "data-fill=\""+strings.TrimPrefix(p.fill, "#")+"\" data-kind=", 1)
}

// fillInkColor is inkColor on the fill behind the span: a declared ink as its custom property on
// that fill, black as the fill's own ink property.
func (p *Paper) fillInkColor(color string) string {
	fill := strings.TrimPrefix(p.fill, "#")
	if color == textPaintColor || strings.EqualFold(color, "#000000") {
		return "var(--ink-on-" + fill + ")"
	}
	if p.declaredInks[color] {
		return "var(--ink-" + strings.TrimPrefix(color, "#") + "-on-" + fill + ")"
	}
	return color
}

// darkInk is the dark theme's --ink (darkTheme).
const darkInk = "#e9e6df"

// fillInksCSS are the declarations of the fills and the inks on them (decision 099). The light
// theme paints a fill in the author's colour and shows an ink on it as itself or its recorded
// adjustment against the fill; the PDF's black there is the theme's ink, the PDF's paint. The
// dark theme paints a light fill as a dark box of its hue (contrast.DarkFill) and shows each ink
// on it, black as the theme's ink, lightened to read on that box and on the page around it.
// rules give the marked fills (spans) and uncoloured spans on them (textMarks) those colours.
func fillInksCSS(document rec.Map) (dark, light, rules string) {
	if _, ok := document["nodes"].(rec.Map); !ok {
		return "", "", ""
	}
	recorded := contrast.Recorded(document)
	var darkInks, lightInks, marked strings.Builder
	inks := contrast.Inks(document)
	for _, ink := range slices.Sorted(maps.Keys(inks)) {
		color, _ := contrast.Parse(ink)
		for _, fill := range contrast.FillsOf(inks[ink]) {
			name := "--ink-" + strings.TrimPrefix(ink, "#") + "-on-" + strings.TrimPrefix(fill, "#")
			shown := ink
			if adjusted, ok := recorded[contrast.Context(ink, fill)]; ok {
				shown = adjusted
			}
			background, _ := contrast.Parse(fill)
			darkInks.WriteString(name + ": " + contrast.OnDarkFill(color, background).Hex() + "; ")
			lightInks.WriteString(name + ": " + shown + "; ")
		}
	}
	themeInk, _ := contrast.Parse(darkInk)
	for _, fill := range contrast.Fills(document) {
		hex := strings.TrimPrefix(fill, "#")
		background, _ := contrast.Parse(fill)
		darkInks.WriteString("--fill-" + hex + ": " + contrast.DarkFill(background).Hex() + "; --ink-on-" + hex + ": " +
			contrast.OnDarkFill(themeInk, background).Hex() + "; ")
		lightInks.WriteString("--fill-" + hex + ": " + fill + "; --ink-on-" + hex + ": var(--ink); ")
		marked.WriteString("[data-fill-box=\"" + hex + "\"] { background: var(--fill-" + hex + ") !important; }\n" +
			"[data-fill=\"" + hex + "\"] { color: var(--ink-on-" + hex + "); }\n")
	}
	if canvas := strings.TrimPrefix(contrast.Canvas(document), "#"); canvas != "" {
		// the canvas is behind the text and its structures, which paint no tint of their own (decision 107)
		marked.WriteString("main, main .theorem, main .proof { background: var(--fill-" + canvas + "); }\n")
	}
	return darkInks.String(), lightInks.String(), marked.String()
}

// cellContent writes a rich table cell's content on the cell's colortbl fill (contrast.CellFill),
// or on the fill of the cell around its table, and returns with it the attribute that marks the
// cell as a fill box (data-fill-box, painted by fillInksCSS's rule in each theme), "" for a cell
// without a fill of its own.
func (p *Paper) cellContent(cell rec.Map) (content, fillBox string) {
	outer := p.cellFill
	defer func() { p.cellFill = outer }()
	fill := contrast.CellFill(cell)
	if fill != "" {
		p.cellFill = fill
		fillBox = "data-fill-box=\"" + strings.TrimPrefix(fill, "#") + "\" "
	}
	return p.spans(rec.FetchList(cell, "content")), fillBox
}
