package syncdemo

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// Stats counts what the annotation reached: prose words of the HTML (outside formulas), those in a
// node the PDF's glyphs belong to, those given a source range (Exact: with columns, LineOnly: the
// line only), and the formulas of the page and those given a range.
type Stats struct {
	Words, Owned, Matched, Exact, LineOnly int
	Formulas, FormulasMapped               int
	Missed                                 []any
}

// htmlWord is a whitespace-delimited piece of a text token, by byte offsets in the unescaped text.
type htmlWord struct {
	token      int
	start, end int
	norm       string
}

var wordPiece = regexp.MustCompile(`\S+`)

var escapeText = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

type element struct {
	name string
	id   string
}

// walk is one pass over the tokens: for each prose text token its node, and the formulas.
type walk struct {
	data      *Data
	tokens    []token
	leafOf    map[int]string
	formulas  map[int]string
	unescaped map[int]string
}

// scan finds each text token's node (the nearest enclosing element the PDF's glyphs belong to) and
// each formula element of the body.
func scan(tokens []token, data *Data) *walk {
	w := &walk{data: data, tokens: tokens, leafOf: map[int]string{}, formulas: map[int]string{}, unescaped: map[int]string{}}
	var stack []element
	inBody, mathDepth := false, 0
	for i, tok := range tokens {
		switch {
		case tok.text:
			if leaf := w.nearestLeaf(stack); inBody && mathDepth == 0 && tok.name != "raw" {
				w.leafOf[i], w.unescaped[i] = leaf, html.UnescapeString(tok.raw)
			}
		case tok.close:
			stack = popTo(stack, tok.name)
			if tok.name == "math" && mathDepth > 0 {
				mathDepth--
			}
		case tok.void:
		default:
			stack = append(stack, element{name: tok.name, id: idOf(tok)})
			inBody = inBody || tok.name == "body"
			if tok.name == "math" {
				mathDepth++
				w.formulas[i] = idOf(tok)
			}
		}
	}
	return w
}

func (w *walk) nearestLeaf(stack []element) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].id != "" {
			if _, ok := w.data.Leaves[stack[i].id]; ok {
				return stack[i].id
			}
		}
	}
	return ""
}

func popTo(stack []element, name string) []element {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].name == name {
			return stack[:i]
		}
	}
	return stack
}

// words are the prose words of every text token, grouped by node in reading order.
func (w *walk) words() (map[string][]htmlWord, []htmlWord) {
	byLeaf := map[string][]htmlWord{}
	var all []htmlWord
	indexes := make([]int, 0, len(w.unescaped))
	for i := range w.unescaped {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		text := w.unescaped[i]
		for _, span := range wordPiece.FindAllStringIndex(text, -1) {
			word := htmlWord{token: i, start: span[0], end: span[1], norm: Normalize(text[span[0]:span[1]])}
			if word.norm == "" {
				continue
			}
			all = append(all, word)
			byLeaf[w.leafOf[i]] = append(byLeaf[w.leafOf[i]], word)
		}
	}
	return byLeaf, all
}

// Annotate returns the export's HTML with each word that maps to the source wrapped in a span
// (class w, data-o the source ranges, focusable) and each mapped formula marked likewise (class sx).
func Annotate(page string, data *Data) (string, Stats) {
	tokens := tokenize(page)
	w := scan(tokens, data)
	byLeaf, all := w.words()
	stats := Stats{Words: len(all)}
	marks := map[int][]mark{}
	for leaf, words := range byLeaf {
		if leaf == "" {
			continue
		}
		stats.Owned += len(words)
		w.align(leaf, words, marks, &stats)
	}
	for _, word := range all {
		if !marked(marks[word.token], word) && len(stats.Missed) < 40 {
			stats.Missed = append(stats.Missed, w.unescaped[word.token][word.start:word.end])
		}
	}
	w.formulaMarks(&tokens, &stats)
	return render(tokens, w.unescaped, marks), stats
}

// mark is one wrapped word: its byte range in the unescaped text and its source ranges.
type mark struct {
	start, end int
	origins    string
}

func marked(list []mark, word htmlWord) bool {
	for _, m := range list {
		if m.start == word.start {
			return true
		}
	}
	return false
}

// align pairs a node's HTML words with the PDF's words of the node and records the marks.
func (w *walk) align(leaf string, words []htmlWord, marks map[int][]mark, stats *Stats) {
	var pdf []string
	var pdfWords []Word
	for _, word := range w.data.Leaves[leaf] {
		if word.Norm != "" {
			pdf = append(pdf, word.Norm)
			pdfWords = append(pdfWords, word)
		}
	}
	norms := make([]string, len(words))
	for i, word := range words {
		norms[i] = word.norm
	}
	for i, j := range align(norms, pdf) {
		if j < 0 || len(pdfWords[j].Origins) == 0 {
			continue
		}
		origins := pdfWords[j].Origins
		marks[words[i].token] = append(marks[words[i].token], mark{words[i].start, words[i].end, originList(origins)})
		stats.Matched++
		if origins[0].Start >= 0 {
			stats.Exact++
		} else {
			stats.LineOnly++
		}
	}
}

// originList is the data-o attribute of a set of origins: FILE:LINE:START:END, space separated,
// FILE:LINE alone when the map knows the line only.
func originList(origins []Origin) string {
	seen := map[Origin]bool{}
	var parts []string
	for _, origin := range origins {
		if seen[origin] {
			continue
		}
		seen[origin] = true
		parts = append(parts, formatOrigin(origin))
	}
	return strings.Join(parts, " ")
}

func formatOrigin(origin Origin) string {
	if origin.Start < 0 {
		return fmt.Sprintf("%d:%d", origin.File, origin.Line)
	}
	return fmt.Sprintf("%d:%d:%d:%d", origin.File, origin.Line, origin.Start, origin.End)
}

// formulaMarks marks each formula whose node owns glyphs with the source lines and columns they span.
func (w *walk) formulaMarks(tokens *[]token, stats *Stats) {
	indexes := make([]int, 0, len(w.formulas))
	for i := range w.formulas {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		stats.Formulas++
		words := w.data.Leaves[w.formulas[i]]
		if len(words) == 0 {
			continue
		}
		var origins []Origin
		for _, word := range words {
			origins = append(origins, word.Origins...)
		}
		ranges := w.data.formulaRanges(origins)
		if len(ranges) == 0 {
			continue
		}
		stats.FormulasMapped++
		(*tokens)[i].raw = withAttributes((*tokens)[i], `class="sx" tabindex="0" data-o="`+originList(ranges)+`"`)
	}
}

// render writes the tokens, each marked text token with its words wrapped.
func render(tokens []token, unescaped map[int]string, marks map[int][]mark) string {
	var out strings.Builder
	for i, tok := range tokens {
		list := marks[i]
		if len(list) == 0 {
			out.WriteString(tok.raw)
			continue
		}
		sort.Slice(list, func(a, b int) bool { return list[a].start < list[b].start })
		text, previous := unescaped[i], 0
		for _, m := range list {
			out.WriteString(escapeText.Replace(text[previous:m.start]))
			out.WriteString(`<span class="w" tabindex="0" data-o="` + m.origins + `">`)
			out.WriteString(escapeText.Replace(text[m.start:m.end]))
			out.WriteString("</span>")
			previous = m.end
		}
		out.WriteString(escapeText.Replace(text[previous:]))
	}
	return out.String()
}
