package a11ylint

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	// beamerClass and frameBegin: a beamer deck labels slides with bold text on purpose.
	beamerClass = regexp.MustCompile(`\\documentclass\s*(?:\[[^\]]*\])?\s*\{beamer\}`)
	frameBegin  = regexp.MustCompile(`\\begin\s*\{frame\*?\}`)
	// fakeHeading matches a whole paragraph that is only bold text, optionally behind \noindent:
	// \textbf{X}, {\bf X} or {\bfseries X}, with an optional final . or :. X may hold one level
	// of braces (\emph{L^p}), no more.
	fakeHeading = regexp.MustCompile(`\A(?:\\noindent\s+)?(?:\\textbf\s*\{((?:[^{}]|\{[^{}]*\})*)\}|\{\s*\\bf(?:series)?\s+((?:[^{}]|\{[^{}]*\})*)\})\s*[.:]?\z`)
	// theoremLabel is a run-in label of the document's own environments, a different style.
	theoremLabel = regexp.MustCompile(`(?i)\A(?:theorem|lemma|proof|proposition|corollary|definition|remark|example|note|claim|fact|observation|exercise|solution|answer|conjecture)(?:\s+[A-Z]?\d+(?:\.\d+)*)?\s*[.:]?\z`)
)

// labelEnvironments are the environments where bold text alone is a label by design.
var labelEnvironments = map[string]bool{
	"figure": true, "table": true, "tabular": true, "array": true, "align": true, "equation": true,
	"itemize": true, "enumerate": true, "description": true, "tcolorbox": true, "verbatim": true,
	"lstlisting": true, "tikzpicture": true, "minipage": true, "algorithm": true, "algorithmic": true,
	"abstract": true, "center": true, "quote": true, "quotation": true, "thebibliography": true,
}

// beamerPaper: one of the source files is a beamer deck (its class, or a slide environment).
func beamerPaper(files []*File) bool {
	for _, file := range files {
		if beamerClass.MatchString(file.Text) || frameBegin.MatchString(file.Text) {
			return true
		}
	}
	return false
}

// paragraph is a run of non-blank lines: where it starts and ends in file.Text.
type paragraph struct {
	start, end int
}

// paragraphs splits a file's text at blank lines (comments already blanked), keeping each
// paragraph's offsets.
func paragraphs(file *File) []paragraph {
	result := []paragraph{}
	start := -1
	for index := 0; index < len(file.lines); index++ {
		begin := file.lines[index]
		end := len(file.Text)
		if index+1 < len(file.lines) {
			end = file.lines[index+1]
		}
		if strings.TrimSpace(file.Text[begin:end]) == "" {
			if start >= 0 {
				result = append(result, paragraph{start, begin})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = begin
		}
	}
	if start >= 0 {
		result = append(result, paragraph{start, len(file.Text)})
	}
	return result
}

// environmentSpans are the ranges of the environments where bold text is a label by design.
func environmentSpans(text string) [][2]int {
	spans := [][2]int{}
	type open struct {
		name  string
		start int
	}
	stack := []open{}
	for _, match := range environmentEdge.FindAllStringSubmatchIndex(text, -1) {
		kind, name := text[match[2]:match[3]], text[match[4]:match[5]]
		switch kind {
		case "begin":
			stack = append(stack, open{name, match[0]})
		case "end":
			for index := len(stack) - 1; index >= 0; index-- {
				if stack[index].name != name {
					continue
				}
				if labelEnvironments[name] {
					spans = append(spans, [2]int{stack[index].start, match[1]})
				}
				stack = stack[:index]
				break
			}
		}
	}
	return spans
}

// headingItems: a paragraph that is only bold text looks like a heading but is not marked as
// one (WCAG 1.3.1, 2.4.6). It is advice: --strict does not count it.
func headingItems(file *File) []Item {
	items := []Item{}
	spans := environmentSpans(file.Text)
	for _, found := range paragraphs(file) {
		if inside(spans, found.start) || insideDefinition(file.Text, found.start) {
			continue
		}
		text := strings.Join(strings.Fields(file.Text[found.start:found.end]), " ")
		if item, ok := headingItem(file, found.start, text); ok && !runInLabel(file, found.end, text) {
			items = append(items, item)
		}
	}
	return items
}

// closingPunctuation ends a bold label that is punctuated like the start of a sentence ("Stage 1:").
var closingPunctuation = regexp.MustCompile(`[.:]\s*\}?\s*\z`)

// runInLabel: the bold text ends in a colon or point and the paragraph goes on in the source ("Stage
// 1:", "Evaluation Metrics."): a run-in label of the paragraph, which \paragraph{…} would spell
// differently (F11). A bold line without that punctuation stays advice.
func runInLabel(file *File, end int, text string) bool {
	return closingPunctuation.MatchString(text) && continuesInSource(file, end)
}

// continuesInSource: the paragraph that ends at end (a run of lines that are blank once comments are
// blanked) goes on in the source: the next line that is not a comment holds text, with no blank
// line between (the sentence follows the label, a commented-out alternative between them).
func continuesInSource(file *File, end int) bool {
	for _, line := range strings.Split(file.Source[end:], "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "%") {
			return trimmed != ""
		}
	}
	return false
}

// headingItem is the item of one paragraph when it is only bold text, false for prose and for
// the forms that are labels by design.
func headingItem(file *File, offset int, text string) (Item, bool) {
	match := fakeHeading.FindStringSubmatch(text)
	if match == nil {
		return Item{}, false
	}
	title := strings.TrimSpace(match[1])
	if title == "" {
		title = strings.TrimSpace(match[2])
	}
	title = trimHeading(title)
	if !headingLike(title) {
		return Item{}, false
	}
	line := file.Line(offset)
	item := Item{Kind: "heading", Advisory: true, File: file.Path, Line: line, Offset: offset,
		Message: fmt.Sprintf("a paragraph of bold text looks like a heading (%q) but is not marked as one", title),
		Fix: fmt.Sprintf(`\paragraph{%s} (run-in), or \subsubsection*{%s}, so the heading is a heading in every output`,
			title, title),
		Suggestion: `\paragraph{` + title + `}`, NoteLine: line, Place: PlaceInstead}
	item.Note = fmt.Sprintf("“%s” is a paragraph of bold text: it looks like a heading but is set as body text, "+
		"which a screen reader cannot navigate to; write %s (run-in), or \\subsubsection*{%s}, so it is a heading "+
		"in every output (WCAG 1.3.1, 2.4.6)", title, item.Suggestion, title)
	return item, true
}

// headingLike: title is a heading the lint can suggest, not prose and not a run-in label.
func headingLike(title string) bool {
	length := utf8.RuneCountInString(title)
	if length < 2 || length > 80 || strings.Contains(title, `\\`) || strings.ContainsAny(title, "&$") ||
		strings.Contains(title, `\item`) || len(strings.Fields(title)) > 8 {
		return false
	}
	return !theoremLabel.MatchString(title)
}

// trimHeading drops the final . or : of a heading label.
func trimHeading(title string) string {
	title = strings.TrimSpace(title)
	if title != "" && (title[len(title)-1] == '.' || title[len(title)-1] == ':') {
		title = strings.TrimSpace(title[:len(title)-1])
	}
	return title
}
