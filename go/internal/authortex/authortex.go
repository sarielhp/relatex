// Package authortex is the author's TeX as the outputs ship it (decision 102): a formula's TeX
// without labels, comments and blank lines (Shipped), and the kept macro definitions as one
// block (Block). The core's math verification cuts formulas the same way (authormath forwards);
// the Markdown reads them here.
package authortex

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

var labelCommand = regexp.MustCompile(`\\label[ \t\n\v\f\r]*\{[^{}]*\}`)

// Shipped is MathVerification.shipped: the TeX a Markdown reader gets: labels go (references
// use the row anchors), and so do comments and blank lines, which would end the formula in
// Markdown.
func Shipped(text string) string {
	text = withoutComments(strings.ReplaceAll(text, "\r", ""))
	return rec.Strip(labelCommand.ReplaceAllString(rec.WithoutBlankLines(text), ""))
}

// withoutComments: a % starts a comment unless an odd number of backslashes escapes it: \% is
// a percent sign, \\% a row end and then a comment.
func withoutComments(text string) string {
	var builder strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		index := commentIndex(line)
		if index < 0 {
			builder.WriteString(line)
			continue
		}
		builder.WriteString(line[:index])
		if strings.HasSuffix(line, "\n") {
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func commentIndex(line string) int {
	for index := 0; index < len(line); index++ {
		if line[index] != '%' {
			continue
		}
		backslashes := 0
		for at := index - 1; at >= 0 && line[at] == '\\'; at-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			return index
		}
	}
	return -1
}

// Block is MathMacros.block: the definitions shipped with Markdown, read by Pandoc, MathJax
// and KaTeX. A macro may redefine a standard command (\vec): \providecommand then
// \renewcommand works either way.
func Block(kept rec.Map) string {
	names := make([]string, 0, len(kept))
	for name := range kept {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, len(names))
	for index, name := range names {
		macro := rec.AsMap(kept[name])
		parameters := rec.FetchInt(macro, "parameters")
		count := ""
		if parameters > 0 {
			count = fmt.Sprintf("[%d]", parameters)
		}
		lines[index] = "\\providecommand{\\" + name + "}{}\\renewcommand{\\" + name + "}" + count + "{" + rec.FetchString(macro, "body") + "}"
	}
	return strings.Join(lines, "\n")
}

// PairedVariant reports whether the text uses a kept paired delimiter (decision 114) starred or
// sized, \abs*{x} or \abs[\big]{x}.
func PairedVariant(text string, kept rec.Map) bool {
	decls := pairedDeclarations(kept)
	if len(decls) == 0 {
		return false
	}
	for pos := 0; pos < len(text); {
		slash := strings.IndexByte(text[pos:], '\\')
		if slash < 0 {
			return false
		}
		word, end := readControlWord(text, pos+slash)
		if word == "" {
			pos = end
			continue
		}
		pos = end
		if _, paired := decls[word]; !paired {
			continue
		}
		after := skipSpaces(text, end)
		if after < len(text) && (text[after] == '*' || text[after] == '[') {
			return true
		}
	}
	return false
}

const maxPairedDepth = 16

type pairedDeclaration struct {
	parameters                        int
	before, open, close, after, inner string
}

var sizeDelimiters = map[string][2]string{
	`\big`:  {`\bigl`, `\bigr`},
	`\Big`:  {`\Bigl`, `\Bigr`},
	`\bigg`: {`\biggl`, `\biggr`},
	`\Bigg`: {`\Biggl`, `\Biggr`},
}

// ExpandPairedVariants expands starred and sized uses of kept paired delimiters (\abs*{x},
// \abs[\big]{x}) to plain TeX delimiters (\left\lvert x\right\rvert, \bigl\lvert x\bigr\rvert)
// that Markdown's Pandoc, MathJax and KaTeX readers accept. Plain uses stay unexpanded. ok is false
// when an expansion is not sound (non-standard sizes, sized null delimiters, or malformed calls), so
// the caller keeps its rendered fallback.
func ExpandPairedVariants(text string, kept rec.Map) (string, bool) {
	decls := pairedDeclarations(kept)
	if len(decls) == 0 {
		return text, true
	}
	return expandPaired(text, decls, 0)
}

func pairedDeclarations(kept rec.Map) map[string]pairedDeclaration {
	decls := map[string]pairedDeclaration{}
	for name, value := range kept {
		macro, _ := value.(rec.Map)
		paired, ok := macro["paired"].(rec.Map)
		if !ok {
			continue
		}
		decls[name] = pairedDeclaration{
			parameters: intField(macro, "parameters"),
			before:     stringField(paired, "before"),
			open:       stringField(paired, "open"),
			close:      stringField(paired, "close"),
			after:      stringField(paired, "after"),
			inner:      stringField(paired, "inner"),
		}
	}
	return decls
}

func stringField(m rec.Map, key string) string {
	v, _ := m[key].(string)
	return v
}

func intField(m rec.Map, key string) int {
	switch v := m[key].(type) {
	case int64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func expandPaired(text string, decls map[string]pairedDeclaration, depth int) (string, bool) {
	if depth >= maxPairedDepth {
		return "", false
	}
	var out strings.Builder
	for pos := 0; pos < len(text); {
		slash := strings.IndexByte(text[pos:], '\\')
		if slash < 0 {
			out.WriteString(text[pos:])
			break
		}
		out.WriteString(text[pos : pos+slash])
		at := pos + slash
		word, end := readControlWord(text, at)
		decl, paired := decls[word]
		if word == "" || !paired {
			out.WriteString(text[at:end])
			pos = end
			continue
		}
		expanded, nextPos, matched, ok := expandPairedCall(text, end, decl, decls, depth)
		if !ok {
			return "", false
		}
		if !matched {
			out.WriteString(text[at:end])
			pos = end
			continue
		}
		out.WriteString(expanded)
		pos = nextPos
	}
	return out.String(), true
}

func expandPairedCall(text string, afterWord int, decl pairedDeclaration, decls map[string]pairedDeclaration, depth int) (string, int, bool, bool) {
	optAt := skipSpaces(text, afterWord)
	if optAt >= len(text) || (text[optAt] != '*' && text[optAt] != '[') {
		return "", afterWord, false, true
	}
	if decl.open == "" || decl.close == "" || decl.parameters < 0 || decl.parameters > 9 || strings.Contains(decl.inner, `\delimsize`) {
		return "", 0, true, false
	}
	leftPrefix, rightPrefix, argStart, starred, ok := parseVariantOption(text, optAt)
	if !ok || (!starred && (decl.open == "." || decl.close == ".")) {
		return "", 0, true, false
	}
	args, nextPos, ok := readMacroArgs(text, argStart, decl.parameters)
	if !ok {
		return "", 0, true, false
	}
	expandedArgs := make([]string, len(args))
	for i, arg := range args {
		expanded, ok := expandPaired(arg, decls, depth+1)
		if !ok {
			return "", 0, true, false
		}
		expandedArgs[i] = expanded
	}
	inner := substituteArgs(decl.inner, expandedArgs)
	return formatPairedExpansion(decl, leftPrefix, rightPrefix, inner, tailFollowsLetter(text, nextPos, decl.after)), nextPos, true, true
}

func parseVariantOption(text string, optAt int) (leftPrefix, rightPrefix string, nextPos int, starred, ok bool) {
	if text[optAt] == '*' {
		return `\left`, `\right`, optAt + 1, true, true
	}
	closeAt, found := findBracketClose(text, optAt+1)
	if !found {
		return "", "", 0, false, false
	}
	size := strings.TrimSpace(text[optAt+1 : closeAt])
	pair, known := sizeDelimiters[size]
	if !known {
		return "", "", 0, false, false
	}
	return pair[0], pair[1], closeAt + 1, false, true
}

func findBracketClose(text string, start int) (int, bool) {
	depth := 0
	for pos := start; pos < len(text); {
		switch text[pos] {
		case '\\':
			pos = skipEscape(text, pos)
		case '{':
			depth++
			pos++
		case '}':
			if depth == 0 {
				return 0, false
			}
			depth--
			pos++
		case ']':
			if depth == 0 {
				return pos, true
			}
			pos++
		default:
			pos++
		}
	}
	return 0, false
}

func readMacroArgs(text string, start, count int) ([]string, int, bool) {
	args := make([]string, count)
	pos := start
	for i := 0; i < count; i++ {
		arg, nextPos, ok := readOneArg(text, skipSpaces(text, pos))
		if !ok {
			return nil, 0, false
		}
		args[i] = arg
		pos = nextPos
	}
	return args, pos, true
}

func readOneArg(text string, pos int) (string, int, bool) {
	if pos >= len(text) || text[pos] == '}' {
		return "", 0, false
	}
	if text[pos] == '{' {
		return readBracedArg(text, pos)
	}
	if text[pos] == '\\' {
		if pos+1 >= len(text) {
			return "", 0, false
		}
		_, end := readControlWord(text, pos)
		return text[pos:end], end, true
	}
	_, size := utf8.DecodeRuneInString(text[pos:])
	return text[pos : pos+size], pos + size, true
}

func readBracedArg(text string, start int) (string, int, bool) {
	depth := 1
	for pos := start + 1; pos < len(text); {
		switch text[pos] {
		case '\\':
			pos = skipEscape(text, pos)
		case '{':
			depth++
			pos++
		case '}':
			depth--
			if depth == 0 {
				return text[start+1 : pos], pos + 1, true
			}
			pos++
		default:
			pos++
		}
	}
	return "", 0, false
}

func substituteArgs(pattern string, args []string) string {
	var out strings.Builder
	for pos := 0; pos < len(pattern); pos++ {
		if pattern[pos] != '#' || pos+1 >= len(pattern) || pattern[pos+1] < '1' || pattern[pos+1] > '9' {
			out.WriteByte(pattern[pos])
			continue
		}
		idx := int(pattern[pos+1] - '1')
		if idx >= len(args) {
			out.WriteString(pattern[pos : pos+2])
			pos++
			continue
		}
		out.WriteString(args[idx])
		if endsWithControlWord(args[idx]) && pos+2 < len(pattern) && isASCIILetter(pattern[pos+2]) {
			out.WriteByte(' ')
		}
		pos++
	}
	return out.String()
}

func formatPairedExpansion(decl pairedDeclaration, leftPrefix, rightPrefix, inner string, nextIsLetter bool) string {
	var out strings.Builder
	out.WriteString(decl.before)
	out.WriteString(leftPrefix)
	out.WriteString(decl.open)
	if isControlWord(decl.open) {
		out.WriteByte(' ')
	}
	out.WriteString(inner)
	out.WriteString(rightPrefix)
	out.WriteString(decl.close)
	if isControlWord(decl.close) && nextIsLetter {
		out.WriteByte(' ')
	}
	out.WriteString(decl.after)
	return out.String()
}

func readControlWord(text string, slash int) (string, int) {
	end := slash + 1
	if end >= len(text) {
		return "", end
	}
	if !isASCIILetter(text[end]) {
		return "", end + 1
	}
	for end < len(text) && isASCIILetter(text[end]) {
		end++
	}
	return text[slash+1 : end], end
}

func skipEscape(text string, slash int) int {
	if slash+1 < len(text) {
		return slash + 2
	}
	return slash + 1
}

func skipSpaces(text string, pos int) int {
	for pos < len(text) && strings.IndexByte(" \t\n\v\f\r", text[pos]) >= 0 {
		pos++
	}
	return pos
}

func isControlWord(token string) bool {
	return len(token) > 1 && token[0] == '\\' && isASCIILetter(token[1])
}

func endsWithControlWord(text string) bool {
	end := len(text)
	pos := end
	for pos > 0 && isASCIILetter(text[pos-1]) {
		pos--
	}
	if pos == end || pos == 0 || text[pos-1] != '\\' {
		return false
	}
	// An odd run of backslashes means the trailing backslash starts a control word rather than escaping another \.
	slashes := 0
	for i := pos - 1; i >= 0 && text[i] == '\\'; i-- {
		slashes++
	}
	return slashes%2 == 1
}

func tailFollowsLetter(text string, nextPos int, after string) bool {
	if after != "" {
		return isASCIILetter(after[0])
	}
	return nextPos < len(text) && isASCIILetter(text[nextPos])
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
