package a11ylint

import (
	"regexp"
	"strings"
)

const (
	captionEmptyMessage    = "the caption has no text, so the figure/table has no visible name"
	captionNumberedMessage = "the caption has no text, so the figure/table is named by its number only"
	captionGenericMessage  = `the caption only repeats what kind of float it is ("Figure 3")`
	captionFix             = `\caption{…} with what the figure shows, in a few words`
	captionNote            = `write what the figure or table shows in a few words, in the caption itself: ` + `\caption{…}`
)

var (
	// captionCommand matches the commands whose argument is a figure's or table's visible name,
	// with their star and short form (\captionof's float type is read after the match).
	captionCommand = regexp.MustCompile(`\\(captionof|subcaption|caption)\s*\*?\s*(?:\[[^\]]*\])?`)
	// captionLabel is a label: a caption holding only one shows nothing.
	captionLabel = regexp.MustCompile(`\\label\s*\{[^{}]*\}`)
	// captionTextCommands keep the text of their argument when their name goes.
	captionTextCommands = map[string]bool{"textbf": true, "emph": true, "textit": true, "text": true}
	// captionSwitches show nothing (size, shape and alignment switches, spacing); any other
	// command may expand to text, so it counts as text.
	captionSwitches = map[string]bool{`\`: true, ",": true, ";": true, "!": true, " ": true,
		"centering": true, "raggedright": true, "raggedleft": true,
		"tiny": true, "scriptsize": true, "footnotesize": true, "small": true, "normalsize": true, "large": true,
		"Large": true, "LARGE": true, "huge": true, "Huge": true, "bfseries": true, "itshape": true, "em": true,
		"normalfont": true, "protect": true, "noindent": true, "hfill": true, "quad": true, "qquad": true,
		"relax": true, "par": true, "newline": true}
	// genericCaptions say only what kind of float the caption belongs to.
	genericCaptions = map[string]bool{"figure": true, "fig": true, "table": true, "tab": true, "plot": true,
		"image": true, "picture": true, "graph": true, "diagram": true, "chart": true, "illustration": true}
)

// captionItems (WCAG 1.1.1, 2.4.6): the caption is the float's visible name, so one that holds no
// text, or only the kind of float ("Figure 3"), names nothing. The text is what is left after the
// commands that show none go; a caption inside a command's definition is a template whose uses are
// the captions, and one holding a macro parameter shows it only at the use. Decision 167: a
// numbered caption still shows its label, so an empty one names the float as "Figure 3" does and
// is advice; only an empty \caption* leaves the float nameless. Neither is an item in a float whose
// figures are described (described tells): the description says what the figure shows.
func captionItems(file *File, described func(offset int) bool) []Item {
	items := []Item{}
	for _, match := range captionCommand.FindAllStringSubmatchIndex(file.Text, -1) {
		if insideDefinition(file.Text, match[0]) {
			continue
		}
		text, ok := captionArgument(file.Text, match)
		if !ok || strings.Contains(text, "#1") {
			continue
		}
		plain := captionText(text)
		if (plain == "" || genericCaption(plain)) && described(match[0]) {
			continue
		}
		if plain == "" && namedByParent(file.Text, match[0]) {
			continue // an empty panel caption: the figure's caption names the panels (decision 167, amendment)
		}
		switch {
		case plain == "" && strings.HasPrefix(strings.TrimSpace(file.Text[match[3]:match[1]]), "*"):
			items = append(items, captionItem(file, match[0], false, captionEmptyMessage))
		case plain == "":
			items = append(items, captionItem(file, match[0], true, captionNumberedMessage))
		case genericCaption(plain):
			items = append(items, captionItem(file, match[0], true, captionGenericMessage))
		}
	}
	return items
}

// captionItem is one caption item at offset; the author writes the caption, so it has no suggestion.
func captionItem(file *File, offset int, advisory bool, message string) Item {
	line := file.Line(offset)
	return Item{Kind: "caption", File: file.Path, Line: line, Message: message, Fix: captionFix,
		Note: captionNote, NoteLine: line, Advisory: advisory}
}

// captionArgument is the text a caption command's argument holds; \captionof's float type and a
// short form are read past.
func captionArgument(text string, match []int) (string, bool) {
	after := match[1]
	if text[match[2]:match[3]] == "captionof" {
		_, end, ok := argument(text, after, '{', '}')
		if !ok {
			return "", false
		}
		after = end
		if _, end, ok = argument(text, after, '[', ']'); ok {
			after = end
		}
	}
	group, _, ok := argument(text, after, '{', '}')
	return group, ok
}

// captionText is the text a caption's argument shows: \label groups, switches and spacing
// (\centering, \small, \\, \,) go, \textbf and its like keep the text of their argument, any
// other command counts as text (a macro may expand to one), then braces, $, ~ and spaces are trimmed.
func captionText(text string) string {
	text = captionLabel.ReplaceAllString(text, "")
	var out strings.Builder
	for index := 0; index < len(text); {
		if text[index] != '\\' {
			out.WriteByte(text[index])
			index++
			continue
		}
		name, after := captionCommandAt(text, index)
		switch {
		case captionTextCommands[name]:
			index = after // its argument's text stands in its place
		case after < len(text) && text[after] == '{':
			out.WriteString(text[index:after])
			index = after
		case captionSwitches[name]:
			index = after // a switch or spacing command shows nothing
		default:
			out.WriteString(text[index:after]) // an unknown macro may expand to text
			index = after
		}
	}
	return strings.Trim(out.String(), "{}~$ \t\r\n")
}

// captionCommandAt reads the command whose backslash is at index: its name and the offset where a
// brace argument would start, past its star and spaces.
func captionCommandAt(text string, index int) (string, int) {
	end := index + 1
	if end < len(text) && !captionLetter(text[end]) {
		return text[end : end+1], end + 1 // a control symbol: \\ and \, show nothing, \% shows %
	}
	for end < len(text) && captionLetter(text[end]) {
		end++
	}
	after := captionSpaces(text, end)
	if after < len(text) && text[after] == '*' {
		after = captionSpaces(text, after+1)
	}
	return text[index+1 : end], after
}

func captionLetter(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char == '@'
}

func captionSpaces(text string, index int) int {
	for index < len(text) && strings.IndexByte(" \t\r\n", text[index]) >= 0 {
		index++
	}
	return index
}

// genericCaption: the text names only the kind of float, with a trailing number and point left
// off: "Figure 3.", "Table 2:" and "Plot" say nothing, "Figure 3. The hulls" says something.
func genericCaption(text string) bool {
	word := strings.TrimRight(strings.TrimSpace(strings.ToLower(text)), ".:")
	word = strings.TrimSpace(strings.TrimRight(word, "0123456789."))
	return genericCaptions[strings.TrimRight(word, ".:")]
}
