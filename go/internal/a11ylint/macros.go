package a11ylint

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The author's own commands matter to the lint twice (decision 115): a command whose body inputs a
// file named by its arguments (a book's \InputChap{36_frechet}{frechet.tex}, which \input{#1/#2})
// is followed like \input; a command that passes its optional argument on to \includegraphics's
// options (\IncludeGraphics[#1]{#2}) takes the alt key there, so the annotated copy writes the
// placeholder into its call. Both are read from \newcommand, \renewcommand, \providecommand and
// \DeclareRobustCommand definitions, with their arguments substituted; TeX itself (the conversion's
// record of each graphic's place) stays the authority for anything a definition hides.

// macro is an author's command: its number of arguments, whether the first is optional, and its body.
type macro struct {
	arguments int
	optional  bool
	body      string
}

var (
	definition = regexp.MustCompile(`\\(?:newcommand|renewcommand|providecommand|DeclareRobustCommand)\*?\s*` +
		`(?:\{\s*\\([A-Za-z@]+)\s*\}|\\([A-Za-z@]+))\s*(?:\[\s*(\d)\s*\])?`)
	inputPath = regexp.MustCompile(`\\def\s*\\input@path\s*\{((?:\s*\{[^{}]*\})+)\s*\}`)
	parameter = regexp.MustCompile(`#([1-9])`)
)

// macros are the commands files define, by name; a later definition replaces an earlier one.
func macros(files []*File) map[string]macro {
	result := map[string]macro{}
	for _, file := range files {
		addMacros(result, file.Text)
	}
	return result
}

// addMacros adds the definitions of text to defined.
func addMacros(defined map[string]macro, text string) {
	for _, match := range definition.FindAllStringSubmatchIndex(text, -1) {
		name := submatch(text, match, 1) + submatch(text, match, 2)
		count, _ := strconv.Atoi(submatch(text, match, 3))
		_, after, optional := argument(text, match[1], '[', ']')
		body, _, ok := argument(text, after, '{', '}')
		if ok && name != "" {
			defined[name] = macro{arguments: count, optional: optional && count > 0, body: body}
		}
	}
}

func submatch(text string, match []int, group int) string {
	if match[2*group] < 0 {
		return ""
	}
	return text[match[2*group]:match[2*group+1]]
}

// call is one use of an author's command in a text: where it starts and ends, and its arguments
// (the optional one first when the command has one; "" when the call leaves it out).
type call struct {
	start, end int
	arguments  []string
	optional   bool // the call gives the optional argument
}

// calls are the uses of command name in text whose arguments are all there.
func calls(text, name string, definition macro) []call {
	found := []call{}
	needle := `\` + name
	for offset := 0; ; {
		index := strings.Index(text[offset:], needle)
		if index < 0 {
			return found
		}
		start := offset + index
		offset = start + len(needle)
		if offset < len(text) && isLetter(text[offset]) {
			continue // a longer command name
		}
		if use, ok := readCall(text, start, len(needle), definition); ok {
			found = append(found, use)
		}
	}
}

// isLetter: a byte that continues a command name.
func isLetter(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char == '@'
}

// readCall reads the arguments of the call at start (the command name is size bytes long).
func readCall(text string, start, size int, definition macro) (call, bool) {
	use := call{start: start, end: start + size}
	mandatory := definition.arguments
	if definition.optional {
		value, after, given := argument(text, use.end, '[', ']')
		use.arguments, use.optional, use.end = append(use.arguments, value), given, after
		mandatory--
	}
	for range mandatory {
		value, after, ok := argument(text, use.end, '{', '}')
		if !ok {
			return call{}, false
		}
		use.arguments, use.end = append(use.arguments, value), after
	}
	return use, true
}

// substitute is template with #1, #2, … replaced by the call's arguments.
func substitute(template string, arguments []string) string {
	return parameter.ReplaceAllStringFunc(template, func(found string) string {
		index := int(found[1] - '1')
		if index < len(arguments) {
			return arguments[index]
		}
		return found
	})
}

// inputTemplates are the file names (with #n for its arguments) the body of command name inputs, also
// through other commands that input files, up to depth; memo keeps each command's once found.
func inputTemplates(name string, defined map[string]macro, memo map[string][]string, depth int) []string {
	if templates, known := memo[name]; known {
		return templates
	}
	memo[name] = nil // a command that calls itself inputs nothing more through itself
	definition := defined[name]
	templates := []string{}
	for _, match := range inputCommand.FindAllStringSubmatch(definition.body, -1) {
		if file := inputName(match[1], match[2]); strings.Contains(file, "#") {
			templates = append(templates, file)
		}
	}
	for _, inner := range innerCommands(definition.body, defined, name) {
		if depth <= 0 {
			break
		}
		innerTemplates := inputTemplates(inner, defined, memo, depth-1)
		for _, use := range calls(definition.body, inner, defined[inner]) {
			for _, template := range innerTemplates {
				templates = append(templates, substitute(template, use.arguments))
			}
		}
	}
	memo[name] = templates
	return templates
}

var commandName = regexp.MustCompile(`\\([A-Za-z@]+)`)

// innerCommands are the defined commands body calls, each once, other than itself.
func innerCommands(body string, defined map[string]macro, itself string) []string {
	names, seen := []string{}, map[string]bool{}
	for _, match := range commandName.FindAllStringSubmatch(body, -1) {
		if _, known := defined[match[1]]; known && match[1] != itself && !seen[match[1]] {
			names, seen[match[1]] = append(names, match[1]), true
		}
	}
	return names
}

// inputPaths are the directories \def\input@path names, where TeX also looks for an input.
func inputPaths(text string) []string {
	paths := []string{}
	for _, match := range inputPath.FindAllStringSubmatch(text, -1) {
		for _, part := range strings.Split(match[1], "}") {
			if path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "{")); path != "" {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

// graphicCommands are \includegraphics and the author's commands that pass their optional argument
// on to its options (directly or through another such command), as one pattern for a call's start.
func graphicCommands(defined map[string]macro) *regexp.Regexp {
	names := []string{"includegraphics"}
	for changed := true; changed; {
		changed = false
		forwarding := regexp.MustCompile(commandsPattern(names) + `\*?\s*\[[^\]]*#1[^\]]*\]`)
		for name, definition := range defined {
			if !definition.optional || !strings.Contains(definition.body, "#1") || slices.Contains(names, name) ||
				!forwarding.MatchString(definition.body) {
				continue
			}
			names, changed = append(names, name), true
		}
	}
	return regexp.MustCompile(commandsPattern(names) + `\b\s*\*?`)
}

// commandsPattern matches a call of one of names, without what follows the name.
func commandsPattern(names []string) string {
	quoted := make([]string, len(names))
	for index, name := range names {
		quoted[index] = regexp.QuoteMeta(name)
	}
	slices.Sort(quoted)
	return `\\(?:` + strings.Join(quoted, "|") + `)`
}
