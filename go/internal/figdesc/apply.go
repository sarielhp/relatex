package figdesc

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Summary is what applying the sidecar to a document found.
type Summary struct {
	Source    int      // figures the LaTeX source describes (decision 096), \relatexalt with no entry replacing it
	Author    int      // figures described by the author's own text
	Reviewed  int      // by drafts a person accepted
	Drafts    []string // figures described by drafts nobody has reviewed (their names)
	Stale     []string // entries written for other bytes than the figure has now
	Unmatched []string // entries that name no figure of the document
	Applied   int      // image spans given a description
}

// Apply carries the sidecar's descriptions into the document: each image span of a described
// figure gets "alt" and "alt_provenance" (author, reviewed or model), and "long_description" for
// an entry's long text; a decorative entry gives "decorative" instead. A draft made for other bytes
// than the figure has now is not used; an author's or reviewed text is, and is reported stale. A
// figure the source describes keeps the source's text (precedence: source, reviewed, draft), except
// that the author's or a reviewed entry replaces a figure's \relatexalt (decision 125, amendment 2)
// or the description relatex derives from what it draws (decision 163),
// which itself wins over a draft.
func Apply(document rec.Map, sidecar *Sidecar) Summary {
	summary := Summary{}
	alts := map[string]rec.Map{}
	matched := map[string]bool{}
	for _, figure := range Figures(document) {
		entry := sidecar.Entries[figure.Key]
		if entry != nil {
			matched[figure.Key] = true
		}
		if figure.FromSource() || figure.FromMacro() && (entry == nil || entry.Provenance == Model) {
			summary.Source++
			continue
		}
		fields := summary.entryFields(figure, entry)
		if fields == nil {
			summary.Source += boolCount(figure.FromMacro())
			continue
		}
		for _, id := range figure.IDs {
			alts[id] = fields
		}
	}
	for _, key := range sidecar.Keys() {
		if !matched[key] {
			summary.Unmatched = append(summary.Unmatched, key)
		}
	}
	summary.Applied = len(alts)
	if len(alts) > 0 {
		setAlts(rec.FetchMap(document, "nodes"), alts, 0)
	}
	return summary
}

// entryFields are the span fields of figure's sidecar entry, nil when it gives none, with the
// entry counted in the summary.
func (summary *Summary) entryFields(figure *Figure, entry *Entry) rec.Map {
	if entry == nil {
		return nil
	}
	text := Normalize(entry.Alt)
	if text == "" && !entry.Decorative {
		return nil
	}
	if entry.SHA256 != "" && entry.SHA256 != figure.Identity() {
		summary.Stale = append(summary.Stale, figure.Key)
		if entry.Provenance == Model {
			return nil
		}
	}
	switch entry.Provenance {
	case Model:
		summary.Drafts = append(summary.Drafts, figure.Key)
	case Reviewed:
		summary.Reviewed++
	default:
		summary.Author++
	}
	if entry.Decorative && entry.Provenance != Model {
		return rec.Map{"decorative": true, "alt_provenance": Author}
	}
	if text == "" {
		return nil
	}
	fields := rec.Map{"alt": text, "alt_provenance": entry.Provenance}
	if long := Normalize(entry.Long); long != "" {
		fields["long_description"] = long
		if details := Normalize(entry.Details); details != "" {
			fields[DetailsField] = details
		}
	}
	return fields
}

// FromSource tells whether the source describes the figure (decision 096).
func (f *Figure) FromSource() bool {
	return f.span != nil && FromSource(f.span)
}

// FromMacro tells whether the figure's description is its \relatexalt in the source (decision 125,
// amendment 2), or the text relatex derives from what the source draws (a chess diagram's position,
// decision 163), which the author's or a reviewed entry would replace.
func (f *Figure) FromMacro() bool {
	return f.span != nil && (f.span["alt_provenance"] == SourceMacro || f.Derived())
}

// Derived tells whether the figure's text alternative is one relatex derives from what the source
// draws (decision 163), provenance Inferred; a decorative mark relatex inferred is not.
func (f *Figure) Derived() bool {
	return f.span != nil && f.span["alt_provenance"] == Inferred && f.span["decorative"] != true
}

// Included tells whether the source names the figure's file (\includegraphics), as opposed to
// a drawing (TikZ) the conversion rendered.
func (f *Figure) Included() bool {
	_, named := f.span["source"]
	return f.span != nil && named
}

// Rendered tells whether the figure is a picture the source draws (TikZ) that the conversion
// rendered from the PDF and included as rendered/SHA.pdf, not a file of the author's.
func (f *Figure) Rendered() bool {
	return strings.HasPrefix(f.Source(), "rendered/")
}

// Described tells whether the document gives the figure a description or marks it decorative.
func (f *Figure) Described() bool {
	return described(f.span)
}

// described tells whether an image span has a description or the decorative mark.
func described(span rec.Map) bool {
	alt, _ := span["alt"].(string)
	return Normalize(alt) != "" || span["decorative"] == true
}

// Provenance is who described the figure (Source, Author, Reviewed or Model, or another provenance
// a later stage records), "" when nothing describes it.
func (f *Figure) Provenance() string {
	if f.span == nil || !f.Described() {
		return ""
	}
	provenance, _ := f.span["alt_provenance"].(string)
	return provenance
}

// Normalize is an alternative as one line: runs of white space as one space.
func Normalize(text string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(text, " "))
}

// describedFields are the fields a description sets on an image span.
var describedFields = []string{"alt", "alt_provenance", "long_description", DetailsField, "decorative"}

// DetailsField is the image span's optional longer description (decision 125): it extends the short
// one in long_description, and only the HTML and EPUB show it, on demand.
const DetailsField = "description_details"

// Details is the longer description the figure's image span holds (decision 125), "" when none.
func (f *Figure) Details() string {
	details, _ := f.span[DetailsField].(string)
	return details
}

// setAlts finds every image span below value, table cells included, and gives the described
// ones their fields.
func setAlts(value any, alts map[string]rec.Map, depth int) {
	if depth > 200 {
		return
	}
	switch typed := value.(type) {
	case rec.Map:
		if typed["type"] == "image" {
			if fields, ok := alts[rec.ToS(typed["id"])]; ok {
				for _, name := range describedFields {
					delete(typed, name)
				}
				for name, field := range fields {
					typed[name] = field
				}
			}
		}
		for _, child := range typed {
			setAlts(child, alts, depth+1)
		}
	case []any:
		for _, child := range typed {
			setAlts(child, alts, depth+1)
		}
	}
}

// FromSource tells whether an image span's description came from the source, which no sidecar
// entry or model draft replaces.
func FromSource(span rec.Map) bool {
	return span["alt_provenance"] == Source
}

// Source is the file the LaTeX source includes for the figure, "" for a drawing.
func (f *Figure) Source() string {
	source, _ := f.span["source"].(string)
	return source
}

// Long is the long description the figure's image span holds (decision 110), "" when it holds none.
func (f *Figure) Long() string {
	long, _ := f.span["long_description"].(string)
	return long
}

// Alt is the text alternative the figure's image span holds, "" when it holds none.
func (f *Figure) Alt() string {
	alt, _ := f.span["alt"].(string)
	return alt
}

// boolCount is 1 for true, 0 for false.
func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
