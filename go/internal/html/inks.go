package html

import (
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/rec"
)

// inkColor is a paint color as CSS: the text paint is the theme's ink (the light theme's ink
// is that paint), a declared ink of the document is its custom property (inksCSS: itself or its
// recorded adjustment in the light theme, raised to the contrast it needs in the dark one), any
// other color stays itself.
func (p *Paper) inkColor(color string) string {
	if p.declaredInks == nil {
		p.declaredInks = map[string]bool{}
		for _, ink := range declaredInks(p.document) {
			p.declaredInks[ink] = true
		}
	}
	if p.fill != "" {
		return p.fillInkColor(color)
	}
	// Black, however it is spelled (a table's paint is recorded as #000000), is the page's own ink.
	if color == textPaintColor || strings.EqualFold(color, "#000000") {
		return "var(--ink)"
	}
	if p.declaredInks[color] {
		return "var(--ink-" + strings.TrimPrefix(color, "#") + ")"
	}
	return color
}

// inks are the document's text inks (decision 070), #rrggbb.
func inks(document rec.Map) []string {
	paint, ok := document["text_paint"].(rec.Map)
	if !ok {
		return nil
	}
	values := []string{}
	list, _ := paint["inks"].([]any) // a paint without colored text declares no inks
	for _, value := range list {
		values = append(values, rec.ToS(value))
	}
	return values
}

// declaredInks are the inks the page declares as custom properties: the text paint's inks and
// every color of a text span, sorted.
func declaredInks(document rec.Map) []string {
	set := map[string]bool{}
	for _, ink := range inks(document) {
		set[ink] = true
	}
	if _, ok := document["nodes"].(rec.Map); ok {
		for ink := range contrast.Inks(document) {
			set[ink] = true
		}
	}
	values := make([]string, 0, len(set))
	for ink := range set {
		values = append(values, ink)
	}
	sort.Strings(values)
	return values
}

// inksCSS declares the document's inks. The light theme shows the PDF's colors, except an ink
// whose adjustment for contrast the document records (§7.4 decision 1); the dark theme shows each
// ink raised by the smallest lightness change that reads at 4.5:1 on its backgrounds.
func inksCSS(document rec.Map) string {
	values := declaredInks(document)
	darkFills, lightFills, fillRules := fillInksCSS(document)
	if len(values) == 0 && darkFills == "" {
		return ""
	}
	backgrounds := map[string][]string{}
	if _, ok := document["nodes"].(rec.Map); ok {
		backgrounds = contrast.Inks(document)
	}
	recorded := contrast.Recorded(document)
	hidden := map[string]bool{}
	if _, ok := document["nodes"].(rec.Map); ok {
		hidden = contrast.Hidden(document)
	}
	var dark, light strings.Builder
	dark.WriteString(darkFills)
	light.WriteString(lightFills)
	for _, ink := range values {
		name := "--ink-" + strings.TrimPrefix(ink, "#")
		shownDark := contrast.Dark.Shown(ink, contrast.PageKinds(backgrounds[ink]))
		if hidden[ink] {
			shownDark = "transparent" // the author hid it: invisible in both themes
		}
		dark.WriteString(name + ": " + shownDark + "; ")
		shown := ink
		if adjusted, ok := recorded[ink]; ok {
			shown = adjusted
		}
		light.WriteString(name + ": " + shown + "; ")
	}
	return ":root { " + dark.String() + "}\n:root[data-theme=\"light\"] { " + light.String() + "}\n" +
		"@media (prefers-color-scheme: light) { :root:not([data-theme=\"dark\"]) { " + light.String() + "} }\n" + fillRules
}

// hiddenText marks a text the author hid (an ink invisible on the PDF page, contrast.Hidden) as
// hidden from assistive technology too: the page keeps it invisible, as the PDF does.
func (p *Paper) hiddenText(html, color string) string {
	if color == "" || p.fill != "" { // text on an author's fill is painted: never hidden
		return html
	}
	if p.hiddenInks == nil {
		p.hiddenInks = map[string]bool{}
		if _, ok := p.document["nodes"].(rec.Map); ok {
			p.hiddenInks = contrast.Hidden(p.document)
		}
	}
	if !p.hiddenInks[color] {
		return html
	}
	// a link inside hidden text leaves the tab order too: hidden content holds nothing focusable
	// (WCAG 4.1.2, axe aria-hidden-focus)
	html = strings.ReplaceAll(html, "<a ", "<a tabindex=\"-1\" ")
	return strings.Replace(html, "data-kind=", "aria-hidden=\"true\" data-kind=", 1)
}
