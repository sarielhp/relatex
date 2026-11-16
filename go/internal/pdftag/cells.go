package pdftag

import (
	"maps"
	"math"
	"sort"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/glyphowner"
)

// The cells of a table's glyphs (decision 105, table cells): the owners give a table's glyphs the
// block that holds the table, which is no leaf once the block is cut at its tables, and the text
// alignment, looking for each cell's text anywhere, put a cell's figures in the cell or row that
// spelled the same (papers 0237, 0565, 0668, 1099). The model's rich table keeps what TeX painted:
// each native fragment lists its glyphs with their baselines and the source cell that set them. A
// TeX glyph at a fragment glyph's position belongs to that glyph's cell.
//
// The fragment's baselines are in the table's own box from TeX's reference point; the owners' are on
// the page. Where the table lies on the page is found as TeX's own glyph positions agree: the
// placement (an offset, and a scale for a table set in \resizebox) under which nearly every glyph
// of the fragment meets an owners' glyph of its text.

// nativeFragment is a rich table's piece on one page as the model holds it.
type nativeFragment struct {
	Page   int           `json:"page"`
	Glyphs []nativeGlyph `json:"glyphs"`
}

// nativeGlyph is a glyph TeX painted in a rich table: its baseline in sp from TeX's reference
// point, its text and the source cell that set it (none for a glyph outside every cell).
type nativeGlyph struct {
	Baseline [2]float64 `json:"baseline_sp"`
	Text     string     `json:"text"`
	Font     string     `json:"font"`
	Cell     *int       `json:"cell"`
}

// cellTolerance is how far, in bp, a placed fragment glyph may lie from the owners' glyph: both are
// TeX's own positions, the owners' rounded to the hundredth, and the scale found from two of them.
// A math cell's painted glyphs keep the math list's metrics, not the recorder's, and a letter of a
// cell lies up to 0.44 bp from its recorded baseline (papers5 1370's table: TD "1" read "21"), so
// both sides are held together as loosely as the owners' own matching (ownerTolerance) holds TeX's
// origin to the painted glyph.
const cellTolerance = 0.5

// minimumCellMatch is the share of a fragment's glyphs that must meet an owners' glyph of their
// text under the placement found: below it the fragment places nothing.
const minimumCellMatch = 0.9

// anchors bounds the glyphs at either end of a fragment tried as anchors of its placement (a glyph
// the owners spell otherwise, a ligature, anchors nothing).
const anchors = 3

// sampleSize is how many glyphs, spread over a large fragment, a placement must mostly meet before
// all are looked at.
const sampleSize = 8

// Scale bounds of a resized table.
const (
	minimumScale = 0.1
	maximumScale = 10
)

// TableCells reports the table glyphs placed in their cells by the tables' own geometry.
type TableCells struct {
	Fragments int `json:"fragments"`          // rich table fragments with glyphs in cells, placed
	Unplaced  int `json:"unplaced,omitempty"` // fragments no placement brought onto the owners' glyphs
	Glyphs    int `json:"glyphs"`             // painted glyphs bound to their cell this way
}

// cellIndex is, by page, the TeX glyph indices that lie in a table cell, with the cell's leaf.
type cellIndex map[int]map[int]string

// tableCells finds, for every rich table fragment, the owners' glyphs of its cells.
func (t *tagger) tableCells(owners *glyphowner.Owners, report *TableCells) cellIndex {
	pages := map[int]*positionIndex{}
	for _, p := range owners.Pages {
		pages[p.Number] = newPositionIndex(p)
	}
	index := cellIndex{}
	var fixes []sequenceFix
	for _, grid := range t.builder.grids {
		cells := map[int]string{}
		for _, cell := range grid.Cells {
			if cell.SourceCell != nil && !pictureCell(cell) {
				cells[*cell.SourceCell] = cell.ID
			}
		}
		block := gridBlock(t.builder, cells)
		for _, fragment := range grid.NativeFragments {
			glyphs, leaves := cellGlyphs(fragment, cells)
			if len(glyphs) == 0 {
				continue
			}
			if index[fragment.Page] == nil {
				index[fragment.Page] = map[int]string{}
			}
			if placed, fix := placeFragmentBlock(glyphs, pages[fragment.Page], cells, index[fragment.Page], owners, block); placed {
				report.Fragments++
				if fix != nil {
					fixes = append(fixes, *fix)
				}
				continue
			}
			if page := pages[fragment.Page]; block != "" && page != nil {
				if placed := sequencePlace(glyphs, leaves, blockGlyphs(owners, page.page, block), page.text); placed != nil {
					maps.Copy(index[fragment.Page], placed)
					report.Fragments++
					continue
				}
			}
			report.Unplaced++
		}
	}
	for _, fix := range fixes {
		if bound := index[fix.page.page.Number]; bound != nil {
			placeSequence(bound, fix)
		}
	}
	return index
}

// cellGlyphs are a fragment's glyphs set by a cell of the grid, their baselines in bp and their
// text normalized, with their cells' leaves.
func cellGlyphs(fragment nativeFragment, cells map[int]string) ([]nativeGlyph, []string) {
	var glyphs []nativeGlyph
	var leaves []string
	for _, glyph := range fragment.Glyphs {
		if glyph.Cell != nil && cells[*glyph.Cell] != "" {
			glyph.Text = string(normalized(glyph.Text))
			glyph.Baseline = [2]float64{geometry.SPToBP(glyph.Baseline[0]), geometry.SPToBP(glyph.Baseline[1])}
			glyphs = append(glyphs, glyph)
			leaves = append(leaves, cells[*glyph.Cell])
		}
	}
	return glyphs, leaves
}

// placeFragment maps the owners' glyph indices of a fragment's glyphs to their cells' leaves,
// adding them to the page's bound glyphs, and reports whether any was added. A fragment's glyphs
// do not always move together: a styled run's font has other metrics (papers4 1003), a table's
// columns are stretched (papers2 0390), a run is set sideways (papers4 0942), or only a row's own
// span finds a \resizebox scale (papers5 1246). The glyphs the fragment's placement leaves over
// are placed by their cell and run (cellruns.go). A glyph another table's cell already has stays
// its own: a later fragment's equal text (a row of identical figures) must not take it.
func placeFragment(glyphs []nativeGlyph, page *positionIndex, cells map[int]string, bound map[int]string) bool {
	placed, _ := placeFragmentBlock(glyphs, page, cells, bound, nil, "")
	return placed
}

// placeFragmentBlock is placeFragment with the block the fragment's cells were cut from: a cell the
// fragment's own geometry leaves unplaced may be set at an angle placement cannot follow, and its
// glyphs are then placed by the block's glyphs in TeX's order (cellsequence.go placeSequence). The
// correction is returned and applied once every fragment has placed, so that the owners a
// correction frees are not taken by a later fragment of the page.
func placeFragmentBlock(glyphs []nativeGlyph, page *positionIndex, cells map[int]string, bound map[int]string, owners *glyphowner.Owners, block string) (bool, *sequenceFix) {
	if page == nil {
		return false, nil
	}
	before := len(bound)
	matched := make([]bool, len(glyphs))
	leaves := map[string]bool{}
	for _, glyph := range glyphs {
		leaves[cells[*glyph.Cell]] = true
	}
	var taken map[int]bool
	for at, leaf := range bound {
		if leaves[leaf] {
			continue
		}
		if taken == nil {
			taken = map[int]bool{}
		}
		taken[at] = true
	}
	var near *placement
	if at, ok := page.placement(glyphs, taken, nil); ok {
		placeGlyphs(bound, matched, glyphs, page, at, cells, taken)
		near = &at
	}
	lost := unplacedCells(matched, glyphs, cells)
	placeRuns(bound, matched, fragmentCells(glyphs), page, cells, true, near)
	placeRuns(bound, matched, fragmentFontRuns(glyphs), page, cells, false, near)
	bindMissed(bound, matched, glyphs, page, cells, near)
	placed := len(bound) > before
	var fix *sequenceFix
	if placed && owners != nil && block != "" && len(lost) > 0 {
		fix = &sequenceFix{owners: owners, block: block, glyphs: glyphs, cells: cells, page: page, lost: lost}
	}
	return placed, fix
}

// placeGlyphs binds the glyphs a placement brings onto the owners' glyphs of their text, but not
// the glyphs taken.
func placeGlyphs(placed map[int]string, matched []bool, glyphs []nativeGlyph, page *positionIndex, at placement, cells map[int]string, taken map[int]bool) {
	for offset, glyph := range glyphs {
		if matched[offset] {
			continue
		}
		if found, _ := page.find(glyph, at, taken); found >= 0 {
			placed[found] = cells[*glyph.Cell]
			matched[offset] = true
		}
	}
}

// placement puts a fragment's baselines on the page: scaled about TeX's reference point, then
// moved.
type placement struct {
	scale  [2]float64
	offset [2]float64
	turn   int // quarter turns of the table's own box before the scale, for a table set sideways
}

func (p placement) apply(point [2]float64) [2]float64 {
	point = turned(point, p.turn)
	return [2]float64{float64(point[0]*p.scale[0]) + p.offset[0], float64(point[1]*p.scale[1]) + p.offset[1]}
}

// turned is a point turned by quarter turns about TeX's reference point (y down).
func turned(point [2]float64, quarters int) [2]float64 {
	switch quarters {
	case 1:
		return [2]float64{-point[1], point[0]}
	case 2:
		return [2]float64{-point[0], -point[1]}
	case 3:
		return [2]float64{point[1], -point[0]}
	}
	return point
}

// positionIndex finds a page's owners' glyphs by position.
type positionIndex struct {
	page    glyphowner.Page
	text    []string // the glyphs' text, normalized
	buckets map[[2]int][]int
}

func newPositionIndex(p glyphowner.Page) *positionIndex {
	index := &positionIndex{page: p, text: make([]string, len(p.X)), buckets: map[[2]int][]int{}}
	unknown := map[int]bool{}
	for _, at := range p.Unknown {
		unknown[at] = true
	}
	for at := range p.X {
		if at < len(p.Text) {
			index.text[at] = string(normalized(p.Text[at]))
		}
		if !unknown[at] {
			key := bucket(p.X[at], p.Y[at])
			index.buckets[key] = append(index.buckets[key], at)
		}
	}
	return index
}

func bucket(x, y float64) [2]int { return [2]int{int(math.Floor(x)), int(math.Floor(y))} }

// find is the owners' glyph at a fragment glyph's placed position, -1 when none lies there, the one
// with its text first and the nearest of those when several do; same reports whether its text is
// the glyph's. skip holds back the owners' glyphs another cell has already taken: a fallback
// placement must neither qualify nor bind on them.
func (p *positionIndex) find(glyph nativeGlyph, at placement, skip map[int]bool) (found int, same bool) {
	point := at.apply(glyph.Baseline)
	found = -1
	best := math.Inf(1)
	key := bucket(point[0], point[1])
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for _, index := range p.buckets[[2]int{key[0] + dx, key[1] + dy}] {
				if skip[index] {
					continue
				}
				if math.Abs(p.page.X[index]-point[0]) > cellTolerance || math.Abs(p.page.Y[index]-point[1]) > cellTolerance {
					continue
				}
				spelled := glyph.Text == "" || p.text[index] == glyph.Text
				distance := math.Hypot(p.page.X[index]-point[0], p.page.Y[index]-point[1])
				if found < 0 || (spelled && !same) || (spelled == same && distance < best) {
					found, same, best = index, spelled, distance
				}
			}
		}
	}
	return found, same
}

// spelling lists the owners' glyphs whose text is the glyph's (every glyph for one without text),
// but those skip holds back: a glyph already bound to another cell.
func (p *positionIndex) spelling(glyph nativeGlyph, skip map[int]bool) []int {
	var list []int
	for index := range p.page.X {
		if skip[index] {
			continue
		}
		if glyph.Text == "" || p.text[index] == glyph.Text {
			list = append(list, index)
		}
	}
	return list
}

// nearest orders the owners' glyphs by their distance from a point: a fallback run is tried from
// the occurrence nearest where the fragment's own placement put it, not from any occurrence of its
// text (a styled "CP" run would otherwise take another "CP" on the page: papers4 1003).
func (p *positionIndex) nearest(indices []int, point [2]float64) {
	sort.Slice(indices, func(a, b int) bool {
		da := math.Hypot(p.page.X[indices[a]]-point[0], p.page.Y[indices[a]]-point[1])
		db := math.Hypot(p.page.X[indices[b]]-point[0], p.page.Y[indices[b]]-point[1])
		return da < db
	})
}

// placement is where the fragment lies on the page: the offset that puts one of its first glyphs
// on an owners' glyph of its text, else the scale and offset that put one of its first and one of
// its last glyphs on two such glyphs, under which nearly all of its glyphs meet owners' glyphs of
// their text; ok is false when there is none. near, when given, is where the fragment's own
// placement put it: a run's candidates are tried from the one nearest that.
func (p *positionIndex) placement(glyphs []nativeGlyph, skip map[int]bool, near *placement) (placement, bool) {
	for quarters := range 4 {
		at, ok := p.placeTurned(glyphs, quarters, skip, near)
		if ok {
			at.turn = quarters
			return at, true
		}
	}
	return placement{}, false
}

// placeTurned is placement for the fragment's glyphs turned by quarters (a table set sideways on the
// page, as the owners' positions have it).
func (p *positionIndex) placeTurned(fragment []nativeGlyph, quarters int, skip map[int]bool, near *placement) (placement, bool) {
	glyphs := make([]nativeGlyph, len(fragment))
	for index, glyph := range fragment {
		glyph.Baseline = turned(glyph.Baseline, quarters)
		glyphs[index] = glyph
	}
	search := &placementSearch{index: p, glyphs: glyphs, skip: skip, tried: map[placement]bool{}}
	first := glyphs[:min(anchors, len(glyphs))]
	for index, anchor := range first {
		candidates := p.spelling(anchor, skip)
		if near != nil {
			p.nearest(candidates, near.apply(fragment[index].Baseline))
		}
		for _, at := range candidates {
			if search.try(placement{scale: [2]float64{1, 1}, offset: p.offset(anchor, at, [2]float64{1, 1})}) {
				return search.best, true
			}
		}
	}
	last := glyphs[max(len(glyphs)-anchors, 0):]
	for _, from := range first {
		for _, to := range last {
			if search.scaled(from, to, skip) {
				return search.best, true
			}
		}
	}
	return placement{}, false
}

// offset is the offset that puts glyph, scaled, on the owners' glyph at.
func (p *positionIndex) offset(glyph nativeGlyph, at int, scale [2]float64) [2]float64 {
	return [2]float64{round2(p.page.X[at] - float64(glyph.Baseline[0]*scale[0])),
		round2(p.page.Y[at] - float64(glyph.Baseline[1]*scale[1]))}
}

func round2(value float64) float64 { return math.Round(value*100) / 100 }

// placementSearch tries placements of a fragment's glyphs once each.
type placementSearch struct {
	index  *positionIndex
	glyphs []nativeGlyph
	skip   map[int]bool
	tried  map[placement]bool
	best   placement
}

// try reports whether nearly all the glyphs meet owners' glyphs of their text under at.
func (s *placementSearch) try(at placement) bool {
	if s.tried[at] {
		return false
	}
	s.tried[at] = true
	if !s.index.covers(s.glyphs, at, s.skip) {
		return false
	}
	s.best = at
	return true
}

// scaled tries the scales that put from and to on two owners' glyphs of their text. A run lies on
// one baseline (a cell's row): its span has no y, and the table's whole scale is the x one, as a
// \resizebox scales both axes alike.
func (s *placementSearch) scaled(from, to nativeGlyph, skip map[int]bool) bool {
	span := [2]float64{to.Baseline[0] - from.Baseline[0], to.Baseline[1] - from.Baseline[1]}
	if math.Abs(span[0]) < 1 && math.Abs(span[1]) < 1 {
		return false
	}
	targets := s.index.spelling(to, skip)
	for _, at := range s.index.spelling(from, skip) {
		for _, onto := range targets {
			scale := [2]float64{1, 1}
			if math.Abs(span[0]) < 1 {
				scale[1] = (s.index.page.Y[onto] - s.index.page.Y[at]) / span[1]
				scale[0] = scale[1]
			} else if math.Abs(span[1]) < 1 {
				scale[0] = (s.index.page.X[onto] - s.index.page.X[at]) / span[0]
				scale[1] = scale[0]
			} else {
				scale = [2]float64{(s.index.page.X[onto] - s.index.page.X[at]) / span[0], (s.index.page.Y[onto] - s.index.page.Y[at]) / span[1]}
			}
			if !plausibleScale(scale[0]) || !plausibleScale(scale[1]) {
				continue
			}
			scale = [2]float64{math.Round(scale[0]*1e5) / 1e5, math.Round(scale[1]*1e5) / 1e5}
			if s.try(placement{scale: scale, offset: s.index.offset(from, at, scale)}) {
				return true
			}
		}
	}
	return false
}

func plausibleScale(scale float64) bool { return scale >= minimumScale && scale <= maximumScale }

// covers reports whether nearly all the glyphs lie on owners' glyphs of their text under at (by
// position alone, a table would also lie on another table of its columns on the page, paper 0668);
// it stops once too many miss, and first looks at a sample of them.
func (p *positionIndex) covers(glyphs []nativeGlyph, at placement, skip map[int]bool) bool {
	if len(glyphs) > 2*sampleSize {
		sampleMisses := 0
		for step := range sampleSize {
			if _, same := p.find(glyphs[step*len(glyphs)/sampleSize], at, skip); !same {
				sampleMisses++
			}
		}
		if sampleMisses > sampleSize/2-1 {
			return false
		}
	}
	misses := 0
	allowed := float64(len(glyphs)) * (1 - minimumCellMatch)
	for _, glyph := range glyphs {
		if _, same := p.find(glyph, at, skip); same {
			continue
		}
		misses++
		if float64(misses) > allowed {
			return false
		}
	}
	return true
}

// pictureCell reports whether a cell's content is drawn pictures only: what TeX recorded in the
// cell is the pictures' drawing, not the cell's text, and the Figures hold it (a pgfplots plot's
// ticks and exponents, papers4 1081), so the cell's geometry must not place any glyph for it. A
// run of picture glyphs, whose owners TeX does not know, otherwise took the same text elsewhere
// on the page (that paper's page number) for the cell's.
func pictureCell(cell tableCell) bool {
	if len(cell.Content) == 0 {
		return false
	}
	for _, item := range cell.Content {
		if item.Type != "image" {
			return false
		}
	}
	return true
}

// cellLeaf is the cell a painted glyph lies in when the owners gave it the block that holds the
// cell's table, or no node (a footnote mark in a cell has no flow item: paper 0565's authors);
// "" otherwise.
func (t *tagger) cellLeaf(cells map[int]string, pairs map[int]int, index int, node string) string {
	at, matched := pairs[index]
	if !matched {
		return ""
	}
	if cell := cells[at]; cell != "" && (node == "" || t.builder.cutBlock[cell] == node) {
		return cell
	}
	return ""
}
