package papersource

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/reading"
	"github.com/sarielhp/relatex/internal/rec"
)

// Paragraph alignment (decision 116): the typeset lines of each paragraph, read from the glyph map,
// against the document's text columns (reading.Alignment). The result is the node's `align`.

// lineEnd is the glyph map's flag for the last glyph of a typeset line.
const lineEnd = 4

// lineReader gathers the typeset lines of a glyph map by the paragraph they belong to.
type lineReader struct {
	index *ownerIndex
	lines map[string][]reading.Extent
	order []string
	// the line being read
	left, right float64
	node        string
	started     bool
}

// page adds one glyph map page: each typeset line (up to its line-end flag) with the paragraph
// of its first text glyph; the formulas and hyphens in the line widen it.
func (r *lineReader) page(page glyphMapPage) {
	g := page.Glyphs
	size := len(g.Text)
	if len(g.DX) != size || len(g.DY) != size || len(g.Formula) != size || len(g.Slot) != size ||
		len(g.Flags) != size || len(g.Owner) != size || len(g.Font) != size || len(g.Char) != size {
		return
	}
	x := int64(0)
	for at := range size {
		x += g.DX[at]
		if _, moved := page.Matrices[fmt.Sprint(at+1)]; !moved && g.Flags[at]&unknownPosition == 0 {
			r.glyph(page, at, x)
		}
		if g.Flags[at]&lineEnd != 0 {
			r.flush()
		}
	}
	r.flush()
}

func (r *lineReader) glyph(page glyphMapPage, at int, x int64) {
	g := page.Glyphs
	metric := page.Metrics[fmt.Sprintf("%d:%d", g.Font[at], g.Char[at])]
	if len(metric) == 0 {
		return
	}
	left, right := bigPoints(x), bigPoints(x+metric[0])
	if !r.started || left < r.left {
		r.left = left
	}
	if !r.started || right > r.right {
		r.right = right
	}
	r.started = true
	if r.node == "" && g.Formula[at] == 0 {
		r.node = r.index.owner(page.Page, 0, g.Owner[at], g.Slot[at])
	}
}

func (r *lineReader) flush() {
	if r.started && r.node != "" {
		node, _ := strings.CutSuffix(r.node, reading.TitleSuffix)
		if _, seen := r.lines[node]; !seen {
			r.order = append(r.order, node)
		}
		r.lines[node] = append(r.lines[node], reading.Extent{Left: r.left, Right: r.right})
	}
	r.started, r.node = false, ""
}

// alignParagraphs sets `align` on the document's paragraphs that are centred or right-aligned on
// the PDF page. A build without a glyph map records nothing.
func (s *Source) alignParagraphs(document rec.Map) {
	if _, inventoried := s.manifest[s.streamPath+".glyphs.jsonl"]; !inventoried || s.slotBlocks == nil {
		return
	}
	reader := &lineReader{index: newOwnerIndex(document, s.retention, s.slotBlocks), lines: map[string][]reading.Extent{}}
	s.eachGlyphPage(func(page glyphMapPage) bool {
		reader.page(page)
		return true
	})
	nodes := rec.FetchMap(document, "nodes")
	paragraphs := [][]reading.Extent{}
	for _, id := range reader.order {
		if rec.AsMap(nodes[id])["type"] == "paragraph" {
			paragraphs = append(paragraphs, reader.lines[id])
		}
	}
	columns := reading.TextColumns(paragraphs)
	// A block holding a table (authors set in \tabular) is placed as one box: its cells align inside it.
	for _, id := range reader.order {
		node := rec.AsMap(nodes[id])
		if node["type"] != "paragraph" {
			continue
		}
		lines, room := reader.lines[id], reading.ParagraphRoom
		if holdsTable(node) {
			lines, room = []reading.Extent{reading.Hull(lines)}, reading.TableRoom
		}
		if align := reading.Alignment(lines, columns, room); align != "" {
			node["align"] = align
		}
	}
}

// holdsTable: whether the paragraph's content has a table grid.
func holdsTable(node rec.Map) bool {
	return slices.ContainsFunc(rec.FetchList(node, "content"), func(span any) bool { return rec.AsMap(span)["type"] == "table_grid" })
}
