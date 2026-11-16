// Package mathtokens reads TeX math source into the glyph strings its tokens typeset and
// aligns the glyph clusters of a typeset formula, in reading order, with those tokens. It is
// the letter-level half of source synchronization inside math (decision 066): each cluster
// the shipout shows gets the source range of the tokens that produced it, or none.
package mathtokens

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Token is one visible token of a formula: the glyph string it typesets (folded) and its
// scalar offsets in the source text, Stop exclusive.
type Token struct {
	Glyph       string
	Start, Stop int
}

// Match is the source range of one cluster; OK is false when the cluster was not found.
type Match struct {
	Start, Stop int
	OK          bool
}

var folded = strings.NewReplacer("−", "-", "–", "-", "—", "-", "∗", "*", "⋅", "·", "∙", "·", "′", "'", "’", "'",
	"∥", "‖", "ϵ", "ε", "ϕ", "φ", "ϑ", "θ", "ϖ", "π", "ϱ", "ρ", "ς", "σ", "∼", "~", "⟨", "<", "⟩", ">", "〈", "<", "〉", ">",
	"⌊", "⌊", "∕", "/", "⁄", "/")

// Fold is the form in which a typeset cluster and a source token compare: NFKC (math
// alphanumerics become plain letters), no white space, no combining or modifier accents, and
// the minus, asterisk, dot, prime and variant Greek forms of the math fonts made one.
func Fold(text string) string {
	var builder strings.Builder
	for _, character := range norm.NFKC.String(text) {
		if unicode.IsSpace(character) || unicode.Is(unicode.Mn, character) || strings.ContainsRune("ˆ˜¯˙¨ˇ˘´`", character) {
			continue
		}
		builder.WriteRune(character)
	}
	return folded.Replace(builder.String())
}

// Tokenize reads text[start:stop] (scalar offsets into text) as TeX math. Characters that
// typeset a glyph become tokens; markup (braces, ^, _, &, \\, spacing and font commands) does
// not, and an unknown command typesets nothing the aligner can name, so it is skipped too.
func Tokenize(text []rune, start, stop int) []Token {
	reader := &reader{text: text, stop: stop}
	reader.at = start
	reader.run()
	return reader.tokens
}

type reader struct {
	text   []rune
	at     int
	stop   int
	tokens []Token
}

func (r *reader) run() {
	for r.at < r.stop {
		character := r.text[r.at]
		switch {
		case character == '%':
			for r.at < r.stop && r.text[r.at] != '\n' {
				r.at++
			}
		case character == '\\':
			r.command()
		case character == '{' || character == '}' || character == '^' || character == '_' || character == '&' ||
			character == '~' || unicode.IsSpace(character):
			r.at++
		default:
			r.emit(string(character), r.at, r.at+1)
			r.at++
		}
	}
}

func (r *reader) emit(glyph string, start, stop int) {
	if glyph = Fold(glyph); glyph != "" {
		r.tokens = append(r.tokens, Token{Glyph: glyph, Start: start, Stop: stop})
	}
}

func (r *reader) command() {
	start := r.at
	r.at++
	if r.at >= r.stop {
		return
	}
	first := r.text[r.at]
	if !unicode.IsLetter(first) {
		r.at++
		r.symbol(string(first), start)
		return
	}
	for r.at < r.stop && unicode.IsLetter(r.text[r.at]) {
		r.at++
	}
	name := string(r.text[start+1 : r.at])
	r.symbol(name, start)
}

// symbol handles a control word or control symbol: a glyph, markup, or an unknown command.
func (r *reader) symbol(name string, start int) {
	switch {
	case name == "\\" || name == "," || name == ";" || name == ":" || name == "!" || name == " " || name == "/" || name == "-":
		return
	case name == "left" || name == "right" || name == "middle":
		r.delimiter()
	case name == "sqrt":
		r.emit("√", start, r.at)
		r.optionalArgument()
	case invisible[name]:
		r.skipArgumentName(name)
	default:
		if glyph, ok := symbols[name]; ok {
			r.emit(glyph, start, r.at)
		}
	}
}

// delimiter reads the delimiter after \left, \right or \middle: it typesets unless it is a dot.
func (r *reader) delimiter() {
	if r.at >= r.stop {
		return
	}
	if r.text[r.at] == '.' {
		r.at++
	}
}

// optionalArgument skips the brackets of \sqrt[n]{x} while reading n as math.
func (r *reader) optionalArgument() {
	if r.at < r.stop && r.text[r.at] == '[' {
		r.at++
		for r.at < r.stop && r.text[r.at] != ']' {
			if r.text[r.at] == '\\' {
				r.command()
				continue
			}
			if !unicode.IsSpace(r.text[r.at]) && r.text[r.at] != '{' && r.text[r.at] != '}' {
				r.emit(string(r.text[r.at]), r.at, r.at+1)
			}
			r.at++
		}
		if r.at < r.stop {
			r.at++
		}
	}
}

// skipArgumentName drops the {name} of \begin{...} and \end{...}.
func (r *reader) skipArgumentName(name string) {
	if name != "begin" && name != "end" {
		return
	}
	for r.at < r.stop && unicode.IsSpace(r.text[r.at]) {
		r.at++
	}
	if r.at < r.stop && r.text[r.at] == '{' {
		for r.at < r.stop && r.text[r.at] != '}' {
			r.at++
		}
		if r.at < r.stop {
			r.at++
		}
	}
	// \begin{array}{cc}: a column specification is markup, not glyphs.
	if name == "begin" && r.at < r.stop && r.text[r.at] == '{' {
		for r.at < r.stop && r.text[r.at] != '}' {
			r.at++
		}
		if r.at < r.stop {
			r.at++
		}
	}
}

// Align finds, for each cluster in reading order, the earliest unused window of consecutive
// unused tokens whose glyphs spell the cluster. A cluster that no window spells has no match.
func Align(clusters []string, tokens []Token) []Match {
	matches := make([]Match, len(clusters))
	used := make([]bool, len(tokens))
	for index, cluster := range clusters {
		want := Fold(cluster)
		if want == "" {
			continue
		}
		if first, last, ok := find(want, tokens, used); ok {
			for at := first; at <= last; at++ {
				used[at] = true
			}
			matches[index] = Match{Start: tokens[first].Start, Stop: tokens[last].Stop, OK: true}
		}
	}
	return matches
}

func find(want string, tokens []Token, used []bool) (int, int, bool) {
	for first := range tokens {
		if used[first] || !strings.HasPrefix(want, tokens[first].Glyph) {
			continue
		}
		spelled, last := "", first
		for at := first; at < len(tokens) && !used[at]; at++ {
			spelled += tokens[at].Glyph
			last = at
			if spelled == want {
				return first, last, true
			}
			if !strings.HasPrefix(want, spelled) {
				break
			}
		}
	}
	return 0, 0, false
}

// Spells reports whether the tokens that lie exactly within [start, stop) spell the cluster:
// the check the validator makes of a claimed match.
func Spells(tokens []Token, start, stop int, cluster string) bool {
	spelled, first, last := "", -1, -1
	for index, token := range tokens {
		if token.Start >= start && token.Stop <= stop {
			if first < 0 {
				first = index
			}
			last = index
			spelled += token.Glyph
		}
	}
	return first >= 0 && tokens[first].Start == start && tokens[last].Stop == stop && spelled == Fold(cluster) && spelled != ""
}

// Split divides the tokens that lie exactly within [start, stop) among the glyphs of one cluster,
// in order: each glyph takes the consecutive tokens that spell it. It reports false, with no
// matches, when the glyphs do not use up exactly those tokens.
func Split(glyphs []string, tokens []Token, start, stop int) ([]Match, bool) {
	inside := []Token{}
	for _, token := range tokens {
		if token.Start >= start && token.Stop <= stop {
			inside = append(inside, token)
		}
	}
	matches := make([]Match, len(glyphs))
	at := 0
	for index, glyph := range glyphs {
		want := Fold(glyph)
		first, spelled := at, ""
		for at < len(inside) && spelled != want && strings.HasPrefix(want, spelled+inside[at].Glyph) {
			spelled += inside[at].Glyph
			at++
		}
		if want == "" || spelled != want {
			return nil, false
		}
		matches[index] = Match{Start: inside[first].Start, Stop: inside[at-1].Stop, OK: true}
	}
	return matches, at == len(inside)
}
