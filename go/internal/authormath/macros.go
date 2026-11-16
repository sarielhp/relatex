// Package authormath is the Go port of decision 064, the author's TeX for each formula:
// MathSource and MathSourceText cut it from the paper's files, MathMacros keeps the paper's
// simple math macros, and MathVerification typesets it again and compares the MathML
// (MathStructure) with the paper's.
package authormath

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

var (
	operatorMeaning  = regexp.MustCompile(`(?s)\A->\\qopname \\newmcodes@ ([om])\{(.*)\}\z`)
	protectedMeaning = regexp.MustCompile(`\A->\\protect \\([A-Za-z]+)  \z`)
	parameterHead    = regexp.MustCompile(`\A(?:#\d)*\z`)
	ensuremath       = regexp.MustCompile(`(?s)\A\\ensuremath \{(.*)\}\z`)
	balanceToken     = regexp.MustCompile(`(?s)\\.|[{}]`)
	usedName         = regexp.MustCompile(`\\([A-Za-z]+)`)
)

// forbiddenWords and forbiddenPrefixes are MathMacros::FORBIDDEN: tokens a kept body may not
// contain (control flow, expansion tricks, definitions, internals). The pattern ends in \b,
// which Ruby reads with Unicode word characters; it is matched by hand.
var (
	forbiddenWords = []string{"else", "fi", "or", "csname", "endcsname", "expandafter", "noexpand", "catcode", "def", "gdef",
		"edef", "xdef", "let", "futurelet", "afterassignment", "ensuremath", "relax", "begingroup", "endgroup", "global",
		"mathchoice", "hbox", "vbox", "kern", "hskip", "vskip", "raisebox", "footnote", "label", "ref"}
	// prefix, then a run of the characters the pattern repeats.
	forbiddenPrefixes = []struct {
		prefix string
		run    func(rune) bool
	}{{"if", isLower}, {"protected@", isASCIIWord}, {"@", isASCIIWord}, {"cite", isASCIIWord}}
)

func isLower(character rune) bool { return character >= 'a' && character <= 'z' }

func isASCIIWord(character rune) bool {
	return character == '_' || (character >= '0' && character <= '9') || (character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z')
}

// wordCharacter is Onigmo's word character for \b on a UTF-8 string.
func wordCharacter(character rune) bool {
	return character == '_' || unicode.IsLetter(character) || unicode.IsDigit(character) || unicode.Is(unicode.Mn, character) ||
		unicode.Is(unicode.Nd, character) || unicode.Is(unicode.Pc, character)
}

// boundaryAt is \b between text[:at] and text[at:].
func boundaryAt(text string, at int) bool {
	before, after := false, false
	if at > 0 {
		character, _ := utf8.DecodeLastRuneInString(text[:at])
		before = wordCharacter(character)
	}
	if at < len(text) {
		character, _ := utf8.DecodeRuneInString(text[at:])
		after = wordCharacter(character)
	}
	return before != after
}

// forbidden is body.match?(FORBIDDEN).
func forbidden(body string) bool {
	for index := 0; index < len(body); index++ {
		if body[index] == '\\' && forbiddenAt(body, index+1) {
			return true
		}
	}
	return false
}

func forbiddenAt(text string, start int) bool {
	rest := text[start:]
	for _, word := range forbiddenWords {
		if strings.HasPrefix(rest, word) && boundaryAt(text, start+len(word)) {
			return true
		}
	}
	for _, pattern := range forbiddenPrefixes {
		if !strings.HasPrefix(rest, pattern.prefix) {
			continue
		}
		// The run backtracks: any of its lengths may end at a boundary.
		at := start + len(pattern.prefix)
		for {
			if boundaryAt(text, at) {
				return true
			}
			character, width := utf8.DecodeRuneInString(text[at:])
			if at >= len(text) || !pattern.run(character) {
				break
			}
			at += width
		}
	}
	return false
}

// Macro is a kept macro: its parameter count, its body and whether it makes a formula in text.
type Macro struct {
	Parameters int64
	Body       string
	TextMode   bool
	// Paired is a mathtools paired delimiter's declaration (decision 114), zero for other macros.
	Paired Paired
}

// Map is the macro as the document stores it.
func (m Macro) Map() rec.Map {
	result := rec.Map{"parameters": m.Parameters, "body": m.Body}
	if m.TextMode {
		result["text_mode"] = true
	}
	if m.Paired.Open != "" {
		result["paired"] = m.Paired.Map()
	}
	return result
}

// classify is MathMacros.classify: the kept macros, closed under use (a body may use only
// standard commands and other kept macros), and the rejected names with their reasons.
func classify(records []rec.Map) (*omap.Map[string, Macro], *omap.Map[string, string]) {
	meanings := omap.New[string, any]()
	for _, row := range records {
		meanings.Set(rec.FetchString(row, "name"), row["meaning"])
	}
	kept := omap.New[string, Macro]()
	shapes := omap.New[string, any]()
	for _, name := range meanings.Keys() {
		if strings.HasSuffix(name, " ") {
			continue
		}
		macro, reason := shape(name, meanings)
		if reason == "" {
			kept.Set(name, macro)
			shapes.Set(name, macro)
		} else {
			shapes.Set(name, reason)
		}
	}
	for {
		dropped := []string{}
		for _, name := range kept.Keys() {
			macro, _ := kept.Get(name)
			for _, used := range used(macro.Body) {
				if meanings.Has(used) && !kept.Has(used) {
					dropped = append(dropped, name)
					break
				}
			}
		}
		if len(dropped) == 0 {
			break
		}
		for _, name := range dropped {
			shapes.Set(name, "math.macro_uses_rejected")
			kept.Delete(name)
		}
	}
	rejected := omap.New[string, string]()
	for _, name := range shapes.Keys() {
		value, _ := shapes.Get(name)
		if reason, ok := value.(string); ok {
			rejected.Set(name, reason)
		}
	}
	return kept, rejected
}

func shape(name string, meanings *omap.Map[string, any]) (Macro, string) {
	value, _ := meanings.Get(name)
	meaning, ok := value.(string)
	if value == nil {
		return Macro{}, "math.macro_not_macro"
	}
	if !ok {
		rec.Fail("no implicit conversion of %T into String", value)
	}
	inner := &meaning
	if match := protectedMeaning.FindStringSubmatch(meaning); match != nil && match[1] == name {
		inner = nil
		if protected, found := meanings.Get(name + " "); found && protected != nil {
			text := rec.AsString(protected)
			inner = &text
		}
	}
	if inner != nil {
		if macro, ok := pairedMacro(name, *inner, meanings); ok {
			return macro, ""
		}
	}
	if inner != nil && operatorMeaning.MatchString(*inner) {
		match := operatorMeaning.FindStringSubmatch(*inner)
		star := ""
		if match[1] == "m" {
			star = "*"
		}
		return Macro{Parameters: 0, Body: "\\operatorname" + star + "{" + match[2] + "}"}, ""
	}
	text := ""
	if inner != nil {
		text = *inner
	}
	return plain(text)
}

func plain(meaning string) (Macro, string) {
	head, body, found := strings.Cut(meaning, "->")
	if !found || !parameterHead.MatchString(head) {
		return Macro{}, "math.macro_unread"
	}
	body = rec.Strip(body)
	wrapped := ensuremath.FindStringSubmatch(body)
	if wrapped != nil && balanced(wrapped[1]) {
		body = rec.Strip(wrapped[1])
	}
	if forbidden(body) || !balanced(body) {
		return Macro{}, "math.macro_complex"
	}
	return Macro{Parameters: int64(strings.Count(head, "#")), Body: body, TextMode: wrapped != nil}, ""
}

func balanced(text string) bool {
	depth := 0
	for _, token := range balanceToken.FindAllString(text, -1) {
		switch token {
		case "{":
			depth++
		case "}":
			depth--
		}
		if depth < 0 {
			return false
		}
	}
	return depth == 0
}

// used is MathMacros.used: the control-sequence names a TeX text uses, first use first.
func used(text string) []string {
	names := []string{}
	for _, match := range usedName.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(names, match[1]) {
			names = append(names, match[1])
		}
	}
	return names
}

// usedBy is MathMacros.used_by: the kept macros a set of TeX texts uses, with the kept macros
// their bodies use, by name.
func usedBy(kept *omap.Map[string, Macro], texts []string) rec.Map {
	wanted := []string{}
	for _, text := range texts {
		for _, name := range used(text) {
			if kept.Has(name) && !slices.Contains(wanted, name) {
				wanted = append(wanted, name)
			}
		}
	}
	for {
		more := []string{}
		for _, name := range wanted {
			macro, _ := kept.Get(name)
			for _, inner := range used(macro.Body) {
				if kept.Has(inner) && !slices.Contains(wanted, inner) && !slices.Contains(more, inner) {
					more = append(more, inner)
				}
			}
		}
		if len(more) == 0 {
			break
		}
		wanted = append(wanted, more...)
	}
	result := rec.Map{}
	for _, name := range wanted {
		macro, _ := kept.Get(name)
		result[name] = macro.Map()
	}
	return result
}
