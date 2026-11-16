package frontmatter

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Two author forms a paper's own class or package prints (decision 091): spconf's paired
// \twoauthors{name}{address}{name}{address}, and an \author the class typesets where it
// stands (WileyMSP-template: \author{Name} after \maketitle). Both are read only when the
// paper's own style files say so; generic LaTeX prints neither.

var (
	// pairedAuthors: \twoauthors, \threeauthors, \fourauthors and their author count.
	pairedAuthors = map[string]int{"twoauthors": 2, "threeauthors": 3, "fourauthors": 4}
	// pairedDefinition: \def\twoauthors#1#2#3#4{ (its parameter text).
	pairedDefinition = regexp.MustCompile(`\\[gex]?def\s*\\(twoauthors|threeauthors|fourauthors)((?:#\d)*)\s*\{`)
	pairedCommand    = regexp.MustCompile(`\\(?:re)?newcommand\*?\s*\{?\\(twoauthors|threeauthors|fourauthors)\}?\s*\[(\d)\]`)
	// authorDefinitions: the forms a style redefines \author with, up to its body's brace.
	authorDefinitions = []*regexp.Regexp{
		regexp.MustCompile(`\\renewenvironment\*?\s*\{author\}\s*\{`),
		regexp.MustCompile(`\\(?:re)?newcommand\*?\s*(?:\{\\author\}|\\author)\s*(?:\[\d\]\s*)?(?:\[[^\]]*\]\s*)?\{`),
		regexp.MustCompile(`\\[gex]?def\s*\\author((?:#\d)*)\s*\{`),
	}
	// frontEnd: where an in-place author list can no longer stand.
	frontEnd = regexp.MustCompile(`\\begin\{abstract\}|\\section\*?\s*[\[{]`)
)

// WithStyles gives the reader the text of the paper's own class and package files, which
// decide the class author forms. It returns the reader.
func (t *TexMetadata) WithStyles(styles string) *TexMetadata {
	t.styles = withoutComments(styles)
	return t
}

// classAuthors: the authors a class form declares when no \author or \name is in effect at
// \maketitle; nil when there is none.
func (t *TexMetadata) classAuthors(head string) []any {
	if authors := t.spconfAuthors(head); len(authors) > 0 {
		return authors
	}
	return t.inPlaceAuthors()
}

// spconfAuthors reads the last paired-author command the paper's styles define with one
// name and one address argument per author.
func (t *TexMetadata) spconfAuthors(head string) []any {
	names := []string{}
	for name, count := range pairedAuthors {
		if t.pairedArity(name) == 2*count {
			names = append(names, name)
		}
	}
	rows := t.commands(head, names)
	if len(rows) == 0 {
		return nil
	}
	row := rows[len(rows)-1]
	arguments := []string{row.argument}
	ending := row.end
	for len(arguments) < 2*pairedAuthors[row.name] {
		var value string
		value, ending = t.argument(head, ending)
		arguments = append(arguments, value)
	}
	result := []any{}
	for index := 0; index < len(arguments); index += 2 {
		result = append(result, t.addressedAuthor(arguments[index], arguments[index+1]))
	}
	return result
}

// pairedArity is how many arguments the styles' definition of the command takes (0 when
// they do not define it).
func (t *TexMetadata) pairedArity(name string) int {
	arity := 0
	for _, match := range pairedDefinition.FindAllStringSubmatch(t.styles, -1) {
		if match[1] == name {
			arity = len(match[2]) / 2
		}
	}
	for _, match := range pairedCommand.FindAllStringSubmatch(t.styles, -1) {
		if match[1] == name {
			arity = int(match[2][0] - '0')
		}
	}
	return arity
}

// addressedAuthor: one name and its own address block; a line of the block that is an
// address alone is the author's email.
func (t *TexMetadata) addressedAuthor(name, address string) rec.Map {
	affiliations, emails := []any{}, []any{}
	for _, line := range topLevelSplit(address, lineSeparator) {
		text := t.plain(line, false)
		switch {
		case text == "":
		case emailLine.MatchString(text):
			emails = append(emails, text)
		default:
			affiliations = append(affiliations, text)
		}
	}
	return rec.Map{"name": authorMark.ReplaceAllLiteralString(t.plain(name, false), ""),
		"affiliations": affiliations, "emails": emails}
}

// inPlaceAuthors: the \author declarations after \maketitle, before the abstract or first
// section, when the paper's class typesets \author where it stands.
func (t *TexMetadata) inPlaceAuthors() []any {
	if !t.authorPrintsInPlace() {
		return nil
	}
	_, after, found := strings.Cut(t.clean, `\maketitle`)
	if !found {
		return nil
	}
	if location := frontEnd.FindStringIndex(after); location != nil {
		after = after[:location[0]]
	}
	if len(t.commands(after, []string{"author"})) == 0 {
		return nil
	}
	return t.genericAuthors(after, t.genericNames(after))
}

// authorPrintsInPlace: the styles' last definition of \author typesets its argument instead
// of keeping it in \@author for \maketitle.
func (t *TexMetadata) authorPrintsInPlace() bool {
	last, body := -1, ""
	for _, pattern := range authorDefinitions {
		for _, location := range pattern.FindAllStringIndex(t.styles, -1) {
			if location[0] <= last {
				continue
			}
			if value, ok := t.tryArgument(t.styles, location[1]-1); ok {
				last, body = location[0], value
			}
		}
	}
	return last >= 0 && !strings.Contains(body, `\@author`)
}
