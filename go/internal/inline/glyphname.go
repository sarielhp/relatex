package inline

import (
	"strings"
	"unicode"
)

// GlyphLabel is what a reader hears for a glyph that has no Unicode character but a name in its
// symbol font (a text span's glyph_name): the name's words, split where the case or a digit run
// changes and the separators dropped, the first capitalized and the rest lower case unless they
// are all capitals (marvosym's WashCotton is "Wash cotton", EUR stays "EUR").
func GlyphLabel(name string) string {
	if spoken, ok := spokenNames[name]; ok {
		return spoken
	}
	words := []string{}
	var word []rune
	runes := []rune(name)
	flush := func() {
		if len(word) > 0 {
			words = append(words, string(word))
			word = nil
		}
	}
	for index, character := range runes {
		if character == '.' || character == '_' || character == '-' {
			flush()
			continue
		}
		if index > 0 && len(word) > 0 && wordBreak(runes, index) {
			flush()
		}
		word = append(word, character)
	}
	flush()
	for index, part := range words {
		if index > 0 && strings.ToUpper(part) != part {
			words[index] = strings.ToLower(part)
		}
	}
	return strings.Join(words, " ")
}

// wordBreak: a word starts at a capital after a lower-case letter or a digit, at the last capital
// of a capitals run that a lower-case letter follows (MVDivision: MV, Division), and where digits
// begin or end.
func wordBreak(runes []rune, index int) bool {
	previous, current := runes[index-1], runes[index]
	if unicode.IsDigit(previous) != unicode.IsDigit(current) {
		return true
	}
	if !unicode.IsUpper(current) {
		return false
	}
	if unicode.IsLower(previous) {
		return true
	}
	return unicode.IsUpper(previous) && index+1 < len(runes) && unicode.IsLower(runes[index+1])
}
