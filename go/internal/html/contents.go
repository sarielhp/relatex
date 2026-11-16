package html

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// headingLevels are the ranks of the sectioning counters; the table of contents starts at the
// highest rank the paper uses (a paper's sections, a book's parts or chapters).
var headingLevels = map[string]int{"part": 0, "chapter": 1, "section": 2, "subsection": 3, "subsubsection": 4, "paragraph": 5}

// headings are the main text's headings with their depth and link. The lines of one heading
// (a chapter's "Chapter 1" above its title) are one entry.
func (p *Paper) headings() (levels []int, links []string) {
	var entities []any
	var texts []string
	var ids []any
	previous := -2
	for index, id := range rec.FetchList(p.document, "main_block_order") {
		node := p.node(id)
		if node["type"] != "heading" {
			continue
		}
		text := p.headingText(node)
		semantics, _ := node["semantics"].(rec.Map)
		entity, known := semantics["entity"]
		if known && previous == index-1 && len(entities) > 0 && rec.Equal(entities[len(entities)-1], entity) {
			texts[len(texts)-1] += " " + text
			previous = index
			continue
		}
		previous = index
		level, ranked := headingLevels[rec.AsString(semantics["counter"])]
		if !ranked {
			level = headingLevels["section"]
		}
		levels = append(levels, level)
		entities = append(entities, entity)
		texts = append(texts, text)
		ids = append(ids, id)
	}
	top := slices.Min(append(slices.Clone(levels), headingLevels["section"]))
	for index := range levels {
		levels[index] -= top - 1
		links = append(links, "<a href=\"#"+toS(ids[index])+"\""+p.contentsName(texts[index], ids[index])+">"+htmlkit.Escape(texts[index])+"</a>")
	}
	return levels, links
}

// headingText is the text a contents entry shows for a heading: its words and, where the heading
// holds a formula, the characters the formula shows (the text a reader sees in the page).
func (p *Paper) headingText(node rec.Map) string {
	var text strings.Builder
	for _, value := range rec.FetchList(node, "content") {
		span := rec.AsMap(value)
		if shown, ok := span["text"]; ok {
			text.WriteString(rec.AsString(shown))
		} else if span["type"] == "math" {
			text.WriteString(visibleText(p.renderMath(rec.FetchString(span, "node"))))
		}
	}
	return text.String()
}

// contentsName is the aria-label of a contents entry whose heading has no text, only a formula
// (WCAG 2.4.4): the text the heading's formula shows. An entry with text needs none.
func (p *Paper) contentsName(text string, id any) string {
	if strings.TrimSpace(text) != "" {
		return ""
	}
	name := visibleText(p.spans(rec.FetchList(p.node(id), "content")))
	if name == "" {
		name = "Heading"
	}
	return " aria-label=\"" + htmlkit.Escape(name) + "\""
}

// contentsLink is the line above the text that leads to the table of contents.
func (p *Paper) contentsLink() string {
	if levels, _ := p.headings(); len(levels) == 0 {
		return ""
	}
	return `<p class="page-links"><a href="#` + htmlkit.ContentsID + `">Contents</a></p>`
}

// contents is the table of contents after the text: the headings as nested lists by depth.
func (p *Paper) contents() string {
	levels, links := p.headings()
	if len(levels) == 0 {
		return ""
	}
	var list strings.Builder
	depth := 0
	for index, link := range links {
		level := min(levels[index], depth+1)
		for depth > level {
			list.WriteString("</li></ol>")
			depth--
		}
		if depth == level {
			list.WriteString("</li>")
		} else {
			list.WriteString("<ol>")
			depth++
		}
		list.WriteString("<li>" + link)
	}
	for depth > 0 {
		list.WriteString("</li></ol>")
		depth--
	}
	return `<nav id="` + htmlkit.ContentsID + `" aria-label="Contents"><h2>Contents</h2>` + list.String() + "</nav>"
}
