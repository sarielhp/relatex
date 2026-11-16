package html

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// Accessible names of cross-references (plan accessibility §7.2.2, decision 111): a reference's
// text is its number ("3.2"), and the word of its kind ("Theorem") sits in the sentence before it,
// out of reach in a screen reader's links list (WCAG 2.4.4). The name is "<Kind> <link text>", so
// it holds the visible text (WCAG 2.5.3), given only when the sentence does not say the word and
// the link's own text does not. The fixed kinds are English; a theorem's title is the author's word.

// fixedWords are the English names of the kinds no author names.
var fixedWords = map[string]string{
	"display_math": "Equation", "math_row": "Equation",
	"figure_float": "Figure", "table_float": "Table",
	"heading": "Section", "section": "Section", "list_item": "Item",
}

// kindWords are the words and abbreviations that already say a kind in the sentence.
var kindWords = []string{
	"Thm", "Theorem", "Lem", "Lemma", "Prop", "Proposition", "Cor", "Corollary", "Def", "Definition",
	"Sec", "Section", "§", "Eq", "Eqs", "Equation", "Equations", "Fig", "Figs", "Figure", "Figures",
	"Tab", "Table", "Tables", "Chapter", "Ch", "Appendix", "App", "Item",
}

// referenceName is a cross-reference's accessible name: "<Kind> <link text>", or "" when its
// target's kind is unknown or a theorem without a title, when the document's language is not
// English (the fixed kinds are English), when the link's text already holds the word, or when the
// text span before it ends with the word or a known abbreviation. previous is that span's text,
// "" when it is no text span (a formula says nothing).
func (p *Paper) referenceName(span rec.Map, previous string) string {
	text := strings.Join(strings.Fields(rec.FetchString(span, "text")), " ")
	if text == "" {
		return ""
	}
	kind, fixed := p.referenceKind(rec.FetchString(span, "target"))
	if kind == "" {
		return ""
	}
	if fixed && !p.english() {
		return ""
	}
	if strings.Contains(strings.ToLower(text), strings.ToLower(kind)) {
		return ""
	}
	if saysKind(previous, kind) {
		return ""
	}
	return kind + " " + text
}

// referenceKind is the kind of the node target and its nearest ancestor: the theorem's title
// (fixed false), or a fixed English word (fixed true), or "" when nothing names it. A theorem
// without a title names nothing.
func (p *Paper) referenceKind(target string) (kind string, fixed bool) {
	parents := p.parentMap()
	id := target
	for seen := 0; id != "" && seen <= len(p.nodes); seen++ {
		value, ok := p.nodes[id]
		if !ok {
			return "", false
		}
		node := rec.AsMap(value)
		if word, ok := fixedWords[rec.FetchString(node, "type")]; ok {
			return word, true
		}
		if rec.FetchString(node, "type") == "theorem" {
			semantics, _ := node["semantics"].(rec.Map)
			title, _ := semantics["title"].(string)
			title = strings.TrimRight(strings.TrimSpace(title), ".:")
			if title == "" {
				return "", false
			}
			return title, false
		}
		id = parents[id]
	}
	return "", false
}

// saysKind is whether the text span before a link ends with the kind's word or a known
// abbreviation (case-insensitively, without a final dot), so the sentence already says it.
func saysKind(previous, kind string) bool {
	token := strings.TrimRightFunc(previous, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '(' || r == '[' || r == '~' || r == '\u00a0'
	})
	token = strings.TrimSuffix(token, ".")
	if token == "" {
		return false
	}
	if hasWordSuffix(token, kind) {
		return true
	}
	for _, word := range kindWords {
		if hasWordSuffix(token, word) {
			return true
		}
	}
	return false
}

// hasWordSuffix is whether text ends with word as a whole word: a letter or digit before it
// keeps them apart ("March" does not end with "Ch", "(Definition" does).
func hasWordSuffix(text, word string) bool {
	lower, suffix := strings.ToLower(text), strings.ToLower(word)
	if !strings.HasSuffix(lower, suffix) {
		return false
	}
	before := lower[:len(lower)-len(suffix)]
	if before == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(before)
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// english is whether the document's language is English, or says none (the fixed kind words are
// English; a theorem's title works in any language).
func (p *Paper) english() bool {
	language, _ := p.document["language"].(rec.Map)
	tag, _ := language["tag"].(string)
	tag = strings.ToLower(tag)
	return tag == "" || strings.HasPrefix(tag, "en")
}

// parentMap is the document's child -> parent map, read once.
func (p *Paper) parentMap() map[string]string {
	if p.parents != nil {
		return p.parents
	}
	parents := map[string]string{}
	for id, value := range p.nodes {
		children, _ := rec.AsMap(value)["children"].([]any)
		for _, child := range children {
			if name, ok := child.(string); ok {
				parents[name] = id
			}
		}
	}
	p.parents = parents
	return parents
}
