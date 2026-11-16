package bibclean

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

var verbatimFields = map[string]bool{
	"url":         true,
	"doi":         true,
	"eprint":      true,
	"eprinttype":  true,
	"eprintclass": true,
	"file":        true,
	"pdf":         true,
	"isbn":        true,
	"issn":        true,
	"isrn":        true,
	"date":        true,
	"eventdate":   true,
	"urldate":     true,
	"crossref":    true,
	"xdata":       true,
	"ids":         true,
	"langid":      true,
	"hyphenation": true,
}

type textPiece struct {
	text   string
	isCall bool
	raw    bool
}

type textCleaner struct {
	env      *macroEnv
	field    string
	verbatim bool
	reasons  []Reason
	active   map[string]bool
	math     *mathHook
}

func cleanFieldText(env *macroEnv, field, raw string, hook *mathHook) (string, []Reason) {
	collapsed := collapseBibWhitespace(raw)
	c := &textCleaner{
		env:      env,
		field:    field,
		verbatim: verbatimFields[strings.ToLower(field)],
		active:   map[string]bool{},
		math:     hook,
	}
	if c.verbatim {
		out := c.cleanVerbatim(collapsed)
		return out, c.reasons
	}
	if isNameField(field) {
		collapsed = lowerNameSeparators(collapsed)
	}
	pieces := c.cleanSequence(collapsed)
	out := renderPieces(pieces, true)
	return out, c.reasons
}

func collapseBibWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for i := 0; i < len(s); i++ {
		if isASCIISpace(s[i]) {
			if !inSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			inSpace = true
			continue
		}
		inSpace = false
		b.WriteByte(s[i])
	}
	return strings.TrimRight(b.String(), " ")
}

func (c *textCleaner) cleanSequence(src string) []textPiece {
	var pieces []textPiece
	var buf strings.Builder
	flushBuf := func() {
		if buf.Len() > 0 {
			pieces = append(pieces, textPiece{text: buf.String()})
			buf.Reset()
		}
	}
	pos := 0
	for pos < len(src) {
		ch := src[pos]
		switch {
		case ch == '$':
			flushBuf()
			pieces = append(pieces, c.consumeDollarMath(src, &pos))
		case strings.HasPrefix(src[pos:], `\(`):
			flushBuf()
			pieces = append(pieces, c.consumeParenMath(src, &pos))
		case ch == '{':
			flushBuf()
			pieces = append(pieces, c.consumeBraceGroup(src, &pos)...)
		case ch == '\\':
			flushBuf()
			pieces = append(pieces, c.consumeCommand(src, &pos)...)
		default:
			c.consumePlainToken(src, &pos, &buf)
		}
	}
	flushBuf()
	return pieces
}

func (c *textCleaner) consumePlainToken(src string, pos *int, buf *strings.Builder) {
	rest := src[*pos:]
	switch {
	case strings.HasPrefix(rest, "---"):
		buf.WriteString("—")
		*pos += 3
	case strings.HasPrefix(rest, "--"):
		buf.WriteString("–")
		*pos += 2
	case strings.HasPrefix(rest, "``"):
		buf.WriteString("“")
		*pos += 2
	case strings.HasPrefix(rest, "''"):
		buf.WriteString("”")
		*pos += 2
	case rest[0] == '`':
		buf.WriteString("‘")
		*pos++
	case rest[0] == '~':
		buf.WriteByte('~')
		*pos++
	case (rest[0] == '_' || rest[0] == '^') && !unprintedFields[strings.ToLower(c.field)]:
		// TeX sets a bare script in a printed field as math after an error (JCDCG_2004 and a note
		// SIGGRAPH_1992 print a subscript); fields biblatex never prints (acceptrate) are left alone.
		c.reasons = append(c.reasons, Reason{
			Kind:   "text_script",
			Field:  c.field,
			Detail: fmt.Sprintf("%c outside math", rest[0]),
		})
		writeTypstEscapedRune(buf, src, *pos, rune(rest[0]))
		*pos++
	case rest[0] == '&' && !unprintedFields[strings.ToLower(c.field)]:
		// A bare & is TeX's misplaced alignment tab: an error, and nothing printed.
		c.reasons = append(c.reasons, Reason{Kind: "text_alignment", Field: c.field, Detail: "& outside a table"})
		buf.WriteByte('&')
		*pos++
	case strings.HasPrefix(rest, "..."):
		// TeX prints three periods; Typst's ... is an ellipsis shorthand.
		buf.WriteString(`.\\..`)
		*pos += 3
	case rest[0] == '\'':
		// TeX's ' is always a right quote; Typst's smart quote opens one at a word start ('09).
		buf.WriteString("’")
		*pos++
	default:
		r, size := utf8.DecodeRuneInString(rest)
		writeTypstEscapedRune(buf, src, *pos, r)
		*pos += size
	}
}

func writeTypstEscapedRune(buf *strings.Builder, src string, pos int, r rune) {
	if strings.ContainsRune("#*_<>@[]\\\"", r) {
		buf.WriteString(`\\`)
		buf.WriteRune(r)
		return
	}
	if r == '/' && isCommentStart(src, pos) {
		buf.WriteString(`\\/`)
		return
	}
	buf.WriteRune(r)
}

func isCommentStart(src string, pos int) bool {
	if pos+1 >= len(src) {
		return false
	}
	// Also after http: (phase 2): Typst autolinks only a lower-case scheme, so a style that capitalizes
	// a title (Https://...) would turn the rest into a comment; LaTeX prints such an address as text.
	next := src[pos+1]
	return next == '/' || next == '*'
}

func (c *textCleaner) consumeBraceGroup(src string, pos *int) []textPiece {
	inner, ok := readBalancedBraces(src, pos)
	if !ok {
		c.reasons = append(c.reasons, Reason{
			Kind:   "unbalanced_braces",
			Field:  c.field,
			Detail: "unbalanced '{'",
		})
		return []textPiece{{text: "{", raw: true}}
	}
	if inner == "" {
		return nil
	}
	if fn, rest, matched := matchFontSwitchGroup(inner); matched {
		cleaned := renderPieces(c.cleanSequence(rest), false)
		return []textPiece{{text: "#" + fn + "[" + cleaned + "]", isCall: true}}
	}
	if c.isSingleSpecialGroup(inner) {
		return c.cleanSequence(inner)
	}
	cleaned := renderPieces(c.cleanSequence(inner), false)
	if isCaseField(c.field) || (isNameField(c.field) && strings.ContainsAny(cleaned, " -~")) {
		return []textPiece{{text: "{#[" + cleaned + "]}", isCall: true, raw: true}}
	}
	return []textPiece{{text: "{" + cleaned + "}", raw: true}}
}

// nameFields are biblatex's name lists.
var nameFields = map[string]bool{
	"author": true, "editor": true, "editora": true, "editorb": true, "editorc": true, "translator": true,
	"annotator": true, "commentator": true, "introduction": true, "foreword": true, "afterword": true,
	"bookauthor": true, "holder": true, "shortauthor": true, "shorteditor": true, "sortname": true,
}

// isNameField reports whether FIELD is a name list. There a brace group holding a space, hyphen or tie
// is one name token to biber, which takes a single initial of it ({Hsien{-}Chih} gives H.,
// {M. van} Kreveld gives M. Kreveld). citegeist drops the braces of the parts it returns, so the group
// is also written as a content block, {#[Hsien#[-]Chih]}, which a style's initials can see.
func isNameField(field string) bool {
	return nameFields[strings.ToLower(field)]
}

// lowerNameSeparators writes the name separator " AND " (any case, outside braces) as " and ": biber
// splits a name list at it, citegeist only at the lower-case word.
func lowerNameSeparators(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 && s[i] == ' ' && i+4 < len(s) && strings.EqualFold(s[i+1:i+4], "and") && s[i+4] == ' ' {
			b.WriteString(" and")
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// isCaseField reports whether a style may change FIELD's case (biblatex's sentence case of titles).
// There a protecting brace group is also written as a Typst content block, {#[VC]}: citegeist drops
// the braces of the fields it returns, while the block survives, renders as its text and shows a
// style's own case change what to leave alone.
// unprintedFields are fields the standard biblatex styles never print (outside biblatex's data model, or
// data only), so TeX never typesets their text.
var unprintedFields = map[string]bool{
	"acceptrate": true, "abstract": true, "annotation": true, "annote": true, "comment": true,
	"keywords": true, "review": true, "timestamp": true, "owner": true, "file": true,
}

func isCaseField(field string) bool {
	field = strings.ToLower(field)
	return strings.HasSuffix(field, "title") || strings.HasSuffix(field, "titleaddon") || field == "journal"
}

func matchFontSwitchGroup(inner string) (string, string, bool) {
	trimmed := strings.TrimLeft(inner, " \t\n\r")
	if !strings.HasPrefix(trimmed, `\`) {
		return "", "", false
	}
	p := 1
	cmd := readAlphaWord(trimmed, &p)
	fn := switchCommandToTypst(cmd)
	if fn == "" {
		return "", "", false
	}
	if p < len(trimmed) && isASCIIAlpha(trimmed[p]) {
		return "", "", false
	}
	skipSpaces(trimmed, &p)
	return fn, trimmed[p:], true
}

func switchCommandToTypst(cmd string) string {
	switch cmd {
	case "em", "it", "itshape":
		return "emph"
	case "bf", "bfseries":
		return "strong"
	case "sc", "scshape":
		return "smallcaps"
	default:
		return ""
	}
}

func (c *textCleaner) isSingleSpecialGroup(inner string) bool {
	if !strings.HasPrefix(inner, `\`) {
		return false
	}
	p := 1
	var cmd string
	if p < len(inner) && !isASCIIAlpha(inner[p]) {
		cmd = inner[p : p+1]
		p++
	} else {
		cmd = readAlphaWord(inner, &p)
	}
	if !isSpecialCharCommand(cmd) {
		return false
	}
	if _, isAcc := combiningAccents[cmd]; isAcc {
		skipSpaces(inner, &p)
		if !consumeAccentTargetBounds(inner, &p) {
			return false
		}
		return strings.TrimSpace(inner[p:]) == ""
	}
	skipSpaces(inner, &p)
	if strings.HasPrefix(inner[p:], "{}") {
		p += 2
	}
	return strings.TrimSpace(inner[p:]) == ""
}

func consumeAccentTargetBounds(src string, pos *int) bool {
	if *pos >= len(src) {
		return false
	}
	if src[*pos] == '{' {
		_, ok := readBalancedBraces(src, pos)
		return ok
	}
	if src[*pos] == '\\' {
		*pos++
		if word := readAlphaWord(src, pos); word != "" {
			return true
		}
		if *pos < len(src) {
			*pos++
			return true
		}
		return false
	}
	_, size := utf8.DecodeRuneInString(src[*pos:])
	*pos += size
	return true
}

func (c *textCleaner) consumeCommand(src string, pos *int) []textPiece {
	start := *pos
	*pos++ // consume '\'
	if *pos >= len(src) {
		c.reasons = append(c.reasons, Reason{Kind: "unknown_command", Name: `\`, Field: c.field})
		return []textPiece{{text: `\\`, raw: true}}
	}
	if !isASCIIAlpha(src[*pos]) {
		return c.consumeSymbolCommand(src, start, pos)
	}
	cmd := readAlphaWord(src, pos)
	return c.consumeAlphaCommand(src, start, cmd, pos)
}

func (c *textCleaner) consumeSymbolCommand(src string, start int, pos *int) []textPiece {
	sym := src[*pos : *pos+1]
	*pos++
	if mark, ok := combiningAccents[sym]; ok {
		return c.consumeAccent(src, start, sym, mark, pos)
	}
	switch sym {
	case "&", "%":
		return []textPiece{{text: sym}}
	case "#", "_", "{", "}":
		if sym == "{" {
			return []textPiece{{text: "#sym.brace.l", isCall: true}}
		}
		if sym == "}" {
			return []textPiece{{text: "#sym.brace.r", isCall: true}}
		}
		return []textPiece{{text: `\\` + sym}}
	case "$":
		return []textPiece{{text: "#sym.dollar", isCall: true}}
	case "-":
		return nil
	case " ", ",":
		return []textPiece{{text: " "}}
	default:
		c.reasons = append(c.reasons, Reason{
			Kind:   "unknown_command",
			Name:   `\` + sym,
			Field:  c.field,
			Detail: fmt.Sprintf("unknown LaTeX command \\%s", sym),
		})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
}

func (c *textCleaner) consumeAlphaCommand(src string, start int, cmd string, pos *int) []textPiece {
	if mark, ok := combiningAccents[cmd]; ok {
		return c.consumeAccent(src, start, cmd, mark, pos)
	}
	if fn := switchCommandToTypst(cmd); fn != "" {
		skipSpaces(src, pos)
		rest := src[*pos:]
		*pos = len(src)
		cleaned := renderPieces(c.cleanSequence(rest), false)
		return []textPiece{{text: "#" + fn + "[" + cleaned + "]", isCall: true}}
	}
	if fn := wrappedCommandToTypst(cmd); fn != "" {
		return c.consumeWrappedCommand(src, start, cmd, fn, pos)
	}
	switch cmd {
	case "textrm", "textsf":
		// A family change only: the text keeps its shape and weight, which is all a style prints of it.
		return c.consumeFamilyCommand(src, start, cmd, pos)
	case "ensuremath":
		return c.consumeEnsureMath(src, start, pos)
	case "url":
		return c.consumeURLCommand(src, start, pos)
	case "href":
		return c.consumeHrefCommand(src, start, pos)
	case "textdollar":
		consumeCommandTrailer(src, pos)
		return []textPiece{{text: "#sym.dollar", isCall: true}}
	case "textquotesingle":
		consumeCommandTrailer(src, pos)
		return []textPiece{{text: `\\'`}}
	case "nobreakspace":
		consumeCommandTrailer(src, pos)
		return []textPiece{{text: "~"}}
	}
	if letter, ok := specialLetters[cmd]; ok {
		consumeCommandTrailer(src, pos)
		return []textPiece{{text: letter}}
	}
	if symbol, ok := textSymbols[cmd]; ok {
		consumeCommandTrailer(src, pos)
		return []textPiece{{text: symbol}}
	}
	if macro, ok := c.env.preamble[cmd]; ok {
		return c.expandPreambleMacro(src, start, macro, pos)
	}
	c.reasons = append(c.reasons, Reason{
		Kind:   "unknown_command",
		Name:   `\` + cmd,
		Field:  c.field,
		Detail: fmt.Sprintf("unknown LaTeX command \\%s", cmd),
	})
	return []textPiece{{text: src[start:*pos], raw: true}}
}

func wrappedCommandToTypst(cmd string) string {
	switch cmd {
	case "emph", "textit":
		return "emph"
	case "textbf":
		return "strong"
	case "textsc":
		return "smallcaps"
	default:
		return ""
	}
}

func (c *textCleaner) consumeWrappedCommand(src string, start int, cmd, fn string, pos *int) []textPiece {
	arg, ok := c.readCommandArgument(src, cmd, pos)
	if !ok {
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	cleaned := renderPieces(c.cleanSequence(arg), false)
	return []textPiece{{text: "#" + fn + "[" + cleaned + "]", isCall: true}}
}

func (c *textCleaner) consumeURLCommand(src string, start int, pos *int) []textPiece {
	saved := *pos
	skipSpaces(src, pos)
	arg, ok := readBalancedBraces(src, pos)
	if !ok {
		*pos = saved
		c.reasons = append(c.reasons, Reason{Kind: "unknown_command", Name: `\url`, Field: c.field})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	sub := &textCleaner{env: c.env, field: c.field, verbatim: true, active: copyActive(c.active), math: c.math}
	urlText := sub.cleanVerbatim(arg)
	if len(sub.reasons) > 0 {
		c.reasons = append(c.reasons, sub.reasons...)
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	return []textPiece{{text: "#link(" + typstStringLiteral(urlText) + ")", isCall: true}}
}

func (c *textCleaner) consumeHrefCommand(src string, start int, pos *int) []textPiece {
	saved := *pos
	skipSpaces(src, pos)
	urlArg, ok1 := readBalancedBraces(src, pos)
	skipSpaces(src, pos)
	labelArg, ok2 := readBalancedBraces(src, pos)
	if !ok1 || !ok2 {
		*pos = saved
		c.reasons = append(c.reasons, Reason{Kind: "unknown_command", Name: `\href`, Field: c.field})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	urlSub := &textCleaner{env: c.env, field: c.field, verbatim: true, active: copyActive(c.active), math: c.math}
	urlText := urlSub.cleanVerbatim(urlArg)
	if len(urlSub.reasons) > 0 {
		c.reasons = append(c.reasons, urlSub.reasons...)
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	labelSub := &textCleaner{env: c.env, field: c.field, active: copyActive(c.active), math: c.math}
	labelText := renderPieces(labelSub.cleanSequence(labelArg), false)
	if len(labelSub.reasons) > 0 {
		c.reasons = append(c.reasons, labelSub.reasons...)
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	return []textPiece{{text: "#link(" + typstStringLiteral(urlText) + ")[" + labelText + "]", isCall: true}}
}

func (c *textCleaner) consumeAccent(src string, start int, cmd, mark string, pos *int) []textPiece {
	saved := *pos
	skipSpaces(src, pos)
	if *pos >= len(src) {
		*pos = saved
		c.reasons = append(c.reasons, Reason{Kind: "unknown_command", Name: `\` + cmd, Field: c.field})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	base, ok := c.readAccentBase(src, pos)
	if !ok {
		*pos = saved
		c.reasons = append(c.reasons, Reason{Kind: "unknown_command", Name: `\` + cmd, Field: c.field})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	return []textPiece{{text: applyCombiningAccent(mark, base)}}
}

func (c *textCleaner) readAccentBase(src string, pos *int) (string, bool) {
	if src[*pos] == '{' {
		inner, ok := readBalancedBraces(src, pos)
		if !ok {
			return "", false
		}
		if inner == "" {
			return "", false
		}
		sub := &textCleaner{env: c.env, field: c.field, active: copyActive(c.active), math: c.math}
		cleaned := renderPieces(sub.cleanSequence(inner), false)
		if len(sub.reasons) > 0 {
			c.reasons = append(c.reasons, sub.reasons...)
			return "", false
		}
		return cleaned, true
	}
	if src[*pos] == '\\' {
		*pos++
		word := readAlphaWord(src, pos)
		if word == "i" || word == "j" {
			consumeCommandTrailer(src, pos)
			return word, true
		}
		if letter, ok := specialLetters[word]; ok {
			consumeCommandTrailer(src, pos)
			return letter, true
		}
		return "", false
	}
	r, size := utf8.DecodeRuneInString(src[*pos:])
	*pos += size
	return string(r), true
}

func (c *textCleaner) expandPreambleMacro(src string, start int, macro preambleMacro, pos *int) []textPiece {
	name := `\` + macro.name
	if !macro.expandable() {
		c.reasons = append(c.reasons, Reason{
			Kind:   "macro_with_args",
			Name:   name,
			Field:  c.field,
			Detail: fmt.Sprintf("@preamble macro %s takes an optional or delimited argument", name),
		})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	if c.active[macro.name] {
		c.reasons = append(c.reasons, Reason{
			Kind:   "non_plain_macro",
			Name:   name,
			Field:  c.field,
			Detail: fmt.Sprintf("recursive @preamble macro %s", name),
		})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	body, ok := macroBody(src, pos, macro)
	if !ok {
		c.reasons = append(c.reasons, Reason{
			Kind:   "macro_with_args",
			Name:   name,
			Field:  c.field,
			Detail: fmt.Sprintf("@preamble macro %s is missing a braced argument", name),
		})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	nextActive := copyActive(c.active)
	nextActive[macro.name] = true
	sub := &textCleaner{env: c.env, field: c.field, verbatim: c.verbatim, active: nextActive, math: c.math}
	pieces := expandPreambleSub(sub, body)
	if len(sub.reasons) > 0 {
		c.reasons = append(c.reasons, Reason{
			Kind:   "non_plain_macro",
			Name:   name,
			Field:  c.field,
			Detail: summarizeReasons(name, sub.reasons),
		})
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	return pieces
}

func expandPreambleSub(sub *textCleaner, body string) []textPiece {
	if sub.verbatim {
		return []textPiece{{text: sub.cleanVerbatim(body), raw: true}}
	}
	return sub.cleanSequence(body)
}

func summarizeReasons(macro string, reasons []Reason) string {
	parts := make([]string, 0, len(reasons))
	seen := map[string]bool{}
	for _, r := range reasons {
		item := r.Name
		if item == "" {
			item = r.Detail
		}
		if item != "" && !seen[item] {
			seen[item] = true
			parts = append(parts, item)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("@preamble macro %s is not plain text", macro)
	}
	return fmt.Sprintf("@preamble macro %s uses %s", macro, strings.Join(parts, ", "))
}

func consumeCommandTrailer(src string, pos *int) {
	skipSpaces(src, pos)
	if strings.HasPrefix(src[*pos:], "{}") {
		*pos += 2
	}
}

func (c *textCleaner) cleanVerbatim(src string) string {
	trimmed := strings.TrimSpace(src)
	if strings.HasPrefix(trimmed, `\url{`) && strings.HasSuffix(trimmed, "}") {
		p := 4
		if inner, ok := readBalancedBraces(trimmed, &p); ok && p == len(trimmed) {
			trimmed = inner
		}
	}
	var b strings.Builder
	pos := 0
	for pos < len(trimmed) {
		ch := trimmed[pos]
		switch ch {
		case '$':
			piece := c.consumeDollarMath(trimmed, &pos)
			b.WriteString(piece.text)
		case '{':
			inner, ok := readBalancedBraces(trimmed, &pos)
			if !ok {
				c.reasons = append(c.reasons, Reason{Kind: "unbalanced_braces", Field: c.field})
				b.WriteByte('{')
				continue
			}
			b.WriteString(c.cleanVerbatimInner(inner))
		case '\\':
			b.WriteString(c.cleanVerbatimCommand(trimmed, &pos))
		default:
			b.WriteByte(ch)
			pos++
		}
	}
	return b.String()
}

func (c *textCleaner) cleanVerbatimInner(inner string) string {
	if len(inner) == 2 && inner[0] == '\\' && strings.ContainsRune("&#%_$", rune(inner[1])) {
		return inner[1:2]
	}
	if strings.HasPrefix(inner, `\`) {
		return c.cleanVerbatim(inner)
	}
	return "{" + c.cleanVerbatim(inner) + "}"
}

func (c *textCleaner) cleanVerbatimCommand(src string, pos *int) string {
	start := *pos
	*pos++
	if *pos >= len(src) {
		c.reasons = append(c.reasons, Reason{Kind: "unknown_command", Name: `\`, Field: c.field})
		return `\`
	}
	if !isASCIIAlpha(src[*pos]) {
		sym := src[*pos : *pos+1]
		*pos++
		switch sym {
		case "_", "%", "#", "&", "$", "~":
			return sym
		case " ":
			return " "
		default:
			c.reasons = append(c.reasons, Reason{Kind: "unknown_command", Name: `\` + sym, Field: c.field})
			return src[start:*pos]
		}
	}
	cmd := readAlphaWord(src, pos)
	if macro, ok := c.env.preamble[cmd]; ok {
		pieces := c.expandPreambleMacro(src, start, macro, pos)
		var b strings.Builder
		for _, p := range pieces {
			b.WriteString(p.text)
		}
		return b.String()
	}
	c.reasons = append(c.reasons, Reason{Kind: "unknown_command", Name: `\` + cmd, Field: c.field})
	return src[start:*pos]
}

func renderPieces(pieces []textPiece, protectLeading bool) string {
	if len(pieces) == 0 {
		return ""
	}
	var b strings.Builder
	for i, piece := range pieces {
		text := piece.text
		if i == 0 && protectLeading && !piece.isCall && !piece.raw {
			text = protectMarkupStart(text)
		}
		b.WriteString(text)
		if piece.isCall && callNeedsSemicolon(pieces, i) {
			b.WriteByte(';')
		}
	}
	return b.String()
}

func callNeedsSemicolon(pieces []textPiece, index int) bool {
	if index+1 >= len(pieces) {
		return false
	}
	next := ""
	for _, p := range pieces[index+1:] {
		if p.text != "" {
			next = p.text
			break
		}
	}
	if next == "" {
		return false
	}
	if strings.HasPrefix(pieces[index].text, "#sym.") {
		return true
	}
	return strings.ContainsRune("([.;", rune(next[0]))
}

func protectMarkupStart(text string) string {
	if text == "" {
		return text
	}
	if len(text) >= 2 && strings.ContainsRune("=+-/", rune(text[0])) && text[1] == ' ' {
		return `\\` + text
	}
	digits := 0
	for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits+1 < len(text) && text[digits] == '.' && text[digits+1] == ' ' {
		return text[:digits] + `\\` + text[digits:]
	}
	return text
}

func typstStringLiteral(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + replacer.Replace(value) + `"`
}

func copyActive(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src)+1)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
