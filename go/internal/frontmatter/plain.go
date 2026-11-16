package frontmatter

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// plain is PaperTexMetadata#plain: literal front-matter source as the text it prints;
// a macro it cannot read is a Failure.
func (t *TexMetadata) plain(value string, literal bool) string {
	return t.plainText(value, literal, false)
}

// plainTitle is plain(value, title: true): a title prints its text superscripts.
func (t *TexMetadata) plainTitle(value string) string { return t.plainText(value, false, true) }

func (t *TexMetadata) plainText(value string, literal, title bool) string {
	if title {
		value = wordSuperscripts(value)
	}
	notes := t.withoutNotes(affiliationMark.ReplaceAllLiteralString(value, ""), title)
	notes = ensureMath.ReplaceAllStringFunc(notes, func(match string) string {
		return "$" + ensureMath.FindStringSubmatch(match)[1] + "$"
	})
	text := lineBreak.ReplaceAllLiteralString(mathText(affiliationMark.ReplaceAllLiteralString(notes, "")), " ")
	// The dotless \i and \j carry accents (Para\'{\i}ba).
	text = replaceCommands(text, []string{"i", "j"}, func(text string, end int) (int, bool) {
		if !notLetter(text, end) {
			return 0, false
		}
		return skipSpace(text, end), true
	}, func(name string) string { return name })
	text = accent.ReplaceAllStringFunc(text, func(match string) string {
		groups := accent.FindStringSubmatch(match)
		return groups[2] + groups[3] + accents[groups[1]]
	})
	text = letterAccent.ReplaceAllStringFunc(text, func(match string) string {
		groups := letterAccent.FindStringSubmatch(match)
		return groups[2] + groups[3] + letterAccents[groups[1]]
	})
	// Escaped braces and dollars are literal characters: kept aside through brace stripping.
	text = t.caseChanged(escapedCharacter.ReplaceAllStringFunc(text, func(match string) string { return escaped[match[1:]] }))
	return t.finish(text, literal)
}

// wordSuperscript: a superscript inside a title's word (P$^2$NFFT) is title text, not an
// author mark. RE2 has no lookaround, so the letters on either side are matched and kept, and
// the replacement repeats until adjacent superscripts (a$^2$b$^3$c) are all read.
var wordSuperscript = rx(`([\p{L}\p{N}])\$\^(?:\{([^{}$]*)\}|([^{}$\s\\]))\$([\p{L}\p{N}])`)

func wordSuperscripts(value string) string {
	for {
		next := wordSuperscript.ReplaceAllString(value, `${1}\textsuperscript{${2}${3}}${4}`)
		if next == value {
			return value
		}
		value = next
	}
}

// letterEnd is (?![A-Za-z])(?:\{\}|\s*): a letter command's end, with the {} or the spaces
// after it.
func letterEnd(text string, end int) (int, bool) {
	if !notLetter(text, end) {
		return 0, false
	}
	if strings.HasPrefix(text[end:], "{}") {
		return end + 2, true
	}
	return skipSpace(text, end), true
}

func (t *TexMetadata) finish(text string, literal bool) string {
	// A font the paper defines itself (\font\dos=cmti10) is a font switch too.
	switches := append([]string{}, fontSwitches...)
	for _, match := range fontDefinition.FindAllStringSubmatch(t.definitions, -1) {
		switches = append(switches, match[1])
	}
	// A register assignment (\looseness=-1, \parskip=0pt) prints nothing.
	text = register.ReplaceAllLiteralString(text, "")
	text = linkEscape.ReplaceAllString(text, "$1")
	text = replaceCommands(text, switches, wordEndSpace, func(string) string { return "" })
	text = replaceCommands(text, letterNames, letterEnd, func(name string) string { return letters[name] })
	for _, name := range affiliationFields {
		text = replaceCommands(text, []string{name}, wordEndSpace, func(string) string { return " " })
	}
	for _, name := range wrappers {
		text = replaceCommands(text, []string{name}, wordEndSpace, func(string) string { return "" })
	}
	if strings.ContainsAny(text, `\$`) {
		rec.Fail("Unsupported macro in literal ACM metadata: %s", unsupportedMacro.FindString(text))
	}
	text = strings.NewReplacer("{", "", "}", "", "~", " ", "", "{", "", "}", "", "$").Replace(text)
	if !literal {
		// An 8-bit font's ligature program (T1, OT1) prints a typed en dash and a hyphen as one em dash.
		text = strings.ReplaceAll(text, "–-", "—")
		text = strings.ReplaceAll(text, "---", "—")
		text = strings.ReplaceAll(text, "--", "–")
		text = strings.ReplaceAll(text, "``", "“")
		text = strings.ReplaceAll(text, "''", "”")
		text = strings.NewReplacer("`", "‘", "'", "’").Replace(text)
	}
	return nfc(strip(squeezeSpace(text, " ")))
}
