package frontmatter

import (
	"regexp"
	"strings"
)

var (
	starMark          = rx(`\A\s*\*+\s*\z`)
	environmentHead   = rx(`\\(?:re)?newenvironment\*?\s*\{([A-Za-z@*]+)\}\s*\{`)
	environmentMarker = regexp.MustCompile(`\\(begin|end)\{([A-Za-z@*]+)\}`)
)

// textEnvironments: the document's own environments without arguments
// (\newenvironment{IEEEkeywords}{\textbf{Keywords:}}{}); a later definition wins.
func (t *TexMetadata) textEnvironments() map[string][2]string {
	found := map[string][2]string{}
	for _, match := range environmentHead.FindAllStringSubmatchIndex(t.definitions, -1) {
		if codes, ok := t.environmentCodes(match[1] - 1); ok {
			found[t.definitions[match[2]:match[3]]] = codes
		}
	}
	return found
}

// environmentCodes: the begin and end code arguments at position, if both read.
func (t *TexMetadata) environmentCodes(position int) (codes [2]string, ok bool) {
	defer recoverFlag(&ok)
	opening, ending := t.argument(t.definitions, position)
	closing, _ := t.argument(t.definitions, ending)
	return [2]string{opening, closing}, true
}

// expandEnvironments: \begin and \end of the paper's own environments print their code.
func (t *TexMetadata) expandEnvironments(value string) string {
	if t.environments == nil {
		t.environments = t.textEnvironments()
	}
	return environmentMarker.ReplaceAllStringFunc(value, func(match string) string {
		groups := environmentMarker.FindStringSubmatch(match)
		codes, ok := t.environments[groups[2]]
		if !ok {
			return match
		}
		if groups[1] == "begin" {
			return codes[0]
		}
		return codes[1]
	})
}

// displayDelimiters: displayed and parenthesized math (\[...\], $$...$$, \(...\)), each read as
// $...$.
var displayDelimiters = [][2]string{{`\[`, `\]`}, {"$$", "$$"}, {`\(`, `\)`}}

// inlineDisplays is DISPLAY_MATH's gsub: each delimiter pair not preceded by a backslash, its
// body the shortest that reaches a closing one.
func inlineDisplays(value string) string {
	var builder strings.Builder
	position := 0
	for start := 0; start < len(value); start++ {
		if start > 0 && value[start-1] == '\\' {
			continue
		}
		for _, pair := range displayDelimiters {
			if !strings.HasPrefix(value[start:], pair[0]) {
				continue
			}
			if end, ok := closingDelimiter(value, start+2, pair[1]); ok {
				builder.WriteString(value[position:start])
				builder.WriteString("$" + value[start+2:end] + "$")
				position = end + 2
				start = position - 1
				break
			}
		}
	}
	builder.WriteString(value[position:])
	return builder.String()
}

// closingDelimiter is the first closer at or after from that no backslash precedes.
func closingDelimiter(value string, from int, closer string) (int, bool) {
	for index := from; index+len(closer) <= len(value); index++ {
		if strings.HasPrefix(value[index:], closer) && value[index-1] != '\\' {
			return index, true
		}
	}
	return 0, false
}

// plainAbstracts is scan(/\\abstract(?![A-Za-z])(.*?)\\endabstract(?![A-Za-z])/m): the abstract
// environment's plain TeX spelling.
func plainAbstracts(text string) []string {
	result := []string{}
	position := 0
	for {
		start, ok := controlWordAt(text, position, `\abstract`)
		if !ok {
			return result
		}
		end, ok := controlWordAt(text, start+len(`\abstract`), `\endabstract`)
		if !ok {
			return result
		}
		result = append(result, text[start+len(`\abstract`):end])
		position = end + len(`\endabstract`)
	}
}

// controlWordAt is the first occurrence of word at or after from that no letter follows.
func controlWordAt(text string, from int, word string) (int, bool) {
	for from <= len(text) {
		index := strings.Index(text[from:], word)
		if index < 0 {
			return 0, false
		}
		if notLetter(text, from+index+len(word)) {
			return from + index, true
		}
		from += index + 1
	}
	return 0, false
}

// spacingEnd is \b(?:\[[0-4]\])?\s*: a spacing command, with \linebreak's optional priority.
func spacingEnd(text string, end int) (int, bool) {
	if !boundary(text, end) {
		return 0, false
	}
	if end+2 < len(text) && text[end] == '[' && text[end+1] >= '0' && text[end+1] <= '4' && text[end+2] == ']' {
		end += 3
	}
	return skipSpace(text, end), true
}

// mathText: simple inline math (a title's $1/f$, Ce$_{17}$, $\gamma$-ray) reads as text;
// anything else stays math.
func mathText(value string) string {
	return inlineMath.ReplaceAllStringFunc(value, func(match string) string {
		// A formula of stars alone ($*$Department of ...) is a mark, as \ast is.
		if starMark.MatchString(match[1 : len(match)-1]) {
			return ""
		}
		// A math alphabet or text box prints its letters ($\mathbb{P}^1$, $\textrm{Ba}_{1-x}$); math
		// spacing and an empty group print nothing.
		body := mathSpacing.ReplaceAllLiteralString(mathAlphabet.ReplaceAllString(match[1:len(match)-1], "${1}${2}"), "")
		body = mathCommand.ReplaceAllStringFunc(body, func(command string) string {
			name := mathCommand.FindStringSubmatch(command)[1]
			if symbol, ok := mathSymbols[name]; ok {
				return symbol
			}
			return command
		})
		body = subscript.ReplaceAllStringFunc(body, func(text string) string {
			groups := subscript.FindStringSubmatch(text)
			return digitsTo(groups[1]+groups[2], "₀₁₂₃₄₅₆₇₈₉")
		})
		body = superscript.ReplaceAllStringFunc(body, func(text string) string {
			groups := superscript.FindStringSubmatch(text)
			return digitsTo(groups[1]+groups[2], "⁰¹²³⁴⁵⁶⁷⁸⁹")
		})
		// Any other script keeps its mark, as a title's $L^p$ does: L_∞, s_++; a plain group
		// (${\mathrm{H_2}}$) only groups.
		body = mathGroup.ReplaceAllString(scriptGroup.ReplaceAllString(body, "$1$2"), "$1")
		if plainMath.MatchString(body) {
			return body
		}
		return match
	})
}

var (
	// abstractHeading is a centred bold "Abstract" line heading the abstract's own text.
	abstractHeading = rx(`(?i)\\begin\{center\}\s*(?:\\textbf\s*\{\s*Abstract\s*\}|\{\\bf\s+Abstract\s*\})\s*(?:\\\\)?\s*\\end\{center\}`)
	// displayEnvironments: a numbered or starred display environment is a formula like $$...$$.
	displayEnvironments = displayEnvironmentPatterns("equation", "align", "gather", "multline", "eqnarray",
		"displaymath", "alignat", "flalign")
)

func displayEnvironmentPatterns(names ...string) []*regexp.Regexp {
	patterns := []*regexp.Regexp{}
	for _, name := range names {
		patterns = append(patterns, regexp.MustCompile(`(?s)\\begin\{`+name+`\*?\}(.*?)\\end\{`+name+`\*?\}`))
	}
	return patterns
}

// inlineEnvironments reads each display environment as a $...$ formula.
func inlineEnvironments(value string) string {
	for _, pattern := range displayEnvironments {
		value = pattern.ReplaceAllString(value, "$${1}$")
	}
	return value
}

// abstractDecorations drops what an abstract carries beside its prose: REVTeX/EPJ's \PACS{...}
// codes, which print as a block of their own, and a centred "Abstract" heading line.
func (t *TexMetadata) abstractDecorations(value string) string {
	text := abstractHeading.ReplaceAllLiteralString(value, " ")
	for {
		start, end, _, ok := findCommand(text, 0, []string{"PACS"}, beforeBrace)
		if !ok {
			return text
		}
		_, ending := t.argument(text, end)
		text = text[:start] + text[ending:]
	}
}

func digitsTo(digits, alphabet string) string {
	symbols := []rune(alphabet)
	var builder strings.Builder
	for _, digit := range digits {
		builder.WriteRune(symbols[digit-'0'])
	}
	return builder.String()
}
