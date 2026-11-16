// Package pdftag is the tagged PDF's route 2 (decision 105): it tags the ordinary PDF of a finished
// conversion afterwards, from relatex's verified model; it began with the documents LaTeX's own
// tagging (route 1, removed on 2026-11-03) could not tag: Beamer decks, REVTeX papers. The page
// content keeps every byte that paints; it is wrapped in marked content bound to a structure tree made from the
// document's nodes, the rest marked as Artifact. It is an output: it reads the export directory
// (document.json.gz and the layout parts) and the ordinary PDF, and writes through qpdf.
package pdftag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/glyphnames"
	"github.com/sarielhp/relatex/internal/glyphowner"
	"github.com/sarielhp/relatex/internal/pdffonts"
)

// Options say what to tag.
type Options struct {
	Output string // the conversion's output directory
	PDF    string // the ordinary PDF; default: the only *.pdf in Output
	Target string // the tagged PDF to write; default Output/pdf-ua2/JOB.pdf
	Glyphs string // the reviewed glyph names, tex/relatex-symbol-glyphs.txt
}

// Report is what the tagging did.
type Report struct {
	PDF           string          `json:"pdf"`
	Source        string          `json:"source"`
	Pages         int             `json:"pages"`
	PagesTagged   int             `json:"pages_tagged"`
	PageFailures  map[int]string  `json:"page_failures,omitempty"`
	Glyphs        int             `json:"glyphs"`
	GlyphsBound   int             `json:"glyphs_bound"`
	PositionsSeen int             `json:"positions_checked"`
	Words         int             `json:"words"`
	WordsBound    int             `json:"words_bound"`
	Formulas      int             `json:"formulas"`
	FormulasBound int             `json:"formulas_bound"`
	Images        int             `json:"images"`
	ImagesPlaced  int             `json:"images_placed"`
	Undescribed   []string        `json:"undescribed,omitempty"`
	FigureFonts   []FigureFont    `json:"figure_fonts,omitempty"`
	Elements      map[string]int  `json:"elements"`
	Links         int             `json:"links"`
	LinksWithText int             `json:"links_with_text"`
	ActualText    int             `json:"actual_text,omitempty"` // glyphs given the document's character (decision 148)
	Misses        []string        `json:"misses,omitempty"`
	Title         string          `json:"title"`
	Language      string          `json:"language"`
	Warnings      string          `json:"qpdf_warnings,omitempty"`
	Order         OrderCheck      `json:"order_check"`
	Appearance    AppearanceCheck `json:"appearance_check"`
	Fonts         FontReport      `json:"fonts"`
	Binding       OwnerCheck      `json:"binding"`
}

// Faithful reports whether every page was tagged, looks as the ordinary PDF and reads back as the
// model says.
func (r Report) Faithful() bool {
	order, look := r.Order, r.Appearance
	return len(r.PageFailures) == 0 && order.Failure == "" && order.TextEqual == order.TextLeaves &&
		look.Failure == "" && look.Identical == look.Pages
}

// tagger holds one tagging.
type tagger struct {
	options  Options
	doc      *document
	painted  map[int][]paintedGlyph
	file     *pdfFile
	builder  *builder
	binding  *binding
	report   Report
	placed   map[string]bool
	orphans  []*selem                   // leaves bound to glyphs that the tree does not reach
	leafText map[string]expectedText    // each leaf's text in the model
	prepared []*prepared                // the pages interpreted and mapped to the layout
	owners   *glyphowner.Owners         // the core's glyph owners; nil for an export without them
	moved    map[string]map[int]float64 // width changes the rewritten show operators undo, by font
	drawn    map[string]bool            // drawing fonts (pdffonts.Drawing), by reference
	// placedOwners are the glyphs the owners gave a leaf that TeX set outside every literal-drawn box.
	placedOwners map[glyphRef]string
	// linkElements are the Link elements that hold their annotation's text (links.go), by the
	// annotation's reference; linkCount counts each host leaf's links.
	linkElements map[string]*selem
	linkCount    map[string]int
	// linkHosts are the text leaves that hold them, by the Link's leaf: a second annotation over
	// the same text (placeLink) goes into the host, not into the Link.
	linkHosts map[string]*selem
	// spelled is the document's character of each glyph the font spells otherwise (spelling.go).
	spelled map[glyphRef]string
	// stretched are the codes a viewer stretched to their Widths in the ordinary PDF, by font (pdffonts Stretched).
	stretched map[string]map[int]pdffonts.Stretch
	// unicodes are the composite fonts' ToUnicode values by font reference (continuations.go).
	unicodes map[string]map[int]string
}

// Tag tags the ordinary PDF of a conversion and writes the tagged one.
func Tag(options Options) (Report, error) {
	t := &tagger{options: options, placed: map[string]bool{}}
	if err := t.load(); err != nil {
		return Report{}, err
	}
	defer os.Remove(t.file.path)
	root := t.builder.build(t.doc.Root)
	if len(root) != 1 {
		return Report{}, fmt.Errorf("the document tree has no single root")
	}
	root[0].role = "Document"
	root[0].kids = append(root[0].kids, t.builder.notes()...)
	t.builder.markTitle(root[0])
	t.builder.markContents(root[0])
	t.builder.markReferences(root[0])
	pages := t.file.pages()
	t.report.Pages = len(pages)
	pieces := t.builder.pieces()
	t.leafText = leafExpectations(pieces)
	t.binding = bind(pieces, newGlyphStream(t.painted, len(pages)))
	t.report.Binding = OwnerCheck{Source: "alignment"}
	if t.owners != nil {
		t.report.Binding = t.bindOwners(t.owners)
		if t.bindLabels() {
			t.leafText = leafExpectations(t.builder.pieces())
		}
	}
	t.bindLinks()
	t.spellLeaves()
	t.prepared = make([]*prepared, len(pages))
	for index, p := range pages {
		t.prepared[index] = t.prepare(index+1, p)
		t.completeContinuations(t.prepared[index])
	}
	placeImages(t.prepared, t.binding.images, t.drawnLeaves())
	t.report.FigureFonts = t.figureFonts()
	t.report.Fonts = t.repairFonts(t.usedCodes(), glyphnames.Load(t.options.Glyphs))
	t.report.Fonts.GlyphMaps += t.identityGlyphMaps()
	results := make([]pageResult, len(pages))
	for index, page := range t.prepared {
		results[index] = t.tagPage(page)
		t.collect(index+1, results[index])
	}
	writer := newTreeWriter(t, pages)
	if err := writer.write(root[0], results); err != nil {
		return Report{}, err
	}
	t.summarize(writer)
	warnings, err := t.file.write(t.options.Target, t.options.Target)
	if err != nil {
		return Report{}, err
	}
	t.report.Warnings = warnings
	t.report.Order = t.checkOrder(root[0])
	t.report.Appearance = checkAppearance(t.options.PDF, t.options.Target, len(pages))
	return t.report, nil
}

// load reads the model, the layout and the PDF.
func (t *tagger) load() error {
	qpdf := command.Resolve("qpdf")
	if qpdf == "" {
		return fmt.Errorf("qpdf is required to tag a PDF")
	}
	if t.options.PDF == "" {
		found, _ := filepath.Glob(filepath.Join(t.options.Output, "*.pdf"))
		if len(found) != 1 {
			return fmt.Errorf("%s: expected one PDF, found %d (give the PDF)", t.options.Output, len(found))
		}
		t.options.PDF = found[0]
	}
	if t.options.Target == "" {
		t.options.Target = filepath.Join(t.options.Output, "pdf-ua2", filepath.Base(t.options.PDF))
	}
	var err error
	if t.doc, err = readDocument(t.options.Output); err != nil {
		return err
	}
	if t.painted, _, err = readLayout(t.options.Output); err != nil {
		return err
	}
	if t.owners, err = readOwners(t.options.Output); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(t.options.Target), 0o755); err != nil {
		return err
	}
	if t.file, err = readCopy(qpdf, t.options.PDF, t.options.Target+".input.pdf"); err != nil {
		return err
	}
	t.builder = newBuilder(t.doc)
	t.report.PDF, t.report.Source = t.options.Target, t.options.PDF
	return nil
}

// roleOf is a leaf's structure role, the marked-content tag of its content.
func (t *tagger) roleOf(leaf string) string {
	if element, ok := t.builder.leaves[leaf]; ok {
		return element.role
	}
	return "Span"
}

// ownsPaths reports whether the paths painted among a leaf's glyphs are its own.
func (t *tagger) ownsPaths(leaf string) bool {
	role := t.roleOf(leaf)
	return role == "Formula" || role == "Lbl" || role == "Figure"
}

// collect attaches a page's marks to their elements.
func (t *tagger) collect(number int, result pageResult) {
	t.report.Glyphs += result.glyphs
	t.report.PositionsSeen += result.checked
	if result.failure != "" {
		if t.report.PageFailures == nil {
			t.report.PageFailures = map[int]string{}
		}
		t.report.PageFailures[number] = result.failure
	} else {
		t.report.PagesTagged++
	}
	for _, m := range result.marks {
		element := t.builder.leaves[m.leaf]
		if element == nil {
			element = t.builder.leaf("P", m.leaf)
			t.orphans = append(t.orphans, element)
		}
		element.mcrs = append(element.mcrs, mcr{page: number, mcid: m.mcid, first: m.first})
		if t.roleOf(m.leaf) == "Figure" {
			t.placed[m.leaf] = true
		}
	}
}

// summarize fills the report's counts.
func (t *tagger) summarize(writer *treeWriter) {
	b := t.binding
	t.report.GlyphsBound = len(b.leafOf)
	t.report.Words, t.report.WordsBound = b.words, b.wordsBound
	t.report.Formulas, t.report.FormulasBound = b.formulas, b.bound
	if t.owners != nil {
		t.report.FormulasBound = t.boundFormulas()
	}
	t.report.Images, t.report.ImagesPlaced = len(b.images), len(t.placed)
	t.report.Undescribed = t.builder.undescribe
	t.report.Elements = writer.roles
	t.report.Links = writer.links
	t.report.Title, t.report.Language = writer.title, writer.language
	if len(b.misses) > 40 {
		t.report.Misses = append(b.misses[:40:40], fmt.Sprintf("… %d more", len(b.misses)-40))
	} else {
		t.report.Misses = b.misses
	}
}

// jobName is the PDF's name without its extension.
func jobName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
