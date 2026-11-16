package pipeline

import (
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// The files a published page loads: its scripts and images (src), its stylesheets (link href)
// and the fonts and pictures its stylesheet names (url()). Links a reader follows (a href) are
// not loaded with the page; the book split checks its own links between files (split.Check).
var (
	sourceAttribute = regexp.MustCompile(`\ssrc="([^"]*)"`)
	linkElement     = regexp.MustCompile(`<link\b[^>]*\shref="([^"]*)"`)
	styleURL        = regexp.MustCompile(`url\(\s*(?:"([^"]*)"|'([^']*)'|([^"')\s]*))\s*\)`)
	styleComment    = regexp.MustCompile(`(?s)/\*.*?\*/`)
	styleText       = regexp.MustCompile(`(?s)<style\b[^>]*>(.*?)</style>|\sstyle="([^"]*)"`)
	urlScheme       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// pageReferences are the relative files the page text loads, as paths relative to the page.
func pageReferences(text string) []string {
	found := []string{}
	for _, pattern := range []*regexp.Regexp{sourceAttribute, linkElement} {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			found = append(found, html.UnescapeString(match[1]))
		}
	}
	for _, style := range styleText.FindAllStringSubmatch(text, -1) {
		css := styleComment.ReplaceAllString(style[1]+html.UnescapeString(style[2]), "")
		for _, match := range styleURL.FindAllStringSubmatch(css, -1) {
			found = append(found, match[1]+match[2]+match[3])
		}
	}
	references := []string{}
	for _, reference := range found {
		reference, _, _ = strings.Cut(reference, "#")
		reference, _, _ = strings.Cut(reference, "?")
		if reference == "" || urlScheme.MatchString(reference) || strings.HasPrefix(reference, "//") {
			continue
		}
		if unescaped, err := url.PathUnescape(reference); err == nil {
			reference = unescaped
		}
		if !slices.Contains(references, reference) {
			references = append(references, reference)
		}
	}
	return references
}

// missingReferences lists, as "page: file", every file a page in the directory loads that the
// publication does not hold: a page opened from the output must find its fonts, figures and
// script beside it.
func missingReferences(directory string) []string {
	pages, err := filepath.Glob(filepath.Join(directory, "*.html"))
	if err != nil {
		rec.Fail("%v", err)
	}
	missing := []string{}
	for _, page := range pages {
		text, err := os.ReadFile(page)
		if err != nil {
			rec.Fail("%v", err)
		}
		for _, reference := range pageReferences(string(text)) {
			target := filepath.Join(directory, filepath.FromSlash(reference))
			inside, err := filepath.Rel(directory, target)
			if err != nil || strings.HasPrefix(inside, "..") || !isFile(target) {
				missing = append(missing, filepath.Base(page)+": "+reference)
			}
		}
	}
	return missing
}

// checkPublishedReferences fails the publication when a page at its root, or the HTML export's
// own page, loads a file that is not there.
func checkPublishedReferences(staged string) {
	missing := missingReferences(staged)
	for _, line := range missingReferences(filepath.Join(staged, "html")) {
		missing = append(missing, "html/"+line)
	}
	if len(missing) > 0 {
		rec.Fail("Published pages load files the output does not hold: %s", strings.Join(missing, ", "))
	}
}
