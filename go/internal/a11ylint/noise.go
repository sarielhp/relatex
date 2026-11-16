package a11ylint

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/figures"
)

// Decision 167: what the lint leaves out because it is no gap of its own, or reports once.

// floatDescriber tells whether the figure environment holding an offset of file is described: it
// has a \Description or \relatexalt, or each figure it shows (its \includegraphics, and the
// places the last conversion recorded inside it) has a description from its keys, the author's or
// a reviewed sidecar entry, or the conversion; and it shows at least one.
func floatDescriber(file *File, paper Paper) func(offset int) bool {
	ranges := figureRanges(file.Text)
	return func(offset int) bool {
		float := ranges.at(offset)
		if float == nil {
			return false
		}
		if float.described && isACM(file.Text) || float.macro {
			return true
		}
		shown, all := 0, true
		for _, match := range graphic.FindAllStringIndex(file.Text[float.start:float.end], -1) {
			shown++
			all = all && graphicDescribed(file.Text, float.start+match[1], paper.Sidecar)
		}
		first, last := file.Line(float.start), file.Line(max(float.start, float.end-1))
		for _, place := range conversionPlaces(paper) {
			if place.place.File == file.Path && place.place.Line >= first && place.place.Line <= last {
				shown++
				all = all && (place.place.Described || entryDescribes(paper.Sidecar, place.key))
			}
		}
		return shown > 0 && all
	}
}

// keyedPlace is a place the last conversion recorded for the figure the sidecar names key.
type keyedPlace struct {
	key   string
	place figdesc.Place
}

// conversionPlaces are the places of the last conversion's included figures and drawings.
func conversionPlaces(paper Paper) []keyedPlace {
	places := []keyedPlace{}
	for _, figure := range paper.Included {
		for _, place := range figure.Places {
			places = append(places, keyedPlace{figure.Key, place})
		}
	}
	for _, drawing := range paper.Drawings {
		for _, place := range drawing.Places {
			places = append(places, keyedPlace{drawing.Key, place})
		}
	}
	return places
}

// graphicDescribed: the \includegraphics whose options start at after has an alt (not a
// placeholder), actualtext or artifact key, or a sidecar entry by its file describes it.
func graphicDescribed(text string, after int, sidecar *figdesc.Sidecar) bool {
	options, next, _ := argument(text, after, '[', ']')
	keys := keyValues(options)
	if alt, has := keys["alt"]; has {
		return len(placeholderItems(Item{}, alt)) == 0
	}
	if keys["actualtext"] != "" || hasArtifact(keys) {
		return true
	}
	name, _, ok := argument(text, next, '{', '}')
	return ok && entryDescribes(sidecar, figures.SourceName(cleanName(strings.TrimSpace(name))))
}

// entryDescribes: the sidecar's entry for key describes the figure or marks it decorative, and is
// the author's or reviewed (a model draft is still the author's to check).
func entryDescribes(sidecar *figdesc.Sidecar, key string) bool {
	if sidecar == nil {
		return false
	}
	entry := sidecar.Entries[key]
	return entry != nil && entry.Provenance != figdesc.Model && (figdesc.Normalize(entry.Alt) != "" || entry.Decorative)
}

// discardedArguments are the extents in text of the arguments that a command the paper defines never
// uses (its body holds no #k for them): text TeX never typesets, such as a figure description a
// book keeps for its HTML build (\diffyincludegraphics{…}{…}{file}{description}), is no prose.
func discardedArguments(text string, defined map[string]macro) [][2]int {
	ranges := [][2]int{}
	for name, definition := range defined {
		unused := unusedParameters(definition)
		if len(unused) == 0 {
			continue
		}
		for _, use := range calls(text, name, definition) {
			ranges = append(ranges, argumentExtents(text, use.start+len(name)+1, definition, unused)...)
		}
	}
	return ranges
}

// unusedParameters are the parameter numbers (1-based) a definition's body never names.
func unusedParameters(definition macro) map[int]bool {
	unused := map[int]bool{}
	for number := 1; number <= definition.arguments; number++ {
		if !strings.Contains(definition.body, "#"+string(rune('0'+number))) {
			unused[number] = true
		}
	}
	return unused
}

// argumentExtents are the extents of the call's arguments whose numbers are in wanted, read from
// offset (just past the command's name) as readCall reads them.
func argumentExtents(text string, offset int, definition macro, wanted map[int]bool) [][2]int {
	extents := [][2]int{}
	number := 0
	if definition.optional {
		number++
		if _, after, given := argument(text, offset, '[', ']'); given {
			offset = after
		}
	}
	for number < definition.arguments {
		number++
		_, after, ok := argument(text, offset, '{', '}')
		if !ok {
			break
		}
		if wanted[number] {
			extents = append(extents, [2]int{offset, after})
		}
		offset = after
	}
	return extents
}

// pictureEdge is a picture environment's \begin or \end: xfig's pdf_t pictures, TikZ and pgf.
var pictureEdge = regexp.MustCompile(`\\(begin|end)\s*\{(picture|tikzpicture|pgfpicture)\}`)

// pictureAround is the extent (from its \begin to past its \end) of the innermost picture
// environment holding offset; ok is false outside every picture.
func pictureAround(text string, offset int) (start, end int, ok bool) {
	type open struct {
		name  string
		start int
	}
	stack := []open{}
	for _, match := range pictureEdge.FindAllStringSubmatchIndex(text, -1) {
		name := text[match[4]:match[5]]
		if text[match[2]:match[3]] == "begin" {
			stack = append(stack, open{name, match[0]})
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1].name != name {
			continue
		}
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if top.start <= offset && offset < match[1] && (!ok || top.start > start) {
			start, end, ok = top.start, match[1], true
		}
	}
	return start, end, ok
}

// renderedInDrawing: the graphic at offset lies in a picture the last conversion rendered whole as
// one of its drawings (an xfig pdf_t picture overlays labels on its \includegraphics). The export
// shows the drawing, not the file, so the drawing's item is the gap and the graphic has none.
func renderedInDrawing(file *File, offset int, drawings []Drawing) bool {
	start, end, ok := pictureAround(file.Text, offset)
	if !ok {
		return false
	}
	first, last := file.Line(start), file.Line(max(start, end-1))
	for _, drawing := range drawings {
		for _, place := range drawing.Places {
			if place.File == file.Path && place.Line >= first && place.Line <= last {
				return true
			}
		}
	}
	return false
}
