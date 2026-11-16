package accessibility

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// The structural checks of the page's landmarks, named groups and visual rules (decision 095),
// read from the page as written.

var (
	groupOpener   = regexp.MustCompile(`<div\b[^>]*\bdata-structure="(?:theorem|proof)"[^>]*>`)
	groupNamed    = regexp.MustCompile(`\brole="group"[^>]*\baria-label(?:ledby)?="[^"]+"`)
	headerElement = regexp.MustCompile(`(?s)<body>\s*<a class="skip-link"[^>]*>[^<]*</a>\s*<header>(.*?)</header>`)
	contentsNav   = regexp.MustCompile(`<nav id="` + htmlkit.ContentsID + `"`)
	mainElement   = regexp.MustCompile(`<main id="` + htmlkit.MainID + `">`)
	bibliography  = regexp.MustCompile(`<section\b[^>]*\brole="doc-bibliography"[^>]*\baria-labelledby="[^"]+"`)
	bibRun        = regexp.MustCompile(`<div\b[^>]*\brole="doc-bibliography"[^>]*\baria-label="References"`)
	styleElement  = regexp.MustCompile(`(?s)<style>(.*?)</style>`)
)

// landmarkItems: the page's landmarks. The banner (a header right after the skip link) holds the
// status notice and, when the page has contents, the link to them; main is the skip link's
// target; the bibliography the document's citations place in a section of its own is a
// doc-bibliography landmark named by its heading.
func landmarkItems(document rec.Map, page []byte) []any {
	items := []any{}
	text := string(page)
	banner := headerElement.FindStringSubmatch(text)
	switch {
	case banner == nil:
		items = append(items, rec.Map{"landmark": "banner"})
	case !strings.Contains(banner[1], `role="status"`):
		items = append(items, rec.Map{"landmark": "banner", "missing": "status"})
	case contentsNav.MatchString(text) && !strings.Contains(banner[1], `href="#`+htmlkit.ContentsID+`"`):
		items = append(items, rec.Map{"landmark": "banner", "missing": "contents"})
	}
	if !mainElement.MatchString(text) {
		items = append(items, rec.Map{"landmark": "main"})
	}
	switch {
	case htmlkit.BibliographySection(document) != "":
		if !bibliography.MatchString(text) {
			items = append(items, rec.Map{"landmark": "doc-bibliography"})
		}
	case htmlkit.English(document):
		if _, _, _, run := htmlkit.BibliographyRun(document); run && !bibRun.MatchString(text) {
			items = append(items, rec.Map{"landmark": "doc-bibliography"})
		}
	}
	return items
}

// groupItems: theorem-like structures and proofs that are not named groups.
func groupItems(page []byte) []any {
	return unmarkedItems(page, groupOpener, groupNamed)
}

// styleItems: the rules of check (htmlkit.VisualRules) the page's stylesheet lacks.
func styleItems(check string, page []byte) []any {
	style := ""
	if match := styleElement.FindSubmatch(page); match != nil {
		style = string(match[1])
	}
	items := []any{}
	for _, rule := range htmlkit.MissingVisualRules(check, style) {
		items = append(items, rec.Map{"rule": rule})
	}
	return items
}

// contrastItems: the palette pairs of the page's stylesheet below the contrast WCAG asks.
func contrastItems(page []byte) []any {
	style := ""
	if match := styleElement.FindSubmatch(page); match != nil {
		style = string(match[1])
	}
	return htmlkit.ThemeContrastGaps(style)
}
