package a11ylint

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/rec"
)

// Item is one gap only the author can close: where it is, what it is and how to fix it.
type Item struct {
	Kind    string `json:"kind"` // figure, draft, drawing, language, table, link, colour, caption, heading, math
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
	// For a figure: its sidecar key, the file as the source names it and where its
	// \includegraphics starts in the file (Annotate's placeholder goes there).
	Key    string `json:"key,omitempty"`
	Target string `json:"target,omitempty"`
	Offset int    `json:"-"`
	// CropLine is the line of the item's float caption, where the review page crops the PDF when the
	// item's own line draws nothing (a table's \begin; export items, decision 184).
	CropLine int `json:"-"`
	// The note: the exact text to add (Suggestion), the line it refers to (NoteLine) and the
	// sentence saying where it goes and why (Note). Place: the annotated copy writes the
	// suggestion, commented out, after NoteLine, as a line to add there (PlaceAfter) or one to
	// replace NoteLine by (PlaceInstead).
	Suggestion string `json:"suggestion,omitempty"`
	NoteLine   int    `json:"note_line,omitempty"`
	Note       string `json:"note,omitempty"`
	Place      string `json:"-"`
	// Advisory marks advice from a heuristic (a complex figure with a short description only, decision
	// 110), which --strict does not count.
	Advisory bool `json:"advisory,omitempty"`
}

// Options choose the opt-in parts of the report.
type Options struct {
	Math bool // the document-level summary of ambiguous math (never per occurrence)
}

// Paper is what the lint reads: the source files, the sidecar and, when there is one, the last
// conversion's figures.
type Paper struct {
	Files   []*File
	Sidecar *figdesc.Sidecar
	// Directory is the paper's source directory, where relatex/hints.json holds author hints (decision
	// 122); "" when the caller does not give it.
	Directory string
	// Drawings are the last conversion's figures that no \includegraphics names (TikZ and other
	// drawings), by their sidecar names, with whether something describes them.
	Drawings []Drawing
	// Complex are the last conversion's figures that look complex (decision 110), with whether a
	// long description reached the export.
	Complex []ComplexFigure
	// Included are the last conversion's figures included from a file, with where TeX included them
	// (decision 115); nil without a conversion that recorded it.
	Included []IncludedFigure
	// SourceMap is the last conversion's line-to-page map (the bundle's source_map), for Crops;
	// nil without a conversion that recorded it.
	SourceMap rec.Map
	// QEDKeys are the figure keys of images the last conversion's QED symbols show: marks named "End of
	// proof" by the export (decision 176), no figures to describe.
	QEDKeys map[string]bool
	// Export is the open structural items of the export's accessibility.json (decision 184), nil
	// without one.
	Export *Export
}

// onlyMarks: every id is the image of a QED mark (and there is at least one).
func onlyMarks(ids []string, marks map[string]bool) bool {
	for _, id := range ids {
		if !marks[id] {
			return false
		}
	}
	return len(ids) > 0
}

// ComplexFigure is a figure whose drawing holds many labels and marks (figdesc.Complexity).
type ComplexFigure struct {
	Key       string
	Source    string // the included file, "" for a drawing
	Measure   figdesc.Complexity
	Described bool // the export gives it a description
	Long      bool // and a long description
}

// Drawing is a figure of the last conversion drawn by the source rather than included.
type Drawing struct {
	Key       string
	Described bool
	Places    []figdesc.Place // where the source begins it, when the conversion recorded it (decision 115)
}

// Lint is the report: every item, in file order, then the document-level ones.
func Lint(paper Paper, options Options) []Item {
	items := languageItems(paper.Files)
	tables, keys := newTableContext(paper.Files), takesKeys(paper.Files)
	beamer := beamerPaper(paper.Files)
	for _, file := range paper.Files {
		items = append(items, figureItems(file, paper, keys)...)
		items = append(items, tableItems(file, tables)...)
		items = append(items, linkItems(file)...)
		items = append(items, colourItems(file, tables.macros)...)
		items = append(items, captionItems(file, floatDescriber(file, paper))...)
		if !beamer {
			items = append(items, headingItems(file)...)
		}
		if paper.Drawings == nil {
			items = append(items, drawnItems(file)...)
		}
	}
	items = append(items, includedItems(paper, staticGraphics(paper.Files), graphicCommands(macros(paper.Files)))...)
	items = append(items, drawingItems(paper)...)
	items = append(items, complexItems(paper)...)
	items = append(items, hintItems(paper)...)
	if options.Math {
		items = append(items, mathItems(paper.Files)...)
	}
	return collapseRepeats(withoutQEDMarks(mergeExport(items, paper), paper))
}

// shownPlaces bounds how many further places a collapsed figure item names.
const shownPlaces = 3

// imageExtension is an extension \includegraphics finds by itself, which a source may leave out.
var imageExtension = regexp.MustCompile(`(?i)\.(?:pdf|png|jpe?g|eps|ps|svg|jbig2|jb2|mps|gif|tiff?|webp)$`)

// collapseRepeats keeps one item per undescribed figure file (decision 167): one sidecar entry
// describes every place of a file, so a file included in several chapters (or by a style macro
// defined twice) is one gap. A figure is its sidecar key and its file without the extension (the key
// is shortened, so two files can share one; a multipage file has a key per page). The first place
// keeps the item and its note; the message names the others.
func collapseRepeats(items []Item) []Item {
	first := map[string]int{}
	others := map[string][]string{}
	kept := make([]Item, 0, len(items))
	for _, item := range items {
		name := path.Clean(cleanName(item.Target))
		if item.Key == "" || name == "." || (item.Kind != "figure" && item.Kind != "draft") {
			kept = append(kept, item)
			continue
		}
		id := item.Kind + "\x00" + item.Key + "\x00" + imageExtension.ReplaceAllString(name, "")
		if _, seen := first[id]; seen {
			others[id] = append(others[id], fmt.Sprintf("%s:%d", item.File, item.Line))
			continue
		}
		first[id] = len(kept)
		kept = append(kept, item)
	}
	for id, places := range others {
		shown := places[:min(len(places), shownPlaces)]
		more := ""
		if len(places) > shownPlaces {
			more = fmt.Sprintf(" and %d more", len(places)-shownPlaces)
		}
		kept[first[id]].Message += fmt.Sprintf(" (also at %s%s)", strings.Join(shown, ", "), more)
	}
	return kept
}

var graphic = regexp.MustCompile(`\\includegraphics\b\s*\*?`)

// figureItems: each included graphic has a description: its alt, actualtext or artifact key,
// acmart's \Description in its figure, or an entry of the sidecar; a model draft there still
// needs review, and so does a placeholder of the annotated copy (alt={TODO: …}, alt={DRAFT: …}).
// A graphic inside a picture the last conversion rendered as a drawing is that drawing's item.
func figureItems(file *File, paper Paper, takesKeys bool) []Item {
	sidecar, drawings := paper.Sidecar, paper.Drawings
	items := []Item{}
	ranges := figureRanges(file.Text)
	acm := isACM(file.Text)
	for _, match := range graphic.FindAllStringIndex(file.Text, -1) {
		options, after, hasOptions := argument(file.Text, match[1], '[', ']')
		name, _, ok := argument(file.Text, after, '{', '}')
		name = strings.TrimSpace(name)
		if !ok || strings.Contains(name, "#") || renderedInDrawing(file, match[0], drawings) {
			continue
		}
		item := Item{File: file.Path, Line: file.Line(match[0]), Offset: match[0], Key: figures.SourceName(cleanName(name)), Target: name}
		keys := keyValues(options)
		if alt, has := keys["alt"]; has {
			items = append(items, placeholderItems(item, alt)...)
			continue
		}
		if keys["actualtext"] != "" || hasArtifact(keys) {
			continue
		}
		if figure := ranges.at(match[0]); figure != nil && (acm && figure.described || figure.macro) {
			continue
		}
		if unusedTemplate(file, match[0], paper.Files) {
			continue
		}
		for _, found := range sidecarItems(item, sidecar.Entries[item.Key], acm) {
			if found.Kind == "figure" {
				found = figureNote(found, file.Text[match[0]:match[1]], figureOptions{options, hasOptions, acm, takesKeys})
			}
			items = append(items, found)
		}
	}
	return items
}

// Placeholder prefixes of the annotated copy's alt keys: the author has not yet written the
// description, or not yet checked the model's draft.
const (
	TodoPrefix  = "TODO:"
	DraftPrefix = "DRAFT:"
	todoFix     = "replace " + TodoPrefix + " … by a description of the figure, or the alt key by artifact if it is decoration"
	draftFix    = "check the draft against the figure, correct it and remove " + DraftPrefix
)

// placeholderItems: an alt key that still holds the annotated copy's placeholder.
func placeholderItems(item Item, alt string) []Item {
	alt = strings.TrimSpace(alt)
	switch {
	case strings.HasPrefix(alt, TodoPrefix):
		item.Kind, item.Message = "figure", "figure "+item.Target+" still has the placeholder alt={"+TodoPrefix+" …}"
		item.Fix = todoFix
	case strings.HasPrefix(alt, DraftPrefix):
		item.Kind, item.Message = "draft", "figure "+item.Target+" has a model draft in alt={"+DraftPrefix+" …} nobody has reviewed"
		item.Fix = draftFix
	default:
		return nil
	}
	return []Item{item}
}

// sidecarItems: a graphic the source does not describe needs a reviewed sidecar entry.
func sidecarItems(item Item, entry *figdesc.Entry, acm bool) []Item {
	switch {
	case entry != nil && entry.Provenance == figdesc.Model:
		item.Kind, item.Message = "draft", "figure "+item.Target+" has a model draft nobody has reviewed"
		item.Fix = "relatex alt review, correct [figs." + item.Key + "] in " + figdesc.FileName + ", then relatex alt accept " + item.Key
	case entry != nil && (figdesc.Normalize(entry.Alt) != "" || entry.Decorative):
		return nil
	default:
		item.Kind, item.Message = "figure", "figure "+item.Target+" has no description"
		item.Fix = figureFix(item.Target, item.Key, acm)
	}
	return []Item{item}
}

// figureOptions are what a figure's note needs: its \includegraphics options (when it has some),
// whether the paper is an acmart one and whether \includegraphics takes graphicx's keys.
type figureOptions struct {
	options         string
	hasOptions, acm bool
	keys            bool
}

// figureNote is the ready-to-fill line for a figure without a description: its \includegraphics
// with graphicx's alt key (TeX Live's graphicx accepts alt, actualtext and artifact; the tagging
// project reads them for the tagged PDF), or the sidecar entry relatex alt drafts.
func figureNote(item Item, command string, given figureOptions) Item {
	options := strings.TrimSpace(given.options)
	if given.hasOptions && options != "" {
		options += ", "
	}
	item.Suggestion = strings.TrimSpace(command) + "[" + options + "alt={…}]{" + item.Target + "}"
	item.NoteLine = item.Line
	sidecar := fmt.Sprintf("or in %s beside the main file: [figs.%s] with alt = \"…\" (after a conversion, "+
		"relatex alt draft drafts it, relatex alt review shows it, relatex alt accept %s keeps a checked draft)",
		figdesc.FileName, item.Key, item.Key)
	switch {
	case !given.keys:
		item.Note = `this paper loads graphics, whose \includegraphics takes no alt key: load graphicx instead and write ` +
			item.Suggestion + ", " + sidecar
	case given.acm:
		item.Note = "ready to fill: " + item.Suggestion + `, or \Description{…} in its figure (acmart), ` + sidecar
	default:
		item.Note = "ready to fill: " + item.Suggestion + ", " + sidecar
	}
	return item
}

func figureFix(name, key string, acm bool) string {
	fix := `\includegraphics[alt={…}]{` + name + `}`
	if acm {
		fix += `, \Description{…} in its figure`
	}
	return fix + `, or [figs.` + key + `] alt = "…" in ` + figdesc.FileName + ` (relatex alt draft drafts one); ` +
		`artifact (or alt={}) if it is decoration`
}

func hasArtifact(keys map[string]string) bool {
	value, has := keys["artifact"]
	return has && strings.TrimSpace(strings.ToLower(value)) != "false"
}

var acmClass = regexp.MustCompile(`\\documentclass\s*(?:\[[^\]]*\])?\s*\{acmart\}`)

// isACM: acmart's \Description is read only in acmart papers; a file other than the main one
// counts as ACM when it uses \Description at all.
func isACM(text string) bool {
	return acmClass.MatchString(text) || strings.Contains(text, `\Description`)
}

// complexItems: notes on the last conversion's complex figures (plots, rich diagrams) that have a
// short description and no long one (decision 110), at the graphic that includes them when the
// source names one. The heuristic is conservative and the item a note: --strict ignores it.
func complexItems(paper Paper) []Item {
	items := []Item{}
	for _, figure := range paper.Complex {
		entry := paper.Sidecar.Entries[figure.Key]
		described := figure.Described || (entry != nil && figdesc.Normalize(entry.Alt) != "")
		if !figure.Measure.Complex() || figure.Long || !described || (entry != nil && figdesc.Normalize(entry.Long) != "") {
			continue
		}
		item := Item{Kind: "long", Advisory: true, Key: figure.Key, Target: figure.Key,
			Message: "figure " + figure.Key + " looks complex (" + figure.Measure.String() + ") and has a short description only",
			Fix: `[figs.` + figure.Key + `] long = "…" in ` + figdesc.FileName + ` (relatex alt set ` + figure.Key +
				` --long "…"), or \Description[short]{long} in an acmart figure; or the data in a table or the text`}
		item.File, item.Line = graphicLocation(paper.Files, figure.Source)
		items = append(items, longNote(item))
	}
	return items
}

// longNote is a complex figure's note (decision 110, in the form of decision 098's amendment): the
// sidecar line to add beside its short description. It belongs in relatex/alt.toml, not in the
// source, so the annotated copy shows it in the mark and REVIEW.md rather than as a line to
// uncomment (no Place).
func longNote(item Item) Item {
	item.Suggestion = `long = "…"`
	item.NoteLine = item.Line
	item.Note = "this figure looks complex: add a long description beside its short one, " + item.Suggestion + " under [figs." +
		item.Key + "] in " + figdesc.FileName + " (relatex alt set " + item.Key + ` --long "…" writes it; relatex alt draft drafts one), ` +
		`or \Description[short]{long} in its figure (acmart): the axes with their ranges, each series and its shape, the labelled parts`
	return item
}

// graphicLocation is where the source includes the file source (as the export records it, with
// the directory and extension TeX found), else the main file.
func graphicLocation(files []*File, source string) (string, int) {
	for _, file := range files {
		for _, match := range graphic.FindAllStringIndex(file.Text, -1) {
			_, after, _ := argument(file.Text, match[1], '[', ']')
			if name, _, ok := argument(file.Text, after, '{', '}'); ok && source != "" &&
				figures.SourceName(strings.TrimSpace(name)) == figures.SourceName(source) {
				return file.Path, file.Line(match[0])
			}
		}
	}
	if len(files) > 0 {
		return files[0].Path, 0
	}
	return "", 0
}

// drawingPlace is where the source begins a drawing (the first undescribed place the conversion
// recorded), else the main file.
func drawingPlace(drawing Drawing, files []*File, main string) (string, int) {
	for _, place := range drawing.Places {
		if !place.Described {
			return place.File, pictureStart(files, place)
		}
	}
	return main, 0
}

// pictureStart is the line of the \begin{tikzpicture} (or pgfpicture, tikzcd) that holds the recorded
// line, within locateWindow lines before it; the recorded line when none is found.
func pictureStart(files []*File, place figdesc.Place) int {
	for _, file := range files {
		if file.Path != place.File || place.Line < 1 || place.Line > len(file.lines) {
			continue
		}
		end := len(file.Text)
		if place.Line < len(file.lines) {
			end = file.lines[place.Line]
		}
		start := file.lines[max(0, place.Line-1-locateWindow)]
		if found := drawing.FindAllStringIndex(file.Text[start:end], -1); len(found) > 0 {
			return file.Line(start + found[len(found)-1][0])
		}
	}
	return place.Line
}

// drawingItems: figures of the last conversion that the source draws (TikZ, pictures) need a
// sidecar entry, as no \includegraphics key can describe them.
func drawingItems(paper Paper) []Item {
	items := []Item{}
	main := ""
	if len(paper.Files) > 0 {
		main = paper.Files[0].Path
	}
	for _, drawing := range paper.Drawings {
		if drawing.Described {
			continue
		}
		file, line := drawingPlace(drawing, paper.Files, main)
		entry := paper.Sidecar.Entries[drawing.Key]
		if entry != nil && (figdesc.Normalize(entry.Alt) != "" || entry.Decorative) {
			if entry.Provenance == figdesc.Model {
				items = append(items, Item{Kind: "draft", File: file, Line: line, Message: "drawing figs/" + drawing.Key + " has a model draft nobody has reviewed",
					Fix: "relatex alt review, then relatex alt accept " + drawing.Key})
			}
			continue
		}
		items = append(items, Item{Kind: "drawing", File: file, Line: line, Message: "drawing figs/" + drawing.Key + " (from the last conversion) has no description",
			Fix: fmt.Sprintf("[figs.%s] alt = \"…\" in %s, or \\Description{…} in its figure (acmart); relatex alt draft drafts one",
				drawing.Key, figdesc.FileName)})
	}
	return items
}

// WithConversion adds a conversion's figures to the paper: the drawings no \includegraphics names
// and the figures that look complex (decision 110), read from its document.
func (paper Paper) WithConversion(document rec.Map) Paper {
	paper.Drawings = []Drawing{}
	marks := qedImageIDs(document)
	for _, figure := range figdesc.Figures(document) {
		if onlyMarks(figure.IDs, marks) {
			if paper.QEDKeys == nil {
				paper.QEDKeys = map[string]bool{}
			}
			paper.QEDKeys[figure.Key] = true
			continue
		}
		if !figure.Included() || figure.Rendered() {
			paper.Drawings = append(paper.Drawings, Drawing{Key: figure.Key, Described: figure.Described(), Places: figure.Places})
		} else if len(figure.Places) > 0 {
			paper.Included = append(paper.Included, IncludedFigure{Key: figure.Key, Source: figure.Source(), Places: figure.Places})
		}
		if measure := figure.Complexity(); measure.Complex() {
			paper.Complex = append(paper.Complex, ComplexFigure{Key: figure.Key, Source: figure.Source(), Measure: measure,
				Described: figure.Described(), Long: strings.TrimSpace(figure.Long()) != ""})
		}
	}
	return paper
}
