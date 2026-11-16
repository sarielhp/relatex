package bibclean

import (
	"fmt"
	"strings"
	"unicode"
)

func (c *textCleaner) consumeDollarMath(src string, pos *int) textPiece {
	start := *pos
	delim := "$"
	*pos++
	if *pos < len(src) && src[*pos] == '$' {
		delim = "$$"
		*pos++
	}
	end := findClosingMath(src, *pos, delim)
	if end < 0 {
		c.reasons = append(c.reasons, Reason{
			Kind:   "unclosed_math",
			Field:  c.field,
			Detail: fmt.Sprintf("unclosed %s math delimiter", delim),
		})
		*pos = len(src)
		return textPiece{text: src[start:], raw: true}
	}
	body := src[*pos:end]
	*pos = end + len(delim)
	if delim == "$$" {
		c.reasons = append(c.reasons, Reason{
			Kind:   "math_command",
			Field:  c.field,
			Detail: fmt.Sprintf("display math %s%s%s", delim, body, delim),
		})
		return textPiece{text: src[start:*pos], raw: true}
	}
	if routed, ok := c.routeMath(body); ok {
		return routed
	}
	c.checkMathBody(body)
	return textPiece{text: "$" + body + "$", raw: true}
}

func (c *textCleaner) consumeParenMath(src string, pos *int) textPiece {
	start := *pos
	*pos += 2
	end := strings.Index(src[*pos:], `\)`)
	if end < 0 {
		c.reasons = append(c.reasons, Reason{
			Kind:   "unclosed_math",
			Field:  c.field,
			Detail: `unclosed \( math delimiter`,
		})
		*pos = len(src)
		return textPiece{text: src[start:], raw: true}
	}
	body := src[*pos : *pos+end]
	*pos += end + 2
	if routed, ok := c.routeMath(body); ok {
		return routed
	}
	before := len(c.reasons)
	c.checkMathBody(body)
	if len(c.reasons) > before {
		return textPiece{text: src[start:*pos], raw: true}
	}
	return textPiece{text: "$" + body + "$", raw: true}
}

// mathHook is an entry's MathRoute with the count of the formulas it has met.
type mathHook struct {
	route MathRoute
	key   string
	next  int
}

// routeMath is the MathRoute's markup for an inline formula of a printed field, when it gives one.
func (c *textCleaner) routeMath(body string) (textPiece, bool) {
	if c.math == nil || c.verbatim {
		return textPiece{}, false
	}
	index := c.math.next
	c.math.next++
	markup, ok := c.math.route(c.math.key, index, body)
	if !ok {
		return textPiece{}, false
	}
	return textPiece{text: markup, raw: true}, true
}

func findClosingMath(src string, from int, delim string) int {
	for i := from; i < len(src); i++ {
		if src[i] == '\\' && i+1 < len(src) {
			i++
			continue
		}
		if strings.HasPrefix(src[i:], delim) {
			return i
		}
	}
	return -1
}

func (c *textCleaner) checkMathBody(body string) {
	if cmds := findMathCommands(body); len(cmds) > 0 {
		for _, cmd := range cmds {
			c.reasons = append(c.reasons, Reason{
				Kind:   "math_command",
				Name:   `\` + cmd,
				Field:  c.field,
				Detail: fmt.Sprintf("math command \\%s in $%s$", cmd, body),
			})
		}
		return
	}
	for _, r := range body {
		if !isAllowedPlainMathRune(r) {
			c.reasons = append(c.reasons, Reason{
				Kind:   "math_command",
				Field:  c.field,
				Detail: fmt.Sprintf("unsupported character %q in math $%s$", r, body),
			})
			return
		}
	}
	if why := typstReadsDifferently(body); why != "" {
		c.reasons = append(c.reasons, Reason{
			Kind:   "math_typst",
			Field:  c.field,
			Detail: fmt.Sprintf("%s in $%s$", why, body),
		})
	}
}

// typstShorthandRunes form Typst math shorthands when two of them meet (<= is ≤, ... is …, -> is →),
// where TeX prints the characters.
const typstShorthandRunes = "<>=-!:|.~*"

// typstReadsDifferently says why plain TeX math BODY would not print the same as Typst math, or "".
// Decision A's MathML route converts such math; until then its entry stays unconverted.
func typstReadsDifferently(body string) string {
	runes := []rune(body)
	for i, r := range runes {
		var prev rune
		if i > 0 {
			prev = runes[i-1]
		}
		if why := typstRuneReading(prev, r, i == 0); why != "" {
			return why
		}
	}
	return ""
}

func typstRuneReading(prev, r rune, first bool) string {
	switch {
	case unicode.IsLetter(prev) && (unicode.IsLetter(r) || unicode.IsDigit(r)):
		return fmt.Sprintf("Typst reads %q as part of one identifier", string([]rune{prev, r}))
	case r == '{' || r == '}':
		return "Typst prints TeX's grouping braces"
	case r == '/':
		return "Typst makes / a fraction"
	case r == '~':
		return "Typst prints ~ as an operator, TeX as a space"
	case (r == '^' || r == '_' || r == '\'') && (first || !isMathBase(prev)):
		return fmt.Sprintf("%c has no base", r)
	case strings.ContainsRune(typstShorthandRunes, prev) && strings.ContainsRune(typstShorthandRunes, r):
		return fmt.Sprintf("Typst may read %q as a shorthand", string([]rune{prev, r}))
	}
	return ""
}

// isMathBase reports whether R can carry a script: a letter, a digit, a closing delimiter or a prime.
func isMathBase(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(")]'", r)
}

func findMathCommands(body string) []string {
	var cmds []string
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' || i+1 >= len(body) {
			continue
		}
		p := i + 1
		word := readAlphaWord(body, &p)
		if word == "" {
			word = body[i+1 : i+2]
			p = i + 2
		}
		cmds = append(cmds, word)
		i = p - 1
	}
	return cmds
}

func isAllowedPlainMathRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
		return true
	}
	return strings.ContainsRune("+-*/=<>!|,.:;()[]'^_{}~", r)
}
