package pdftag

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// The model as route 2 reads it: the document's nodes from document.json.gz and the painted
// glyphs of every page from the layout parts.

// contentItem is one item of a block's content.
type contentItem struct {
	Type            string       `json:"type"`
	Image           *contentItem `json:"image"` // a QED mark's image (decision 176)
	Text            string       `json:"text"`
	Node            string       `json:"node"`
	ID              string       `json:"id"`
	Source          string       `json:"source"`
	Alt             string       `json:"alt"`
	LongDescription string       `json:"long_description"`
	Decorative      bool         `json:"decorative"`
	Grid            *tableGrid   `json:"grid"`
	WidthSp         float64      `json:"width_sp"`
	HeightSp        float64      `json:"height_sp"`
	Entry           int          `json:"bibliography_entry"` // the bibliography entry the text belongs to, from 1
	Style           struct {
		Shape string `json:"shape"`
	} `json:"style"`
}

// tableGrid is a table's cells as the model holds them; raw is the whole grid, which the table
// layout reads for its header cells (decision 131).
type tableGrid struct {
	Cells           []tableCell      `json:"cells"`
	NativeFragments []nativeFragment `json:"native_fragments"`
	raw             rec.Map
}

// UnmarshalJSON reads the grid's cells and keeps the whole grid.
func (g *tableGrid) UnmarshalJSON(data []byte) error {
	type cells tableGrid
	if err := json.Unmarshal(data, (*cells)(g)); err != nil {
		return err
	}
	raw, err := canonjson.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	g.raw, _ = raw.(rec.Map)
	return nil
}

// tableCell is one cell of a table grid.
type tableCell struct {
	ID         string        `json:"id"`
	Row        int           `json:"row"`
	Column     int           `json:"column"`
	Role       string        `json:"role"`
	Colspan    int           `json:"colspan"`
	Rowspan    int           `json:"rowspan"`
	Content    []contentItem `json:"content"`
	SourceCell *int          `json:"source_cell"` // the native cell that set it, as the fragments name it
}

// authorTeX is a formula's TeX as its author wrote it, with the status decision 064 gave it.
type authorTeX struct {
	Status string `json:"status"`
	Text   string `json:"text"`
}

// formulaTeXField is a formula's TeX written from its MathML (decision 141).
type formulaTeXField struct {
	Text        string `json:"text"`
	Environment string `json:"environment"`
}

// node is a node of the document tree.
type node struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Children   []string         `json:"children"`
	Content    []contentItem    `json:"content"`
	MathML     string           `json:"mathml"`
	TeX        *authorTeX       `json:"tex"`
	FormulaTeX *formulaTeXField `json:"formula_tex"` // the TeX written from the MathML that round-trips (decision 141)
	Semantics  struct {
		Level      int    `json:"level"`
		Kind       string `json:"kind"`
		Runin      bool   `json:"runin"`
		TitleSpans int    `json:"title_spans"` // a run-in heading's title: its paragraph's first spans
	} `json:"semantics"`
	Alt             string `json:"alt"`
	LongDescription string `json:"long_description"`
	Decorative      bool   `json:"decorative"`
	Labels          []struct {
		Block string `json:"block"`
		Text  string `json:"text"`
	} `json:"labels"` // a note's number, typeset at the start of its block
}

// document is the part of document.json route 2 uses.
type document struct {
	Root       string          `json:"root"`
	Nodes      map[string]node `json:"nodes"`
	BlockOrder []string        `json:"block_order"`
	NoteOrder  []string        `json:"note_order"`
	Language   struct {
		Tag string `json:"tag"`
	} `json:"language"`
	Frontmatter struct {
		Title any `json:"title"`
	} `json:"frontmatter"`
}

// layoutGlyph is a glyph of a layout page; Paint is nil for a space the layout added.
type layoutGlyph struct {
	ID             string `json:"id"`
	Text           string `json:"text"`
	OutlineBinding struct {
		Paint *struct {
			Index    int        `json:"index"`
			Baseline [2]float64 `json:"baseline"`
			Advance  [2]float64 `json:"advance"`
		} `json:"paint"`
	} `json:"outline_binding"`
}

// layoutPage is a page of the layout.
type layoutPage struct {
	Number int           `json:"number"`
	Width  float64       `json:"width"`
	Height float64       `json:"height"`
	Glyphs []layoutGlyph `json:"glyphs"`
}

// paintedGlyph is a glyph the page paints, by its paint index.
type paintedGlyph struct {
	text     string
	baseline [2]float64
	unknown  bool    // a paint index the layout skipped (a glyph it did not record): its position unknown
	advance  float64 // its horizontal advance in bp; 0 when the layout gives none
}

func readGzipJSON(path string, into any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := json.NewDecoder(reader).Decode(into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// readDocument reads the document from an output directory's document.json.gz.
func readDocument(output string) (*document, error) {
	var bundle struct {
		Payload struct {
			Document document `json:"document"`
		} `json:"payload"`
	}
	if err := readGzipJSON(filepath.Join(output, "document.json.gz"), &bundle); err != nil {
		return nil, err
	}
	doc := bundle.Payload.Document
	if doc.Root == "" || len(doc.Nodes) == 0 {
		return nil, fmt.Errorf("%s: no document tree", output)
	}
	return &doc, nil
}

// readLayout reads the painted glyphs of every page, by page number, from the layout parts.
func readLayout(output string) (map[int][]paintedGlyph, map[int]layoutPage, error) {
	parts, _ := filepath.Glob(filepath.Join(output, "layout", "pages-*.json.gz"))
	if len(parts) == 0 {
		return nil, nil, fmt.Errorf("%s: no layout parts (convert without --no-layout)", output)
	}
	slices.Sort(parts)
	glyphs, pages := map[int][]paintedGlyph{}, map[int]layoutPage{}
	for _, part := range parts {
		var content struct {
			Pages []layoutPage `json:"pages"`
		}
		if err := readGzipJSON(part, &content); err != nil {
			return nil, nil, err
		}
		for _, p := range content.Pages {
			painted, err := paintOrder(p)
			if err != nil {
				return nil, nil, err
			}
			glyphs[p.Number] = painted
			p.Glyphs = nil
			pages[p.Number] = p
		}
	}
	return glyphs, pages, nil
}

// paintOrder lists a page's painted glyphs by paint index.
func paintOrder(p layoutPage) ([]paintedGlyph, error) {
	var painted []paintedGlyph
	for _, glyph := range p.Glyphs {
		paint := glyph.OutlineBinding.Paint
		if paint == nil {
			continue
		}
		for len(painted) <= paint.Index {
			painted = append(painted, paintedGlyph{unknown: true})
		}
		painted[paint.Index] = paintedGlyph{text: glyph.Text, baseline: paint.Baseline, advance: paint.Advance[0]}
	}
	return painted, nil
}

// linkLeftSlack is how far left of a link's box a glyph may begin and still be inside it, in bp.
const linkLeftSlack = 1.0

// snapped is the glyph's origin, moved onto the box's left edge when the glyph straddles it: it
// begins within linkLeftSlack left of the edge and more than half its advance lies right of it.
// A link's box may begin just inside a glyph (a W at a line start: its origin lies 0.6 bp left of
// the box), and that glyph is the link's.
func (g paintedGlyph) snapped(rect bounds) [2]float64 {
	x := g.baseline[0]
	half := float64(g.advance / 2) // rounded before the sum: no fused multiply-add on arm64 (decision 132)
	if x < rect[0] && rect[0]-x <= linkLeftSlack && x+half > rect[0] {
		x = rect[0]
	}
	return [2]float64{x, g.baseline[1]}
}
