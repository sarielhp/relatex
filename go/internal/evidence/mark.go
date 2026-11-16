package evidence

import (
	"regexp"
	"strings"
)

var (
	// markWrapper: markup around a footnote mark that prints only what it wraps or nothing:
	// \protect, \relax, the boxes and superscript commands a class sets the mark in, math
	// shifts and superscripts (MNRAS records its \thanks star as
	// \protect \mbox {{$\protect \mbox {${^{\star }}$}\relax $}}).
	markWrapper = regexp.MustCompile(`\\(?:protect|relax|mbox|hbox|ensuremath|textsuperscript)(?:\s+|\b)|[$^]`)
	// markSymbolRun: a mark that is one or more math symbols (\star \star) once wrappers,
	// groups and spaces are gone.
	markSymbolRun  = regexp.MustCompile(`\A(?:\\(?:dagger|ddagger|ast|star|S|P|sharp|natural|flat|bullet)|\*)+\z`)
	markSymbolName = regexp.MustCompile(`\\[A-Za-z]+|\*`)
	// texSpecial: a character that makes a recorded number TeX source rather than text.
	texSpecial = `\${}^`
)

// PrintedMark is the text a recorded footnote number prints, never TeX source: a plain number
// is kept as recorded; a literal or kernel symbol mark reads as LiteralNumber does; a mark set
// in boxes, math or superscripts reads as the symbols it wraps (⋆, †, ∗). A mark left as code
// the PDF does not show as text (\@thefnmark, \thefootnote, \hskip) prints nothing: "".
func PrintedMark(number string) string {
	if !strings.ContainsAny(number, texSpecial) {
		return number
	}
	trimmed := strings.TrimSpace(number)
	if text := LiteralNumber(trimmed); text != nil {
		return *text
	}
	peeled := strings.TrimSpace(markWrapper.ReplaceAllString(trimmed, ""))
	for strings.HasPrefix(peeled, "{") && strings.HasSuffix(peeled, "}") {
		peeled = strings.TrimSpace(peeled[1 : len(peeled)-1])
	}
	if text := LiteralNumber(peeled); text != nil {
		return *text
	}
	bare := strings.Join(strings.FieldsFunc(peeled, func(character rune) bool {
		return character == '{' || character == '}' || character == ' ' || character == '\t' || character == '\n'
	}), "")
	if !markSymbolRun.MatchString(bare) {
		return ""
	}
	return markSymbolName.ReplaceAllStringFunc(bare, func(name string) string {
		if name == "*" {
			return name
		}
		return mathSymbols[name[1:]]
	})
}
