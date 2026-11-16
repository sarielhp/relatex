package a11ylint

import (
	"regexp"
	"strings"
)

// A graphic in the body of a \newcommand that nothing calls is a template, not a place (F1, decision 167
// amendment B3): the export shows no figure for it. A command that is called keeps its item at the
// definition, where alt={…} describes every use.

// definitionStart ends the text before the name of a command being defined.
var definitionStart = regexp.MustCompile(`\\(?:newcommand|renewcommand|providecommand|DeclareRobustCommand|[gex]?def)\*?\s*\{?\s*\z`)

// templateName is the command of the \newcommand family whose body holds offset at.
func templateName(text string, at int) (string, bool) {
	for _, match := range definition.FindAllStringSubmatchIndex(text[:at], -1) {
		_, after, _ := argument(text, match[1], '[', ']')
		if _, end, ok := argument(text, after, '{', '}'); ok && end > at {
			return submatch(text, match, 1) + submatch(text, match, 2), true
		}
	}
	return "", false
}

// firstUse is where the first use of the command name stands among files: its file, the offset of the
// call; false when no file uses it. A mention that defines the command is no use.
func firstUse(files []*File, name string) (*File, int, bool) {
	needle := `\` + name
	for _, file := range files {
		for from := 0; ; {
			index := strings.Index(file.Text[from:], needle)
			if index < 0 {
				break
			}
			at := from + index
			from = at + len(needle)
			if from < len(file.Text) && isLetter(file.Text[from]) {
				continue // a longer command name
			}
			if definitionStart.MatchString(file.Text[max(0, at-40):at]) {
				continue // the definition's own name
			}
			return file, at, true
		}
	}
	return nil, 0, false
}

// unusedTemplate: the graphic at offset at is in the body of a command nothing calls: no figure the
// export shows, so no item (a used command keeps its item at the definition, where alt={…} is the fix
// for every use). A \def's or an environment's body is not followed and keeps the item.
func unusedTemplate(file *File, at int, files []*File) bool {
	name, inside := templateName(file.Text, at)
	if !inside {
		return false
	}
	_, _, used := firstUse(files, name)
	return !used
}
