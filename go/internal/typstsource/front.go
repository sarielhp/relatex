package typstsource

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Front matter of the source-faithful writer: the title, the author grid and the blocks they
// replace.

func (e *exporter) indexFrontmatter() {
	front, _ := e.document["frontmatter"].(rec.Map)
	e.abstractBlocks = stringList(front["abstract_blocks"])
	e.frontBlocks = []string{}
	for _, id := range children(e.node(rec.Fetch(e.document, "root"))) {
		if e.node(id)["type"] == "section" {
			break
		}
		e.frontBlocks = append(e.frontBlocks, id)
	}
	e.titleBlocks = e.findTitleBlocks(comparableText(toS(front["title"])))
	for _, id := range e.titleBlocks {
		e.consumed[id] = true
	}
	for _, id := range e.frontBlocks {
		if abstractHeading.MatchString(rec.Strip(plain(e.node(id)))) {
			if len(e.abstractBlocks) > 0 {
				e.consumed[id] = true
			}
			break
		}
	}
	e.indexAuthors(maps(front["authors"]))
}

func stringList(value any) []string {
	result := []string{}
	for _, item := range list(value) {
		result = append(result, toS(item))
	}
	return result
}

// findTitleBlocks: the title as one front block, or a few consecutive ones (a title broken
// over lines).
func (e *exporter) findTitleBlocks(title string) []string {
	if title == "" {
		return []string{}
	}
	for width := 1; width <= 3; width++ {
		for start := 0; start+width <= len(e.frontBlocks); start++ {
			run := e.frontBlocks[start : start+width]
			var joined strings.Builder
			for _, id := range run {
				joined.WriteString(plain(e.node(id)))
			}
			if comparableText(joined.String()) == title {
				return slices.Clone(run)
			}
		}
	}
	return []string{}
}

// indexAuthors: the blocks printing the authors (names, affiliations, emails, "and" and
// affiliation marks, nothing else) give way to an author block written from the front matter.
func (e *exporter) indexAuthors(authors []rec.Map) {
	vocabulary := []string{}
	names := []string{}
	for _, author := range authors {
		vocabulary = append(vocabulary, toS(author["name"]))
		vocabulary = append(vocabulary, stringList(author["affiliations"])...)
		vocabulary = append(vocabulary, stringList(author["emails"])...)
		names = append(names, wordPattern.FindAllString(downcase(toS(author["name"])), -1)...)
	}
	known := map[string]bool{"and": true}
	for _, word := range wordPattern.FindAllString(downcase(strings.Join(vocabulary, " ")), -1) {
		known[word] = true
	}
	e.authors = authors
	e.authorBlocks = []string{}
	for _, id := range e.frontBlocks {
		if slices.Contains(e.titleBlocks, id) || slices.Contains(e.abstractBlocks, id) {
			continue
		}
		words := blockWords(e.node(id))
		all := len(words) > 0
		shared := false
		for _, word := range words {
			all = all && (known[word] || authorWord.MatchString(word))
			shared = shared || slices.Contains(names, word)
		}
		if all && shared {
			e.authorBlocks = append(e.authorBlocks, id)
			e.consumed[id] = true
		}
	}
}

func blockWords(node rec.Map) []string {
	texts := []string{}
	for _, span := range content(node) {
		spans := []rec.Map{span}
		if span["type"] == "table_grid" {
			spans = []rec.Map{}
			for _, cell := range maps(dig(span, "grid", "cells")) {
				spans = append(spans, content(cell)...)
			}
		}
		for _, item := range spans {
			if item["type"] == "text" {
				texts = append(texts, rec.FetchString(item, "text"))
			}
		}
	}
	return wordPattern.FindAllString(downcase(strings.Join(texts, " ")), -1)
}

func comparableText(text string) string {
	var builder strings.Builder
	for _, char := range downcase(text) {
		if letterOrNumber(char) {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

// frontMatter: the title, with the notes no text calls (a class's title notes) as its
// footnotes.
func (e *exporter) frontMatter() string {
	// The title is written first: a note it calls (Beamer's \title{…\footnote{…}}) is written
	// there, not again among the uncalled ones.
	titles := []string{}
	for _, id := range e.titleBlocks {
		node := e.node(id)
		titles = append(titles, rec.Strip(e.inline(content(node), node, 0, false)))
	}
	var notes strings.Builder
	for _, id := range stringList(e.document["note_order"]) {
		if !e.footnotesWritten[id] {
			notes.WriteString(e.footnote(e.node(id)))
		}
	}
	if len(e.titleBlocks) == 0 {
		parts := []string{}
		if notes.Len() > 0 {
			parts = append(parts, notes.String())
		}
		if authors, ok := e.authorGrid(); ok && authors != "" {
			parts = append(parts, authors)
		}
		return strings.Join(parts, "\n\n")
	}
	parts := []string{"#align(center, text(size: 17pt, strong[" + strings.Join(titles, " ") + notes.String() + "]))"}
	if authors, ok := e.authorGrid(); ok {
		parts = append(parts, authors)
	}
	return strings.Join(parts, "\n\n")
}

type authorDetail struct {
	email bool
	line  string
}

// authorGrid: one column per author, at most three a row: name, affiliations, emails.
// Affiliations and emails every author shares (Google Research, {a,b}@host) print once,
// under the grid.
func (e *exporter) authorGrid() (string, bool) {
	if len(e.authorBlocks) == 0 {
		return "", false
	}
	details := make([][]authorDetail, len(e.authors))
	for index, author := range e.authors {
		for _, line := range stringList(author["affiliations"]) {
			details[index] = append(details[index], authorDetail{false, line})
		}
		for _, line := range stringList(author["emails"]) {
			details[index] = append(details[index], authorDetail{true, line})
		}
	}
	shared := []authorDetail{}
	if len(e.authors) > 1 {
		shared = intersection(details)
	}
	cells := make([]string, len(e.authors))
	for index, author := range e.authors {
		parts := []string{escape(toS(author["name"]))}
		for _, detail := range details[index] {
			if !slices.Contains(shared, detail) {
				parts = append(parts, formatDetail(detail))
			}
		}
		cells[index] = "[" + strings.Join(parts, ` \ `) + "]"
	}
	grid := fmt.Sprintf("#align(center, grid(columns: %d, column-gutter: 2em, row-gutter: 1em, %s))", min(len(cells), 3),
		strings.Join(cells, ", "))
	if len(shared) == 0 {
		return grid, true
	}
	lines := make([]string, len(shared))
	for index, detail := range shared {
		lines[index] = formatDetail(detail)
	}
	return grid + "\n\n#align(center)[" + strings.Join(lines, ` \ `) + "]", true
}

// intersection is details.reduce(:&): the first list's distinct details in every list.
func intersection(details [][]authorDetail) []authorDetail {
	result := []authorDetail{}
	for _, detail := range details[0] {
		if slices.Contains(result, detail) {
			continue
		}
		everywhere := true
		for _, other := range details[1:] {
			everywhere = everywhere && slices.Contains(other, detail)
		}
		if everywhere {
			result = append(result, detail)
		}
	}
	return result
}

func formatDetail(detail authorDetail) string {
	if !detail.email {
		return escape(detail.line)
	}
	return "#link(" + typstString("mailto:"+detail.line) + ")[" + escape(detail.line) + "]"
}
