package a11ylint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/wcag"
)

var (
	caption = regexp.MustCompile(`\\caption\s*\*?\s*(?:\[[^\]]*\])?`)
	// colourPhrase is prose that points the reader to a colour: "shown in red", "the blue line".
	colourPhrase = regexp.MustCompile(`(?i)\b(?:(?:shown|marked|drawn|plotted|highlighted|coloured|colored|depicted|indicated|printed)\s+in|in)\s+` +
		`(red|green|blue|yellow|orange|purple|violet|magenta|cyan|pink|brown|gr[ae]y)\b|\b(red|green|blue|yellow|orange|purple|violet|magenta|cyan|pink|brown|gr[ae]y)\s+` +
		`(?:lines?|curves?|dots?|points?|bars?|regions?|areas?|boxes|box|edges?|nodes?|arrows?|cells?|markers?|circles?|squares?|crosses|triangles?|text|entries|vertices|vertex|shading|background)\b`)
)

const colourFix = "say it with more than colour, e.g. shown as dashed red line, red circles and blue squares: " +
	"a word, a marker or a line style (dashed, solid) in the figure and its caption (WCAG 1.4.1)"

// colourItems (WCAG 1.4.1, use of colour): a caption that names colours, or prose that sends the
// reader to a colour ("the red line", "shown in blue"), may rely on colour alone. Whether a word,
// marker or pattern also says it only the author knows: one item per caption and per sentence.
func colourItems(file *File, defined map[string]macro) []Item {
	items := []Item{}
	captions := [][2]int{}
	for _, match := range caption.FindAllStringIndex(file.Text, -1) {
		text, end, ok := argument(file.Text, match[1], '{', '}')
		if !ok {
			continue
		}
		captions = append(captions, [2]int{match[0], end})
		if names := wcag.ColourNames(withoutCode(text)); len(names) > 0 {
			items = append(items, Item{Kind: "colour", File: file.Path, Line: file.Line(match[0]),
				Message: fmt.Sprintf("the caption names colours (%s); colour may be the only thing that tells them apart",
					strings.Join(names, ", ")),
				Fix: colourFix})
		}
	}
	// A figure's own description may name its colours: that is what it is for; so may an argument the
	// command never typesets (decision 167).
	described := append(append(captions, descriptions(file.Text)...), discardedArguments(file.Text, defined)...)
	described = append(described, inlineCode(file.Text)...)
	return append(items, proseColourItems(file, described)...)
}

var (
	// codeCommand opens a string typed as code: \texttt{…}, \verb|…|, \lstinline|…|.
	codeCommand = regexp.MustCompile(`\\(?:texttt|verb\*?|lstinline(?:\[[^\]]*\])?)\s*`)
)

// inlineCode are the ranges of strings typed as code (a quoted prompt, a file name): a colour word in
// one names a string, not something drawn (F4).
func inlineCode(text string) [][2]int {
	spans := [][2]int{}
	for _, match := range codeCommand.FindAllStringIndex(text, -1) {
		if end := codeEnd(text, match); end > match[1] {
			spans = append(spans, [2]int{match[0], end})
		}
	}
	return spans
}

// codeEnd is where the code that starts after the command at match ends: its braced argument, or the
// text up to the delimiter \verb opened with; match[1] when there is none.
func codeEnd(text string, match []int) int {
	if match[1] >= len(text) {
		return match[1]
	}
	if text[match[1]] == '{' {
		if _, end, ok := argument(text, match[1], '{', '}'); ok {
			return end
		}
		return match[1]
	}
	delimiter := text[match[1]]
	if strings.HasPrefix(text[match[0]:], `\texttt`) || delimiter == '\\' {
		return match[1]
	}
	if stop := strings.IndexByte(text[match[1]+1:], delimiter); stop >= 0 {
		return match[1] + 1 + stop + 1
	}
	return match[1]
}

// withoutCode is text with the strings typed as code blanked out.
func withoutCode(text string) string {
	spans := inlineCode(text)
	if len(spans) == 0 {
		return text
	}
	out := []byte(text)
	for _, span := range spans {
		for index := span[0]; index < span[1]; index++ {
			out[index] = ' '
		}
	}
	return string(out)
}

// colourSentence is the prose colour phrases of one sentence: where the first is, their texts, and
// whether one sends the reader to something drawn ("shown in red", "in blue").
type colourSentence struct {
	offset  int
	phrases []string
	visual  bool
}

// proseColourItems are the prose phrases outside described (captions, descriptions), one item per
// sentence (decision 167). A phrase that only names things by a colour ("the red points", the sets
// of a red-blue problem) in a paragraph that refers to no figure is advice: the colour may be a
// name, not something the reader must see.
func proseColourItems(file *File, described [][2]int) []Item {
	sentences := []*colourSentence{}
	bySentence := map[int]*colourSentence{}
	for _, match := range colourPhrase.FindAllStringSubmatchIndex(file.Text, -1) {
		if inside(described, match[0]) || len(wcag.ColourNames(file.Text[match[0]:match[1]])) == 0 {
			continue
		}
		start := sentenceStart(file.Text, match[0])
		sentence := bySentence[start]
		if sentence == nil {
			sentence = &colourSentence{offset: match[0]}
			bySentence[start] = sentence
			sentences = append(sentences, sentence)
		}
		if phrase := fmt.Sprintf("%q", strings.Join(strings.Fields(file.Text[match[0]:match[1]]), " ")); !slices.Contains(sentence.phrases, phrase) {
			sentence.phrases = append(sentence.phrases, phrase)
		}
		sentence.visual = sentence.visual || match[2] >= 0
	}
	items := []Item{}
	for _, sentence := range sentences {
		message := "the text points to a colour (" + sentence.phrases[0] + ")"
		if len(sentence.phrases) > 1 {
			message = "the text points to colours (" + strings.Join(sentence.phrases, ", ") + ")"
		}
		items = append(items, Item{Kind: "colour", File: file.Path, Line: file.Line(sentence.offset), Message: message,
			Fix: colourFix, Advisory: !sentence.visual && !figureReference.MatchString(paragraphAround(file.Text, sentence.offset))})
	}
	return items
}

var (
	// sentenceEnd is the end of a sentence or a paragraph break.
	sentenceEnd = regexp.MustCompile(`[.!?]\s|\n[ \t]*\n`)
	// paragraphBreak is a blank line.
	paragraphBreak = regexp.MustCompile(`\n[ \t]*\n`)
	// figureReference is prose or markup that sends the reader to a figure.
	figureReference = regexp.MustCompile(`(?i)\bfig(?:ure|s?\.)|\bpictures?\b|\bdiagrams?\b|\bplots?\b|\bdrawings?\b|` +
		`\billustrat|\bdepict|\bshown\b|\\(?:fig|auto|c|v)?ref\s*\{[^}]*fig|\\(?:figref|Figref|includegraphics)\b`)
)

// sentenceStart is where the sentence holding offset starts: after the last sentence end before it.
func sentenceStart(text string, offset int) int {
	start := 0
	for _, match := range sentenceEnd.FindAllStringIndex(text[:offset], -1) {
		start = match[1]
	}
	return start
}

// paragraphAround is the paragraph (between blank lines) that holds offset.
func paragraphAround(text string, offset int) string {
	start, end := 0, len(text)
	for _, match := range paragraphBreak.FindAllStringIndex(text[:offset], -1) {
		start = match[1]
	}
	if match := paragraphBreak.FindStringIndex(text[offset:]); match != nil {
		end = offset + match[0]
	}
	return text[start:end]
}

var describing = regexp.MustCompile(`\\includegraphics\s*\*?|\\Description\s*(?:\[[^\]]*\])?`)

// descriptions are the ranges of figure descriptions: \includegraphics' options (alt, actualtext)
// and acmart's \Description.
func descriptions(text string) [][2]int {
	ranges := [][2]int{}
	for _, match := range describing.FindAllStringIndex(text, -1) {
		open, closing := byte('['), byte(']')
		if strings.HasPrefix(text[match[0]:], `\Description`) {
			open, closing = '{', '}'
		}
		if _, end, ok := argument(text, match[1], open, closing); ok {
			ranges = append(ranges, [2]int{match[0], end})
		}
	}
	return ranges
}

func inside(ranges [][2]int, offset int) bool {
	for _, span := range ranges {
		if offset >= span[0] && offset < span[1] {
			return true
		}
	}
	return false
}
