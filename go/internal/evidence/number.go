// Package evidence is the Go port of the Ruby evidence classes (Relatex::Evidence,
// MathEvidence, ReadingEvidence and their helpers): what the collector's record stream
// proves about the paper, before the paper document is built from it.
package evidence

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// literal: letters, digits and the punctuation of enumerate labels ((A1), [i], 1a)).
	literal = regexp.MustCompile(`\A[0-9A-Za-z.()\[\]/:*'+ -]+\z`)
	// footnoteSymbol: \fnsymbol's marks, one or two \TextOrMath groups whose text branch is a
	// character code or a named text symbol (\TU\textdagger).
	footnoteSymbol = regexp.MustCompile(`\A(?:\\TextOrMath\s*\{(?:\\char\s*"[0-9A-F]{4,5}|(?:\\[A-Z0-9]+)?\\text[a-z]+)\s*\}\{[^{}]*\}\s*)+\z`)
	symbolGroup    = regexp.MustCompile(`\\TextOrMath\s*\{(?:\\char\s*"([0-9A-F]{4,5})|(?:\\[A-Z0-9]+)?\\(text[a-z]+))\s*\}\{[^{}]*\}`)
	textSymbols    = map[string]string{"textdagger": "†", "textdaggerdbl": "‡", "textsection": "§", "textparagraph": "¶",
		"textbardbl": "‖", "textasteriskcentered": "∗", "textperiodcentered": "·"}
	// mathSymbols: a mark a class sets as one math symbol (PoS's \note[$\dagger$]).
	mathSymbols = map[string]string{"dagger": "†", "ddagger": "‡", "ast": "∗", "star": "⋆", "S": "§", "P": "¶",
		"sharp": "♯", "natural": "♮", "flat": "♭", "bullet": "•"}
	mathSymbol = regexp.MustCompile(`\A\$\s*\\(dagger|ddagger|ast|star|S|P|sharp|natural|flat|bullet)\s*\$\z`)
	// boxedNumber: \mbox{III-A} as written by IEEEtran's \thesubsection.
	boxedNumber = regexp.MustCompile(`\A(?:\\protect\s*)?(?:\\unhbox\s*\\voidb@x\s*\\hbox|\\mbox)\s*\{([0-9A-Za-z.()\[\]-]+)\}([0-9A-Za-z.()-]*)\z`)
	// innerGroups: a number whose later parts sit in plain groups (a theorem counter written
	// 44.1.{5}): the groups print nothing.
	innerGroups = regexp.MustCompile(`\A[0-9A-Za-z.()-]+(?:\{[0-9A-Za-z.()-]+\}[0-9A-Za-z.()-]*)+\z`)
	groupNumber = regexp.MustCompile(`\A\{([0-9A-Za-z.-]+)\}\z`)
	// fontSwitched: old font switches around a number print nothing (jaa's {\rm {5.}1} is 5.1).
	fontSwitched  = regexp.MustCompile(`\A(?:[{}]|\\(?:rm|bf|it|sf|tt|sc|sl|em|normalfont|upshape|itshape|bfseries|mdseries)\b\s*|[0-9A-Za-z.()\[\]/:*'+ -])+\z`)
	switchOrBrace = regexp.MustCompile(`\\[a-z]+\s*|[{}]`)
	// silent: markup that prints no text, a thin space (III\,A), a colour (\color{blue}8; the argument
	// of \textcolor{blue}{8} stays) and an appendix's counter reset.
	silent = regexp.MustCompile(`\\,|\\(?:text)?color\s*\{[A-Za-z0-9!.]+\}\s*|\\global\s*\\c@[A-Za-z]+\s*\d+\s*\\relax\s*`)
)

// SameMark is Evidence.same_mark?: a printed mark against the number recorded for it;
// \textasteriskcentered (U+2217) prints as a plain asterisk in fonts without it.
func SameMark(printed string, expected *string) bool {
	return expected != nil && strings.ReplaceAll(printed, "∗", "*") == strings.ReplaceAll(*expected, "∗", "*")
}

// LiteralNumber is Evidence.literal_number: the text a recorded counter number prints,
// when it is a literal (with at most one TeX group around it) or a kernel footnote symbol;
// nil otherwise.
func LiteralNumber(number string) *string {
	if silent.MatchString(number) {
		number = strings.TrimSpace(silent.ReplaceAllString(number, ""))
	}
	if strings.Contains(number, "~") && !strings.Contains(number, `\`) {
		number = strings.ReplaceAll(number, "~", " ") // A no-break space (Appendix~A.2) prints as a space.
	}
	if literal.MatchString(number) {
		return &number
	}
	if inner, wrapped := groupedLiteral(number); wrapped {
		return &inner
	}
	if footnoteSymbol.MatchString(number) {
		return footnoteText(number)
	}
	if match := mathSymbol.FindStringSubmatch(number); match != nil {
		symbol := mathSymbols[match[1]]
		return &symbol
	}
	if match := groupNumber.FindStringSubmatch(number); match != nil {
		return &match[1]
	}
	if match := boxedNumber.FindStringSubmatch(number); match != nil {
		text := match[1] + match[2]
		return &text
	}
	if innerGroups.MatchString(number) {
		text := strings.NewReplacer("{", "", "}", "").Replace(number)
		return &text
	}
	return fontSwitchedNumber(number)
}

func footnoteText(number string) *string {
	var text strings.Builder
	for _, group := range symbolGroup.FindAllStringSubmatch(number, -1) {
		if group[1] != "" {
			code, _ := strconv.ParseInt(group[1], 16, 32)
			text.WriteRune(rune(code))
			continue
		}
		symbol, ok := textSymbols[group[2]]
		if !ok {
			return nil
		}
		text.WriteString(symbol)
	}
	result := text.String()
	return &result
}

// groupedLiteral: a literal inside nested plain groups, which print nothing. enumitem records an
// item's custom label so ({{(i)}}, {{1:}} from label=(\roman*), label=\arabic*:).
func groupedLiteral(number string) (string, bool) {
	opened := strings.TrimLeft(number, "{")
	inner := strings.TrimRight(opened, "}")
	depth := len(number) - len(opened)
	return inner, depth > 0 && depth == len(opened)-len(inner) && literal.MatchString(inner)
}

func fontSwitchedNumber(number string) *string {
	if !strings.Contains(number, `\`) || !fontSwitched.MatchString(number) ||
		strings.Count(number, "{") != strings.Count(number, "}") {
		return nil
	}
	text := switchOrBrace.ReplaceAllString(number, "")
	if text == "" {
		return nil
	}
	return &text
}

// mathNumberGroup: a math-mode run of a number ($A$, $_a$, $R_1$) whose body is letters, digits,
// scripts and their braces.
var mathNumberGroup = regexp.MustCompile(`\$([0-9A-Za-z.()\[\]/:*'+ ^_{}-]*)\$`)

var (
	// mathAlphabet: a font or weight command that sets its argument's glyphs in another alphabet
	// ($\bm{\mathcal{Q}}_1$ prints a bold script Q with a subscript 1); it adds no glyph.
	mathAlphabet = regexp.MustCompile(`\\(?:bm|boldsymbol|mathcal|mathbf|mathit|mathrm|mathsf|mathtt|mathbb|mathscr|mathfrak)\b\s*`)
	// mathSymbolMacro: a symbol of mathSymbols set in a math run ({$\ddagger \ddagger $}).
	mathSymbolMacro = regexp.MustCompile(`\\(dagger|ddagger|ast|star|S|P|sharp|natural|flat|bullet)\b\s*`)
	symbolNumber    = regexp.MustCompile(`\A\{*([†‡∗⋆§¶♯♮♭•]+)\}*\z`)
)

// MathNumber is the text a number typeset partly or wholly as math ({$A$}, {NLS$_a$}, {($R_1$)},
// $\bm{\mathcal{Q}}_1$, {$\ddagger $}) prints: its glyphs in reading order, scripts flattened,
// groups and math alphabets dropped, symbols as their characters; nil when the number has no
// math run or holds any other macro.
func MathNumber(number string) *string {
	if !strings.Contains(number, "$") {
		return nil
	}
	read := mathAlphabet.ReplaceAllString(number, "")
	read = mathSymbolMacro.ReplaceAllStringFunc(read, func(macro string) string {
		return mathSymbols[mathSymbolMacro.FindStringSubmatch(macro)[1]]
	})
	if match := symbolNumber.FindStringSubmatch(strings.ReplaceAll(read, "$", "")); match != nil &&
		strings.Count(read, "$") == 2 {
		return &match[1]
	}
	flat := mathNumberGroup.ReplaceAllStringFunc(read, func(group string) string {
		body := group[1 : len(group)-1]
		return strings.NewReplacer("_", "", "^", "", "{", "", "}", "").Replace(body)
	})
	if strings.ContainsAny(flat, "$\\") {
		return nil
	}
	return LiteralNumber(flat)
}
