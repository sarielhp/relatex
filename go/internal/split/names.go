package split

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/pagetree"
)

// ContentsName is the contents page, the entry of a split book.
const ContentsName = "index.html"

// namer gives the units their file names: front.html, part-N.html, chapter-NN.html for a numbered
// chapter, appendix-x.html for a lettered one, and a name from the title for the others
// (preface.html, bibliography.html), each name once.
type namer struct {
	taken map[string]bool
	parts int
}

func newNamer() *namer {
	return &namer{taken: map[string]bool{ContentsName: true}}
}

func (n *namer) name(p *piece) (name, title string) {
	switch p.kind {
	case "front":
		return n.unique("front"), "Front matter"
	case "part":
		n.parts++
		return n.unique(fmt.Sprintf("part-%d", n.parts)), headingTitle(p.part, "Part "+strconv.Itoa(n.parts))
	}
	section := p.content[0]
	for _, candidate := range p.content {
		if candidate.Name == "section" {
			section = candidate
			break
		}
	}
	headings := leadingHeadings(section)
	title = headingTitle(section, "Untitled")
	if len(headings) == 0 {
		return n.unique(slug(title)), title
	}
	number := chapterNumber(headings[0].TextContent())
	switch {
	case isNumber(number):
		value, _ := strconv.Atoi(number)
		return n.unique(fmt.Sprintf("chapter-%02d", value)), title
	case isLetter(number):
		return n.unique("appendix-" + strings.ToLower(number)), title
	}
	return n.unique(slug(title)), title
}

func (n *namer) unique(stem string) string {
	name := stem + ".html"
	for count := 2; n.taken[name]; count++ {
		name = fmt.Sprintf("%s-%d.html", stem, count)
	}
	n.taken[name] = true
	return name
}

// headingTitle is the text of the headings a section opens with, or the fallback.
func headingTitle(section *pagetree.Node, fallback string) string {
	var words []string
	for _, heading := range leadingHeadings(section) {
		words = append(words, strings.Fields(heading.TextContent())...)
	}
	if len(words) == 0 {
		return fallback
	}
	return strings.Join(words, " ")
}

func isNumber(text string) bool {
	if text == "" || len(text) > 4 {
		return false
	}
	for _, c := range text {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isLetter(text string) bool {
	return len(text) == 1 && text[0] >= 'A' && text[0] <= 'Z'
}

// slug is a file stem from a title: lower-case ASCII letters and digits joined by hyphens, at most
// about forty characters; "section" when nothing is left.
func slug(title string) string {
	var words []string
	length := 0
	for _, word := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if length > 0 && length+len(word) > 40 {
			break
		}
		words = append(words, word)
		length += len(word) + 1
	}
	if len(words) == 0 {
		return "section"
	}
	stem := strings.Join(words, "-")
	if stem == "index" {
		stem = "book-index"
	}
	return stem
}

// chapterNumber is the number a chapter heading opens with: the whole heading ("1", "A"), its
// first word when a period ends it ("1. Introduction", "A. Proofs") or when it is a number
// ("1 Introduction"), or the number or letter after a word such as the standard classes'
// "Chapter 1" and "Appendix A" (in any language: "Kapitel 1"); "" otherwise.
func chapterNumber(heading string) string {
	words := strings.Fields(heading)
	switch {
	case len(words) == 1:
		return words[0]
	case len(words) > 1 && strings.HasSuffix(words[0], "."):
		return strings.TrimSuffix(words[0], ".")
	case len(words) > 1 && isNumber(words[0]):
		return words[0]
	case len(words) > 1 && isWord(words[0]):
		number := strings.TrimRight(words[1], ".:")
		if isNumber(number) || isLetter(number) {
			return number
		}
	}
	return ""
}

// isWord is a word of letters only, such as "Chapter".
func isWord(text string) bool {
	for _, c := range text {
		if !unicode.IsLetter(c) {
			return false
		}
	}
	return text != ""
}
