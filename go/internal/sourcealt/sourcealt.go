// Package sourcealt is what an author wrote in the LaTeX source about a figure (decision 096):
// graphicx's alt, actualtext and artifact keys (the LaTeX tagging project's), relatex's
// \relatexalt and acmart's \Description of the figure that holds a graphic, as the collector
// records them. These take precedence over the sidecar relatex/alt.toml (decision 086), except
// \relatexalt, which an author's or reviewed sidecar entry replaces (decision 125, amendment 2). The rule that turns them into an
// image's text alternative, its long description or its decorative mark lives here, so the HTML,
// EPUB and tagged PDF read the same values from document.json. It is in the model layer
// (decision 102): paperdoc applies the descriptions while it projects the document.
package sourcealt

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

// ShortLimit is the longest text, in characters, used whole as an alternative; a longer
// description becomes the long description, with its first sentence (or its start) as the alt.
const ShortLimit = 150

// Description is one graphic's description from the source.
type Description struct {
	Alt        string // the short text alternative ("" when decorative)
	Long       string // the long description, when the author's text is longer than an alt
	Details    string // the longer description beside Long (decision 125), from \relatexalt only
	Decorative bool   // artifact, or alt={}: screen readers skip it
	Macro      bool   // from \relatexalt: the sidecar may replace it (provenance figdesc.SourceMacro)
	// Inferred: relatex derived it from what the source draws (a chess diagram's position, decision
	// 163); the sidecar may replace it like a \relatexalt (provenance figdesc.Inferred).
	Inferred bool
}

// Evidence is the collector's records: each graphic use's keys and figure, each figure's
// \Description, each graphic use's place in the source.
type Evidence struct {
	uses     map[int64]rec.Map
	figures  map[int64]rec.Map
	macros   map[int64]rec.Map // each figure's \relatexalt
	pictures map[int64]rec.Map
	origins  map[int64]Origin
	// drawings are where each picture TeX drew (TikZ, picture) begins, by its context number.
	drawings map[int64]Origin
}

// Origin is where the source includes a graphic (decision 115): the file TeX was reading, as the
// source names it, and the line; for a graphic an author's macro includes, the macro's call.
type Origin struct {
	File string
	Line int64
}

// Read collects the graphic_alternative, figure_description, figure_alt and graphic_origin records among
// rows (stream records or an inspection's diagnostics). A later \Description or \relatexalt of the
// same figure replaces an earlier one. A PGF picture's picture_alternative records (one per key, decision 111) merge.
func Read(rows []rec.Map) *Evidence {
	evidence := &Evidence{uses: map[int64]rec.Map{}, figures: map[int64]rec.Map{}, macros: map[int64]rec.Map{}, pictures: map[int64]rec.Map{},
		origins: map[int64]Origin{}, drawings: map[int64]Origin{}}
	for _, row := range rows {
		switch row["code"] {
		case "graphic_alternative":
			if use, ok := row["use"].(int64); ok {
				evidence.uses[use] = row
			}
		case "figure_description":
			if figure, ok := row["figure"].(int64); ok {
				evidence.figures[figure] = row
			}
		case "figure_alt":
			if figure, ok := row["figure"].(int64); ok {
				evidence.macros[figure] = row
			}
		case "picture_alternative":
			if graphic, ok := row["graphic"].(int64); ok {
				evidence.pictures[graphic] = rec.Merge(evidence.pictures[graphic], row)
			}
		case "graphic_origin":
			use, isUse := row["use"].(int64)
			file, isFile := row["file"].(string)
			line, isLine := row["line"].(int64)
			if isUse && isFile && isLine && line >= 1 {
				evidence.origins[use] = Origin{File: file, Line: line}
			}
		case "unsupported_graphic_source":
			evidence.addDrawing(row)
		}
	}
	return evidence
}

// addDrawing keeps where a drawn picture begins, from the first record of its context that says.
func (e *Evidence) addDrawing(row rec.Map) {
	context, isContext := row["context"].(int64)
	file, isFile := row["filename"].(string)
	file = strings.TrimPrefix(file, "./")
	line, isLine := row["line"].(int64)
	if _, known := e.drawings[context]; isContext && isFile && isLine && line >= 1 && !known {
		e.drawings[context] = Origin{File: file, Line: line}
	}
}

// DrawingOrigin is where the source begins the picture of graphic context context (a TikZ picture
// the conversion rendered from the PDF); false when no record says.
func (e *Evidence) DrawingOrigin(context any) (Origin, bool) {
	number, ok := context.(int64)
	if !ok || e == nil {
		return Origin{}, false
	}
	origin, ok := e.drawings[number]
	return origin, ok
}

// OriginOf is where the source includes graphic use use; false when the collector did not record it.
func (e *Evidence) OriginOf(use any) (Origin, bool) {
	number, ok := use.(int64)
	if !ok || e == nil {
		return Origin{}, false
	}
	origin, ok := e.origins[number]
	return origin, ok
}

// For is the description of graphic use use: its own keys first (artifact, a given alt, then
// actualtext), else its figure's \Description; false when the source says nothing about it.
func (e *Evidence) For(use any) (Description, bool) {
	if e == nil {
		return Description{}, false
	}
	return e.lookup(e.uses, use, false)
}

// ForPicture is the description of the PGF picture (its graphic context number, decision 111)
// from the alt, actualtext and artifact keys given to it, else its figure's \Description, else
// what relatex derives from what it draws (a chess diagram's position, decision 163); false when
// the source says nothing about it.
func (e *Evidence) ForPicture(graphic any) (Description, bool) {
	if e == nil {
		return Description{}, false
	}
	if description, ok := e.lookup(e.pictures, graphic, true); ok {
		return description, true
	}
	number, _ := graphic.(int64)
	return chess(e.pictures[number])
}

// lookup is the description of the row a number names in rows; false when it names none. A
// picture's row may hold its figure alone (the binding \relatexalt needs): its figure's
// \Description describes it only when the picture has keys of its own, as before the binding.
func (e *Evidence) lookup(rows map[int64]rec.Map, key any, picture bool) (Description, bool) {
	number, ok := key.(int64)
	if !ok {
		return Description{}, false
	}
	row, ok := rows[number]
	if !ok {
		return Description{}, false
	}
	return e.describe(row, !picture || keyed(row))
}

// keyed tells whether a picture's row holds one of the tagging project's keys.
func keyed(row rec.Map) bool {
	for _, key := range []string{"alt", "actualtext", "artifact"} {
		if _, has := row[key]; has {
			return true
		}
	}
	return false
}

// describe is a graphic's or picture's keys as its description: artifact or an empty alt
// (decorative), a given alt, then actualtext, else its figure's \relatexalt, then (when descriptions)
// its \Description.
func (e *Evidence) describe(row rec.Map, descriptions bool) (Description, bool) {
	alt, given := row["alt"].(string)
	alt = Normalize(alt)
	if Placeholder(alt) {
		return Description{}, false
	}
	if row["artifact"] == true || given && alt == "" {
		return Description{Decorative: true}, true
	}
	if alt != "" {
		return Description{Alt: alt}, true
	}
	if actual := Normalize(text(row["actualtext"])); actual != "" {
		return Description{Alt: actual}, true
	}
	figure, ok := row["figure"].(int64)
	if !ok {
		return Description{}, false
	}
	if macro, found := Macro(e.macros[figure]); found {
		return macro, true
	}
	described, ok := e.figures[figure]
	if !ok || !descriptions {
		return Description{}, false
	}
	return Split(text(described["short"]), text(described["description"]))
}

// Split is acmart's \Description[short]{long} as an alternative and a long description: the short
// form is the alt and the long one its description; without a short form a text up to ShortLimit
// is the alt alone, and a longer one is the long description with its first sentence (clipped to
// ShortLimit at a word) as the alt. False when both are empty.
func Split(short, long string) (Description, bool) {
	short, long = Normalize(short), Normalize(long)
	switch {
	case short != "" && long != "" && long != short:
		return Description{Alt: short, Long: long}, true
	case short != "":
		return Description{Alt: short}, true
	case long == "":
		return Description{}, false
	case len([]rune(long)) <= ShortLimit:
		return Description{Alt: long}, true
	}
	return Description{Alt: Lead(long), Long: long}, true
}

// Macro is a figure_alt record (\relatexalt{alt}{short}[details]) as a description: the alt, the
// short description as the long description and the details beside it; details without a short
// description extend nothing and are left out (decision 125). False without an alt, or with the
// annotated copy's placeholder.
func Macro(row rec.Map) (Description, bool) {
	alt := Normalize(text(row["alt"]))
	if alt == "" || Placeholder(alt) {
		return Description{}, false
	}
	described := Description{Alt: alt, Long: Normalize(text(row["short"])), Macro: true}
	if described.Long != "" {
		described.Details = Normalize(text(row["details"]))
	}
	return described, true
}

var sentenceEnd = regexp.MustCompile(`[.!?](\s|$)`)

// Lead is a long text's first sentence, clipped to ShortLimit characters at a word with an
// ellipsis when the sentence is longer.
func Lead(long string) string {
	if match := sentenceEnd.FindStringIndex(long); match != nil {
		sentence := strings.TrimSpace(long[:match[0]+1])
		if len([]rune(sentence)) <= ShortLimit {
			return sentence
		}
		long = sentence
	}
	runes := []rune(long)
	if len(runes) <= ShortLimit {
		return long
	}
	clipped := string(runes[:ShortLimit-1])
	if cut := strings.LastIndex(clipped, " "); cut > ShortLimit/2 {
		clipped = clipped[:cut]
	}
	return strings.TrimRight(clipped, " ,;:") + "…"
}

// Placeholder tells whether alt is an alt the annotated review copy wrote for the author to replace (decision 098:
// alt={TODO: …}, or a model draft as alt={DRAFT: …}). It describes nothing yet (decision 111).
func Placeholder(alt string) bool {
	return strings.HasPrefix(alt, "TODO:") || strings.HasPrefix(alt, "DRAFT:")
}

var spaces = regexp.MustCompile(`\s+`)

// Normalize is a text as one line: runs of white space as one space.
func Normalize(value string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(value, " "))
}

func text(value any) string {
	s, _ := value.(string)
	return s
}

// Apply gives an image span the fields of its source description: alt and alt_provenance, and
// long_description with description_details; or decorative and alt_provenance. A \relatexalt
// description's provenance is figdesc.SourceMacro, which the sidecar may replace.
func Apply(span rec.Map, description Description) {
	span["alt_provenance"] = figdesc.Source
	if description.Macro {
		span["alt_provenance"] = figdesc.SourceMacro
	}
	if description.Inferred {
		span["alt_provenance"] = figdesc.Inferred
	}
	if description.Decorative {
		span["decorative"] = true
		return
	}
	span["alt"] = description.Alt
	if description.Long != "" {
		span["long_description"] = description.Long
		if description.Details != "" {
			span[figdesc.DetailsField] = description.Details
		}
	}
}
