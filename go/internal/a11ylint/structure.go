package a11ylint

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// figure is a figure environment: its extent and whether acmart's \Description or relatex's
// \relatexalt describes it.
type figure struct {
	start, end int
	described  bool
	macro      bool // a \relatexalt with an alternative (decision 125, amendment 2), in any class
	drawn      bool // it draws (tikzpicture, pgfpicture) and includes no graphic
	drawing    int  // where its first drawing begins, when drawn
}

type figureList []figure

var (
	environment = regexp.MustCompile(`\\(begin|end)\s*\{([^}]+)\}`)
	description = regexp.MustCompile(`\\Description\b`)
	relatexAlt  = regexp.MustCompile(`\\relatexalt\b`)
	drawing     = regexp.MustCompile(`\\begin\s*\{(tikzpicture|pgfpicture|tikzcd)\}`)
)

// figureNames are the environments whose content is one figure.
var figureNames = map[string]bool{"figure": true, "figure*": true, "wrapfigure": true, "marginfigure": true, "teaserfigure": true}

// figureRanges are the figure environments of text, outermost first.
func figureRanges(text string) figureList {
	result := figureList{}
	type open struct {
		name  string
		start int
	}
	stack := []open{}
	for _, match := range environment.FindAllStringSubmatchIndex(text, -1) {
		kind, name := text[match[2]:match[3]], text[match[4]:match[5]]
		if !figureNames[name] {
			continue
		}
		if kind == "begin" {
			stack = append(stack, open{name, match[0]})
			continue
		}
		for index := len(stack) - 1; index >= 0; index-- {
			if stack[index].name == name {
				body := text[stack[index].start:match[1]]
				found := figure{start: stack[index].start, end: match[1], described: describes(body), macro: macroDescribes(body)}
				if first := drawing.FindStringIndex(body); first != nil && !graphic.MatchString(body) {
					found.drawn, found.drawing = true, found.start+first[0]
				}
				result = append(result, found)
				stack = stack[:index]
				break
			}
		}
	}
	return result
}

// describes: the body holds a \Description with some text.
func describes(body string) bool {
	for _, match := range description.FindAllStringIndex(body, -1) {
		_, after, _ := argument(body, match[1], '[', ']')
		if text, _, ok := argument(body, after, '{', '}'); ok && strings.TrimSpace(text) != "" {
			return true
		}
	}
	return false
}

// macroDescribes: the body holds a \relatexalt whose first argument, the alternative, has some text.
func macroDescribes(body string) bool {
	for _, match := range relatexAlt.FindAllStringIndex(body, -1) {
		text, _, ok := argument(body, match[1], '{', '}')
		text = strings.TrimSpace(text)
		if ok && text != "" && !strings.HasPrefix(text, TodoPrefix) && !strings.HasPrefix(text, DraftPrefix) {
			return true
		}
	}
	return false
}

// at is the outermost figure holding offset, nil outside every figure.
func (f figureList) at(offset int) *figure {
	var found *figure
	for index := range f {
		if f[index].start <= offset && offset < f[index].end && (found == nil || f[index].start < found.start) {
			found = &f[index]
		}
	}
	return found
}

// drawnItems: without a last conversion to name them, figures that draw (TikZ) are reported from
// the source, at their drawing. relatex's export and the tagged PDF read the tagging project's
// alt, actualtext and artifact keys on the tikzpicture (decision 111; pgf 3.1.12 in TeX Live
// accepts them and ignores them without tagging), else the sidecar; a picture with one of them
// needs nothing more.
func drawnItems(file *File) []Item {
	items := []Item{}
	for _, figure := range figureRanges(file.Text) {
		if !figure.drawn || figure.macro {
			continue
		}
		item := Item{Kind: "drawing", File: file.Path, Line: file.Line(figure.drawing),
			Message: "a figure drawn in the source (TikZ) has no description",
			Fix: `alt={…} on its tikzpicture, \relatexalt{…}{…} in its figure, or after a conversion its entry in ` + figdesc.FileName +
				" (relatex alt review lists its name)"}
		if alt, keyed := pictureKeyed(file.Text, figure.drawing); keyed {
			item.Target = "drawn at line " + fmt.Sprint(item.Line)
			items = append(items, placeholderItems(item, alt)...)
			continue
		}
		items = append(items, drawingNote(item, file.Text, figure.drawing))
	}
	return items
}

// pictureKeyed: the tikzpicture at start has the tagging project's alt, actualtext or artifact
// key, with its alt (a placeholder of the annotated copy still needs the author).
func pictureKeyed(text string, start int) (alt string, keyed bool) {
	match := drawing.FindStringSubmatchIndex(text[start:])
	if match == nil || text[start+match[2]:start+match[3]] != "tikzpicture" {
		return "", false
	}
	options, _, has := argument(text, start+match[1], '[', ']')
	keys := keyValues(strings.TrimSpace(options))
	alt, given := keys["alt"]
	_, actual := keys["actualtext"]
	_, artifact := keys["artifact"]
	return alt, has && (given || actual || artifact)
}

// drawingNote: for a tikzpicture the alt key the export and the tagged PDF read, as a ready-to-fill
// \begin line written (commented out) under the drawing's in the copy; else the sidecar entry.
func drawingNote(item Item, text string, start int) Item {
	sidecar := "after a conversion relatex alt review names it (figs/fig-N) and relatex alt draft drafts its " +
		"[figs.fig-N] alt = \"…\" in " + figdesc.FileName
	match := drawing.FindStringSubmatchIndex(text[start:])
	item.NoteLine = item.Line
	if match == nil || text[start+match[2]:start+match[3]] != "tikzpicture" {
		item.Note = sidecar
		return item
	}
	options, _, _ := argument(text, start+match[1], '[', ']')
	options = strings.TrimSpace(options)
	if options != "" {
		options += ", "
	}
	item.Suggestion = `\begin{tikzpicture}[` + options + `alt={…}]`
	item.Place = PlaceInstead
	item.Note = "describe it on the picture: " + item.Suggestion +
		" (the tagging project's key, which relatex and the tagged PDF read); or " + sidecar
	return item
}

var href = regexp.MustCompile(`\\href\s*\{[^}]*\}\s*\{([^{}]*)\}`)

// vagueLinks are link texts that do not say where the link goes.
var vagueLinks = map[string]bool{"here": true, "click here": true, "this": true, "link": true, "this link": true,
	"more": true, "read more": true, "website": true, "this page": true}

var (
	// namingPhrase is a sentence's words that say what an address is: "The code is available at", "the
	// public code repository", "the project page".
	namingPhrase = regexp.MustCompile(`(?i)\bavailable\s+(?:at|on|from|here)\b|\b(?:code(?:base)?|source|repository|repo|` +
		`implementation|dataset|webpage|web\s+page|website|homepage|archive|project|demo|software|package|library|tool)\b`)
	// footnoteOpen ends the text before a link that is the whole content of a footnote.
	footnoteOpen = regexp.MustCompile(`\\footnote(?:text)?\s*(?:\[[^\]]*\])?\s*\{\s*\z`)
	footnoteEnd  = regexp.MustCompile(`\A\s*\}`)
)

// namingWindow bounds how far before a link the sentence that introduces it is read.
const namingWindow = 160

// introducedAddress: a bare address as link text is named by its sentence ("The code is available at:
// \href{…}{…}", "the public code repository\footnote{\href{…}{…}}"), so the text need not name the
// target; a link with no such introduction keeps the item. The sentence is read up to namingWindow
// bytes before the link.
func introducedAddress(text string, start, end int) bool {
	before := text[max(sentenceStart(text, start), start-namingWindow):start]
	if footnoteOpen.MatchString(before) && footnoteEnd.MatchString(text[end:]) {
		return true
	}
	return namingPhrase.MatchString(before)
}

// linkItems: a link's text says where it goes, not "here" or a bare address.
func linkItems(file *File) []Item {
	items := []Item{}
	for _, match := range href.FindAllStringSubmatchIndex(file.Text, -1) {
		text := strings.ToLower(strings.Trim(strings.TrimSpace(file.Text[match[2]:match[3]]), ".:"))
		bare := strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "www.")
		if !vagueLinks[text] && !bare {
			continue
		}
		if bare && !vagueLinks[text] && introducedAddress(file.Text, match[0], match[1]) {
			continue
		}
		items = append(items, Item{Kind: "link", File: file.Path, Line: file.Line(match[0]),
			Message: fmt.Sprintf("the link text %q does not say where the link goes", text),
			Fix:     "a text naming the target (\\href{…}{the dataset on Zenodo})"})
	}
	return items
}

var (
	inlineMath   = regexp.MustCompile(`(?s)\$[^$]+\$|\\\((.*?)\\\)|\\\[(.*?)\\\]`)
	normBars     = regexp.MustCompile(`\\\||\\[lr]Vert\b|\\Vert\b`)
	absoluteBars = regexp.MustCompile(`(?:^|[^\\])\||\\[lr]vert\b|\\vert\b`)
	semantic     = regexp.MustCompile(`\\(?:newcommand|renewcommand|DeclareMathOperator|DeclarePairedDelimiter\w*|def)\s*\{?\\(abs|card|norm)\b`)
)

// mathItems (opt-in): one document-level item, never one per formula (the author, 2026-10-30): how
// many bars read as |x| or ‖x‖ without a macro saying what they mean. The fix is a macro per use, not
// one meaning for the document: a paper's bars often mean several things (|S| a set's size, |x| an
// absolute value, |A| a determinant), so a document-wide meaning would make some readings
// confidently wrong (decision 114, item 5).
func mathItems(files []*File) []Item {
	norms, absolutes := 0, 0
	declared := false
	for _, file := range files {
		declared = declared || semantic.MatchString(file.Text)
		for _, formula := range inlineMath.FindAllString(file.Text, -1) {
			norms += len(normBars.FindAllString(formula, -1))
			absolutes += len(absoluteBars.FindAllString(formula, -1))
		}
	}
	if norms+absolutes == 0 || len(files) == 0 {
		return nil
	}
	fix := `write each use with a macro that says what its bars mean, e.g. \DeclarePairedDelimiter\abs{\lvert}{\rvert}, ` +
		`\DeclarePairedDelimiter\card{\lvert}{\rvert} and \DeclarePairedDelimiter\norm{\lVert}{\rVert} (mathtools) for ` +
		`\abs{x}, \card{S} and \norm{v}, and \det A for a determinant; unmarked bars stay valid and are read as symbols`
	if declared {
		fix = `the preamble defines \abs, \card or \norm; use them where the bars mean absolute value, cardinality or ` +
			`norm, and \det A for a determinant`
	}
	main := files[0]
	line := 1
	if match := beginDocument.FindStringIndex(main.Text); match != nil {
		line = main.Line(match[0])
	}
	return []Item{{Kind: "math", File: main.Path, Line: line,
		Message: fmt.Sprintf("%d vertical bars and %d double bars in formulas carry no meaning a reader can announce", absolutes, norms),
		Fix:     fix}}
}
