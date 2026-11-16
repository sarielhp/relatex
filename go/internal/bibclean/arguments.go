package bibclean

import (
	"fmt"
	"strings"
)

// Commands whose braced arguments the cleaner reads (decision 173): @preamble macros with plain
// arguments, the family changes \textrm and \textsf, and \ensuremath.

// readCommandArgument reads the braced argument of the command \CMD at POS; on a missing one it records
// the reason and returns false (POS restored).
func (c *textCleaner) readCommandArgument(src, cmd string, pos *int) (string, bool) {
	saved := *pos
	skipSpaces(src, pos)
	arg, ok := readBalancedBraces(src, pos)
	if ok {
		return arg, true
	}
	*pos = saved
	c.reasons = append(c.reasons, Reason{
		Kind:   "unknown_command",
		Name:   `\` + cmd,
		Field:  c.field,
		Detail: fmt.Sprintf("command \\%s missing braced argument", cmd),
	})
	return "", false
}

// consumeFamilyCommand writes \textrm{..} and \textsf{..} as their text. In a field a style may change
// the case of, the text is protected as a brace group is: biblatex's sentence case leaves a command's
// argument alone.
func (c *textCleaner) consumeFamilyCommand(src string, start int, cmd string, pos *int) []textPiece {
	arg, ok := c.readCommandArgument(src, cmd, pos)
	if !ok {
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	if !isCaseField(c.field) {
		return c.cleanSequence(arg)
	}
	return []textPiece{{text: "{#[" + renderPieces(c.cleanSequence(arg), false) + "]}", isCall: true, raw: true}}
}

// consumeEnsureMath reads \ensuremath{..} as the inline formula $..$ it typesets in text.
func (c *textCleaner) consumeEnsureMath(src string, start int, pos *int) []textPiece {
	arg, ok := c.readCommandArgument(src, "ensuremath", pos)
	if !ok {
		return []textPiece{{text: src[start:*pos], raw: true}}
	}
	formula := "$" + arg + "$"
	at := 0
	return []textPiece{c.consumeDollarMath(formula, &at)}
}

// macroBody reads the macro's braced arguments after the command at POS and returns its body with #1..#n
// replaced by them, as TeX substitutes them; false when an argument is missing. A macro without arguments
// takes its trailer ({} or the spaces after its name).
func macroBody(src string, pos *int, macro preambleMacro) (string, bool) {
	if macro.args == 0 {
		consumeCommandTrailer(src, pos)
		return macro.body, true
	}
	saved := *pos
	replacements := make([]string, 0, 2*macro.args)
	for index := 1; index <= macro.args; index++ {
		skipSpaces(src, pos)
		argument, ok := readBalancedBraces(src, pos)
		if !ok {
			*pos = saved
			return "", false
		}
		replacements = append(replacements, fmt.Sprintf("#%d", index), argument)
	}
	return strings.NewReplacer(replacements...).Replace(macro.body), true
}
