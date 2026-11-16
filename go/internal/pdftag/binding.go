package pdftag

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// The binding of the model's blocks to the glyphs of the ordinary PDF (decision 105 item 2): the
// blocks' text, in reading order, aligned with the glyphs' text in paint order.

// glyphRef is a painted glyph: its page and paint index.
type glyphRef struct{ page, index int }

// pieceKind is what a piece of a block binds.
type pieceKind int

const (
	pieceText pieceKind = iota
	pieceMath
	pieceImage
)

// piece is a part of a block bound to glyphs (or, for an image, to a placement).
type piece struct {
	leaf  string // the structure element it belongs to
	kind  pieceKind
	text  string
	chars []rune     // a formula's characters as its MathML spells them
	first bool       // the first text of its block
	caps  bool       // text set in small capitals, whose glyphs may name the other case
	size  [2]float64 // an image's width and height in bp
}

// glyphStream is the document's painted glyphs as one character sequence.
type glyphStream struct {
	chars []rune
	at    []glyphRef
}

// binding is the result: which structure leaf each glyph belongs to.
type binding struct {
	leafOf     map[glyphRef]string
	images     []imagePiece
	words      int
	wordsBound int
	formulas   int
	bound      int
	misses     []string
}

// imagePiece is an image of the model with the page its block was bound on.
type imagePiece struct {
	leaf string
	page int
	size [2]float64
}

// Search windows, in characters of the glyph stream.
const (
	blockWindow   = 4000
	wordWindow    = 60
	formulaWindow = 400
	formulaStep   = 12
	keyLength     = 16
)

// enclosingCircle is U+20DD. cmsy10's \bigcirc (the large circle the document spells U+25EF) reaches the
// PDF text layer under that code, after the digit it encircles or alone; a combining mark with the
// large circle's glyph is read as the large circle on both sides.
const enclosingCircle, largeCircle = 0x20DD, 0x25EF

// normalized is text as the alignment compares it: NFKC, without white space and invisible
// operators, the enclosing circle read as the large circle.
func normalized(text string) []rune {
	var out []rune
	for _, r := range norm.NFKC.String(text) {
		if unicode.IsSpace(r) || (r >= 0x2061 && r <= 0x2064) || r == 0x200b {
			continue
		}
		if r == enclosingCircle {
			r = largeCircle
		}
		out = append(out, r)
	}
	return out
}

// newGlyphStream joins the pages' painted glyphs.
func newGlyphStream(painted map[int][]paintedGlyph, pageCount int) *glyphStream {
	stream := &glyphStream{}
	for number := 1; number <= pageCount; number++ {
		for index, glyph := range painted[number] {
			for _, r := range normalized(glyph.text) {
				stream.chars = append(stream.chars, r)
				stream.at = append(stream.at, glyphRef{number, index})
			}
		}
	}
	return stream
}

var mathLeaf = regexp.MustCompile(`<m(?:i|n|o|text|s)\b[^>]*>([^<]*)</m(?:i|n|o|text|s)>`)

// mathChars are a formula's characters, from its MathML's token elements.
func mathChars(mathml string) []rune {
	var chars []rune
	for _, match := range mathLeaf.FindAllStringSubmatch(mathml, -1) {
		chars = append(chars, normalized(html.UnescapeString(match[1]))...)
	}
	return chars
}

// isHyphen is a glyph the reading drops at a line break.
func isHyphen(r rune) bool { return r == '-' || r == '‐' || r == '­' }

// matchAt matches word at pos, skipping hyphens inside it; it returns the end.
func (s *glyphStream) matchAt(word []rune, pos int) (int, bool) {
	i, j := 0, pos
	for i < len(word) && j < len(s.chars) {
		switch {
		case s.chars[j] == word[i]:
			i, j = i+1, j+1
		case i > 0 && isHyphen(s.chars[j]) && !isHyphen(word[i]):
			j++
		default:
			return 0, false
		}
	}
	return j, i == len(word)
}

// free reports whether no glyph of the characters from start to end is bound yet.
func (a *aligner) free(start, end int) bool {
	for pos := start; pos < end; pos++ {
		if a.taken[pos] {
			return false
		}
	}
	return true
}

// matchFree matches word at start on glyphs not bound yet.
func (a *aligner) matchFree(word []rune, start int) (int, bool) {
	if a.taken[start] || a.stream.chars[start] != word[0] {
		return 0, false
	}
	end, ok := a.stream.matchAt(word, start)
	return end, ok && a.free(start, end)
}

// find searches word forward from pos within window characters.
func (a *aligner) find(word []rune, pos, window int) (int, int, bool) {
	limit := min(len(a.stream.chars), pos+window)
	for start := max(pos, 0); start < limit; start++ {
		if end, ok := a.matchFree(word, start); ok {
			return start, end, true
		}
	}
	return 0, 0, false
}

// findBack searches word backward from pos within window characters, nearest first: a float
// is painted where its page begins, before the text it follows in reading order.
func (a *aligner) findBack(word []rune, pos, window int) (int, bool) {
	for start := min(pos, len(a.stream.chars)) - 1; start >= max(0, pos-window); start-- {
		if _, ok := a.matchFree(word, start); ok {
			return start, true
		}
	}
	return 0, false
}

// aligner walks the pieces along the glyph stream.
type aligner struct {
	stream *glyphStream
	cursor int    // where the next word is searched
	high   int    // the furthest the reading has bound: a new block is searched from here
	taken  []bool // characters already bound
	result *binding
}

func (a *aligner) assign(start, end int, leaf string) {
	for pos := start; pos < end; pos++ {
		a.result.leafOf[a.stream.at[pos]] = leaf
		a.taken[pos] = true
	}
	a.cursor, a.high = end, max(a.high, end)
}

// text binds a text piece word by word, finding its place again by a longer key when a word is
// not where the previous one ended.
func (a *aligner) text(p piece) {
	words := strings.Fields(p.text)
	if p.first {
		a.cursor = a.high
		a.locate(words, 0)
	}
	for index := 0; index < len(words); index++ {
		runes := normalized(words[index])
		if len(runes) == 0 {
			continue
		}
		a.result.words++
		start, end, ok := a.find(runes, a.cursor, wordWindow)
		if !ok && a.locate(words, index) {
			start, end, ok = a.find(runes, a.cursor, wordWindow)
		}
		if !ok {
			a.result.misses = append(a.result.misses, fmt.Sprintf("%s: %q", p.leaf, words[index]))
			continue
		}
		a.result.wordsBound++
		a.assign(start, end, p.leaf)
	}
}

// locate moves the cursor to the words from index on, searched as one key of at least
// keyLength characters: forward first, then backward.
func (a *aligner) locate(words []string, index int) bool {
	var key []rune
	for _, word := range words[index:] {
		key = append(key, normalized(word)...)
		if len(key) >= keyLength {
			break
		}
	}
	if len(key) == 0 {
		return false
	}
	if start, _, ok := a.find(key, a.cursor, blockWindow); ok {
		a.cursor = start
		return true
	}
	if start, ok := a.findBack(key, a.cursor, blockWindow); ok {
		a.cursor = start
		return true
	}
	return false
}

// formula binds a formula to the glyphs its characters spell, first to last, on one page.
func (a *aligner) formula(p piece) {
	a.result.formulas++
	if p.first {
		a.cursor = a.high
	}
	first, last, pos := -1, -1, a.cursor
	for _, r := range p.chars {
		window := formulaStep
		if first < 0 {
			window = formulaWindow
		}
		found := a.next(r, pos, window, first)
		if found < 0 {
			continue
		}
		if first < 0 {
			first = found
		}
		last, pos = found, found+1
	}
	if first < 0 {
		a.result.misses = append(a.result.misses, p.leaf+": formula")
		return
	}
	a.result.bound++
	a.assign(first, last+1, p.leaf)
}

// next finds r from pos within window, on the page of first when one is set.
func (a *aligner) next(r rune, pos, window, first int) int {
	limit := min(len(a.stream.chars), pos+window)
	for at := pos; at < limit; at++ {
		if first >= 0 && a.stream.at[at].page != a.stream.at[first].page {
			return -1
		}
		if a.stream.chars[at] == r && !a.taken[at] {
			return at
		}
	}
	return -1
}

// image records an image with the page the cursor is on.
func (a *aligner) image(p piece) {
	number := 0
	if len(a.stream.at) > 0 {
		number = a.stream.at[min(a.cursor, len(a.stream.at)-1)].page
	}
	a.result.images = append(a.result.images, imagePiece{leaf: p.leaf, page: number, size: p.size})
}

// bind aligns the pieces with the glyph stream.
func bind(pieces []piece, stream *glyphStream) *binding {
	a := &aligner{stream: stream, taken: make([]bool, len(stream.chars)), result: &binding{leafOf: map[glyphRef]string{}}}
	for _, p := range pieces {
		switch p.kind {
		case pieceText:
			a.text(p)
		case pieceMath:
			a.formula(p)
		case pieceImage:
			a.image(p)
		}
	}
	return a.result
}
