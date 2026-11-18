package frontmatter

// switchHead is a one-argument definition whose whole body is #1 (passes the argument through)
// or empty (drops it): \newcommand{\NotESA}[1]{#1}, \newcommand{\ESA}[1]{}.
var switchHead = rx(`\\(?:re)?newcommand\*?\s*\{?\\([A-Za-z]+)\}?\s*\[1\]\s*\{\s*(#1)?\s*\}`)

// switchMacros are the author's pass-through (true) and dropping (false) one-argument macros,
// the first definition of a name winning as for the other macros (the article build of an
// \ifx switch is conventionally the first branch).
func (t *TexMetadata) switchMacros() map[string]bool {
	found := map[string]bool{}
	for _, match := range switchHead.FindAllStringSubmatch(t.definitions, -1) {
		if _, seen := found[match[1]]; !seen {
			found[match[1]] = match[2] != ""
		}
	}
	return found
}

// withoutSwitches unwraps the pass-through macros (\NotESA{\author{A}} reads as \author{A}) and
// drops the dropping ones with their argument (\ESA{\author{B}{...}} types nothing): the
// front matter is read as TeX prints it, not as the macro hides it.
func (t *TexMetadata) withoutSwitches(text string) string {
	macros := t.switchMacros()
	names := make([]string, 0, len(macros))
	for name := range macros {
		names = append(names, name)
	}
	check := func(text string, end int) (int, bool) {
		if !notLetter(text, end) {
			return 0, false
		}
		return beforeBrace(text, end)
	}
	position := 0
	for len(names) > 0 {
		start, end, name, ok := findCommand(text, position, names, check)
		if !ok {
			return text
		}
		inner, ending, readable := t.argumentAt(text, end)
		if !readable {
			position = end
			continue
		}
		if !macros[name] {
			inner = " "
		}
		text = text[:start] + inner + text[ending:]
		position = start
	}
	return text
}

func (t *TexMetadata) argumentAt(text string, position int) (inner string, ending int, ok bool) {
	defer recoverFlag(&ok)
	inner, ending = t.argument(text, position)
	return inner, ending, true
}
