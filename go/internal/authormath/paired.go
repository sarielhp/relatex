package authormath

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// A mathtools paired delimiter (\DeclarePairedDelimiter, ...X, ...XPP; decision 114) is a robust
// command whose body picks its starred or plain form. The collector reports the starred form's
// meaning as "NAME* ": the delimiters, the body and what stands before and after. The plain form
// \abs{x} sets \mathopen OPEN x \mathclose CLOSE (mathtools' nonscaled wrapper), so the kept macro
// is that body, which Markdown, MathJax and KaTeX read. Its Paired form keeps the declaration, so
// the math verification declares the macro again with mathtools and a starred or sized use
// (\abs*{x}, \abs[\big]{x}) verifies too (decision 114, amendment 2026-11-13).
var (
	pairedChoice = regexp.MustCompile(`\A->\\@ifstar \{\\@nameuse \{MT_delim_\\MH_cs_to_str:N \\([A-Za-z]+) _star:\}\}`)
	pairedStar   = regexp.MustCompile(`(?s)\A((?:#\d)+)->(?:\\begingroup \\def \\delimsize \{\\middle \}(.*?))?` +
		`\\@nameuse \{MT_delim_\\MH_cs_to_str:N \\([A-Za-z]+) _star_wrapper:nnn\}\{\\left (.+?)\}\{(.*)\}\{\\right (.+?)\}` +
		`(.*?)(\\endgroup )?\z`)
	// pairedDelimiter is one delimiter token: a control word, a control symbol or a character.
	pairedDelimiter = regexp.MustCompile(`\A(?:\\[A-Za-z]+|\\[^A-Za-z]|[^\\{}\s])\z`)
	controlWord     = regexp.MustCompile(`\A\\[A-Za-z]+\z`)
)

// pairedMacro reads a paired delimiter's starred meaning into the kept macro of its plain form;
// ok is false for anything else.
func pairedMacro(name, inner string, meanings *omap.Map[string, any]) (Macro, bool) {
	choice := pairedChoice.FindStringSubmatch(inner)
	if choice == nil || choice[1] != name {
		return Macro{}, false
	}
	value, found := meanings.Get(name + "* ")
	star, isText := value.(string)
	if !found || !isText {
		return Macro{}, false
	}
	match := pairedStar.FindStringSubmatch(star)
	// The X forms open a group the plain form closes; the simple form has neither.
	grouped := strings.Contains(star, `->\begingroup `)
	if match == nil || match[3] != name || grouped != (match[8] != "") {
		return Macro{}, false
	}
	open, closing := strings.TrimSpace(match[4]), strings.TrimSpace(match[6])
	if !pairedDelimiter.MatchString(open) || !pairedDelimiter.MatchString(closing) || strings.Contains(match[5], `\delimsize`) {
		return Macro{}, false
	}
	body := strings.TrimSpace(match[2]) + side(`\mathopen`, open) + match[5] + side(`\mathclose`, closing) + strings.TrimSpace(match[7])
	body = rec.Strip(body)
	if forbidden(body) || !balanced(body) {
		return Macro{}, false
	}
	paired := Paired{Before: strings.TrimSpace(match[2]), Open: open, Close: closing, After: strings.TrimSpace(match[7]), Inner: match[5]}
	return Macro{Parameters: int64(strings.Count(match[1], "#")), Body: body, Paired: paired}, true
}

// Paired is a paired delimiter's declaration: what \DeclarePairedDelimiterXPP sets before the
// opening delimiter, the two delimiters, what follows the closing one and the body between them.
// The zero value is a macro of another kind.
type Paired struct {
	Before, Open, Close, After, Inner string
}

// Map is the declaration as document.json keeps it (math_macros' paired).
func (p Paired) Map() rec.Map {
	return rec.Map{"before": p.Before, "open": p.Open, "close": p.Close, "after": p.After, "inner": p.Inner}
}

// declaration is the mathtools command that defines the macro again: the simple form where it is
// one (one parameter, nothing before or after), else the XPP form with the same parts.
func (p Paired) declaration(name string, parameters int64) string {
	if parameters == 1 && p.Before == "" && p.After == "" && p.Inner == "#1" {
		return `\DeclarePairedDelimiter{\` + name + `}{` + p.Open + `}{` + p.Close + `}`
	}
	return fmt.Sprintf(`\DeclarePairedDelimiterXPP{\%s}[%d]{%s}{%s}{%s}{%s}{%s}`, name, parameters, p.Before, p.Open, p.Close,
		p.After, p.Inner)
}

// PairedBlock is the verification's declarations of the kept paired delimiters, after the
// definitions of authortex.Block: each macro is made undefined, then declared with mathtools as
// the paper did, so \abs*{x} and \abs[\big]{x} typeset as in the paper.
func PairedBlock(kept rec.Map) string {
	names := make([]string, 0, len(kept))
	for name := range kept {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := []string{}
	for _, name := range names {
		macro := rec.AsMap(kept[name])
		paired, ok := macro["paired"].(rec.Map)
		if !ok {
			continue
		}
		form := Paired{Before: rec.FetchString(paired, "before"), Open: rec.FetchString(paired, "open"),
			Close: rec.FetchString(paired, "close"), After: rec.FetchString(paired, "after"), Inner: rec.FetchString(paired, "inner")}
		lines = append(lines, `\let\`+name+`\relax`+form.declaration(name, rec.FetchInt(macro, "parameters")))
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n" + strings.Join(lines, "\n")
}

// side is one delimiter of the plain form: none for the null delimiter, and a space after a
// control word so the body reads on.
func side(class, delimiter string) string {
	if delimiter == "." {
		return ""
	}
	if controlWord.MatchString(delimiter) {
		return class + delimiter + " "
	}
	return class + delimiter
}
