package wcag

import (
	"regexp"
	"strings"
)

// colourWord is a basic colour named in prose (WCAG 1.4.1, use of colour). Black and white are left
// out: "black box", "white noise" and "black-and-white" name no legend.
var colourWord = regexp.MustCompile(`(?i)\b(red|green|blue|yellow|orange|purple|violet|magenta|cyan|pink|brown|gr[ae]y|teal|olive)(?:dish)?\b`)

// colourCommand is a LaTeX command whose argument is a colour, not prose (\textcolor{red}, \color{blue}).
var colourCommand = regexp.MustCompile(`\\(?:textcolor|color|colorbox|fcolorbox|cellcolor|rowcolor|rowcolors|definecolor|pagecolor)\*?\s*(?:\[[^\]]*\])?\s*(?:\{[^{}]*\}\s*){1,3}`)

// ColourNames are the colours a text names, lower case and once each, in order; the arguments of
// colour commands do not count (WCAG 1.4.1: a caption or sentence that tells the reader to look for
// a colour, "the red line", "shown in blue").
func ColourNames(text string) []string {
	text = colourCommand.ReplaceAllString(text, " ")
	names := []string{}
	seen := map[string]bool{}
	for _, match := range colourWord.FindAllStringSubmatchIndex(text, -1) {
		if notAColour(text, match[2], match[1]) {
			continue
		}
		name := strings.ToLower(text[match[2]:match[3]])
		if name == "grey" {
			name = "gray"
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// thingNouns follow a colour word in an idiom that names a thing, not a colour to look for: the red
// carpet, a red herring (WCAG 1.4.1 asks about colours that tell data apart).
var thingNouns = []string{" carpet", " herring"}

func namesAThing(after string) bool {
	for _, noun := range thingNouns {
		if strings.HasPrefix(after, noun) {
			return true
		}
	}
	return false
}

// notAColour: a colour word that names something else: a red-black tree, Green's function, a Gray
// code (names of people and structures, common in papers), the red carpet.
func notAColour(text string, start, end int) bool {
	after := strings.ToLower(text[end:min(end+9, len(text))])
	word := text[start:end]
	capital := word[0] >= 'A' && word[0] <= 'Z'
	return strings.HasPrefix(after, "-black") || namesAThing(after) || (capital && (strings.HasPrefix(after, "'s") || strings.HasPrefix(after, " code")))
}
