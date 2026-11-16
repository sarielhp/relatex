package a11ylint

import (
	"regexp"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/rec"
)

// An image drawn by the author's QED symbol is a mark, not a figure: the export names it "End of
// proof" (decision 176, amendment 1), so the lint asks for no description of it. With a conversion
// the marks are the document's own (symbols that show an image); without one, an \includegraphics in
// the body of the QED symbol's definition, or of a macro that body calls, is one.

// qedDefinition is a command definition up to its name: \def, \newcommand and the like.
var qedDefinition = regexp.MustCompile(`\\(?:def|gdef|edef|xdef|newcommand|renewcommand|providecommand|DeclareRobustCommand)\*?\s*\{?\s*\\([A-Za-z@]+)\s*\}?`)

// theoremSymbol is ntheorem's \theoremsymbol{...}: the mark a proof ends with.
var theoremSymbol = regexp.MustCompile(`\\theoremsymbol\s*\{`)

// qedCalledMacro is a command name inside a definition body.
var qedCalledMacro = regexp.MustCompile(`\\([A-Za-z@]+)`)

// qedDepth bounds how many levels of macros a QED symbol is followed through.
const qedDepth = 3

type qedBody struct {
	file       *File
	start, end int
}

// qedBodies are the bodies, in the source files, of the QED symbol's definitions and of the macros
// they call (\def\qedsymbol{\myqedsymbol}, \theoremsymbol{\myqedsymbol}), up to qedDepth levels.
func qedBodies(files []*File) []qedBody {
	names := map[string]bool{"qedsymbol": true}
	for _, file := range files {
		for _, match := range theoremSymbol.FindAllStringIndex(file.Text, -1) {
			if body, _, ok := argument(file.Text, match[1]-1, '{', '}'); ok {
				addCalled(names, body)
			}
		}
	}
	var found []qedBody
	for level := 0; level < qedDepth; level++ {
		found = found[:0]
		before := len(names)
		for _, file := range files {
			for _, match := range qedDefinition.FindAllStringSubmatchIndex(file.Text, -1) {
				name := file.Text[match[2]:match[3]]
				if !names[name] {
					continue
				}
				start, ok := bodyStart(file.Text, match[1])
				if !ok {
					continue
				}
				body, after, _ := argument(file.Text, start, '{', '}')
				found = append(found, qedBody{file, start, after})
				addCalled(names, body)
			}
		}
		if len(names) == before {
			break
		}
	}
	return found
}

// bodyStart is the offset of a definition's body: after its [n] and [default] arguments and the
// #1#2 parameter text of a \def.
func bodyStart(text string, offset int) (int, bool) {
	for range 2 {
		if _, after, ok := argument(text, offset, '[', ']'); ok {
			offset = after
		}
	}
	for offset+1 < len(text) && text[offset] == '#' {
		offset += 2
	}
	_, _, ok := argument(text, offset, '{', '}')
	return offset, ok
}

func addCalled(names map[string]bool, body string) {
	for _, match := range qedCalledMacro.FindAllStringSubmatch(body, -1) {
		names[match[1]] = true
	}
}

// inQED: the offset in file lies in the body of a QED symbol's definition.
func inQED(bodies []qedBody, file string, offset int) bool {
	for _, body := range bodies {
		if body.file.Path == file && offset >= body.start && offset < body.end {
			return true
		}
	}
	return false
}

// withoutQEDMarks drops the "figure has no description" items of images the QED symbol draws.
func withoutQEDMarks(items []Item, paper Paper) []Item {
	bodies := qedBodies(paper.Files)
	if len(bodies) == 0 && len(paper.QEDKeys) == 0 {
		return items
	}
	kept := items[:0:0]
	for _, item := range items {
		if item.Kind == "figure" && (paper.QEDKeys[item.Key] || paper.QEDKeys[figures.SourceName(cleanName(item.Target))] ||
			item.Offset >= 0 && inQED(bodies, item.File, item.Offset)) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// qedImageIDs are the ids of the images the document's QED symbols show.
func qedImageIDs(document rec.Map) map[string]bool {
	ids := map[string]bool{}
	for _, span := range figures.SymbolImages(document) {
		ids[rec.ToS(span["id"])] = true
	}
	return ids
}
