package pdftag

import (
	"slices"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// A table fragment whose positions met no placement is still TeX's own. placeFragment looks for an
// offset and scale (and a quarter turn) that bring the fragment's baselines onto the owners'
// glyphs, and a tabular whose labels are each turned on their side in their cell (papers3 0615's
// figure) has no single such placement, though the block's glyphs are the fragment's own. The
// fragment's glyphs and the owners' glyphs of the block are both TeX's, shipped in order, so where
// their text agrees glyph by glyph the fragment's cell is the owners' glyph's.

// sequencePlace maps a block's owners' glyph indices, in TeX's order, to the leaves of a fragment's
// cells, the fragment's glyphs in TeX's order: the leaf of each owners' glyph, nil when fewer than
// minimumCellMatch of the fragment's glyphs meet one (the text is not that of the block's glyphs).
func sequencePlace(glyphs []nativeGlyph, leaves []string, block []int, text []string) map[int]string {
	match, found := sequenceMatch(glyphs, block, text)
	if float64(found) < float64(len(glyphs))*minimumCellMatch {
		return nil
	}
	placed := map[int]string{}
	for index, at := range match {
		if at >= 0 {
			placed[at] = leaves[index]
		}
	}
	return placed
}

// sequenceMatch maps each of a fragment's glyphs, in TeX's order, to the index of the block's
// owners' glyph it spells (-1 when it spells none), and reports how many it matched.
func sequenceMatch(glyphs []nativeGlyph, block []int, text []string) ([]int, int) {
	match := make([]int, len(glyphs))
	for index := range match {
		match[index] = -1
	}
	next, found := 0, 0
	for index := 0; index < len(glyphs) && next < len(block); index++ {
		switch {
		case text[block[next]] == glyphs[index].Text:
			match[index] = block[next]
			found++
			next++
		case index+1 < len(glyphs) && text[block[next]] == glyphs[index+1].Text:
			// a fragment glyph the block's text does not spell (a ligature, a glyph of no cell)
		case next+1 < len(block) && text[block[next+1]] == glyphs[index].Text:
			next++
			index--
		}
	}
	return match, found
}

// sequenceFix is one fragment's cells that the fragment's own geometry left unplaced, kept until
// every fragment of the page has placed so that a correction frees no owner for a later fragment.
type sequenceFix struct {
	owners *glyphowner.Owners
	block  string
	glyphs []nativeGlyph
	cells  map[int]string
	page   *positionIndex
	lost   map[string]bool
}

// placeSequence corrects the glyphs of a fragment's cells that the fragment's own placement bound
// none of, where the block's owners' glyphs spell them in TeX's order, after the cell and run
// fallbacks have had their say (cellruns.go). A header cell turned at an angle placement cannot
// follow (60 degrees under adjustbox, papers5 1187) has no placement of its own, and its digits,
// split into runs by a citation's link, are placed by their text onto a neighbouring data cell's
// glyph, which then loses its own; the block's order is TeX's own and gives each glyph its cell.
// Where every lost cell's glyphs already read its text in the page's order, the sequence has
// nothing to correct (papers3 0615's repeated labels): it does not rebind glyphs merely to move
// them to the block's own occurrence, which would change how many the cells hold. Where the
// block's order would leave any selected cell with fewer glyphs than it had, the fallbacks'
// bindings stand: a correction never makes a cell lose glyphs.
func placeSequence(placed map[int]string, fix sequenceFix) {
	selected, base := sequenceSelection(placed, fix.lost)
	if len(selected) == 0 {
		return
	}
	match, found := sequenceMatch(fix.glyphs, blockGlyphs(fix.owners, fix.page.page, fix.block), fix.page.text)
	if float64(found) < float64(len(fix.glyphs))*minimumCellMatch {
		return
	}
	if sequenceReads(selected, base, fix) {
		// Every lost cell already reads its text: the fragment's own placement put its glyphs in
		// the block's order, and the sequence has nothing to correct (papers3 0615's figures).
		return
	}
	applySequence(placed, fix, match, selected, base)
}

// sequenceSelection are the cells the fallbacks bound glyphs for that the fragment's own placement
// left unplaced, and the owners each holds.
func sequenceSelection(placed map[int]string, lost map[string]bool) (map[string]bool, map[string][]int) {
	selected := map[string]bool{}
	base := map[string][]int{}
	for at, leaf := range placed {
		if !lost[leaf] {
			continue
		}
		selected[leaf] = true
		base[leaf] = append(base[leaf], at)
	}
	return selected, base
}

// sequenceReads reports whether every selected cell's glyphs already spell its text in the page's
// order: the cell then reads correctly however the fallbacks found its glyphs.
func sequenceReads(selected map[string]bool, base map[string][]int, fix sequenceFix) bool {
	texts := textsFor(fix.glyphs, fix.cells)
	for leaf := range selected {
		if !spellsCell(base[leaf], fix.page.text, texts[leaf]) {
			return false
		}
	}
	return true
}

// applySequence replaces the selected cells' bindings with the owners' glyphs of the block's order.
// Where that order spells less of a cell than the fallbacks bound (a short match), the cell would
// lose glyphs, so the original bindings are restored instead. A cell takes as many glyphs as the
// fallbacks bound for it, and the glyph that follows its last in the block's order: a fallback that
// put a cell's glyphs on a prose word of the same text (all but the small capital A of
// "Approximate", books/0006) bound one fewer than the cell has, and the glyph that ends the cell's
// run in the block's order was left out. A later glyph of the cell that the block spells elsewhere
// (the "3" of a formula "y(3)" that the cell's text does not hold) is not that glyph.
func applySequence(placed map[int]string, fix sequenceFix, match []int, selected map[string]bool, base map[string][]int) {
	original := map[int]string{}
	for at, leaf := range placed {
		if selected[leaf] {
			original[at] = leaf
			delete(placed, at)
		}
	}
	order := map[int]int{}
	for position, at := range blockGlyphs(fix.owners, fix.page.page, fix.block) {
		order[at] = position
	}
	rebound := map[string]int{}
	last := map[string]int{}
	for offset, at := range match {
		leaf := fix.cells[*fix.glyphs[offset].Cell]
		if at < 0 || !selected[leaf] {
			continue
		}
		if follows, seen := last[leaf]; rebound[leaf] >= len(base[leaf]) && (!seen || order[at] != follows+1) {
			continue
		}
		if other, taken := placed[at]; taken && other != leaf {
			continue
		}
		placed[at] = leaf
		rebound[leaf]++
		last[leaf] = order[at]
	}
	for leaf := range selected {
		if rebound[leaf] < len(base[leaf]) {
			for at, other := range placed {
				if selected[other] {
					delete(placed, at)
				}
			}
			for at, other := range original {
				placed[at] = other
			}
			return
		}
	}
}

// spellsCell reports whether owners, in the page's order, spell the cell's text: the cell then reads
// correctly already, however its glyphs were found.
func spellsCell(owners []int, text []string, cell string) bool {
	ordered := slices.Clone(owners)
	slices.Sort(ordered)
	spelled := ""
	for _, at := range ordered {
		spelled += text[at]
	}
	return spelled == cell
}

// textsFor are a fragment's cells' texts, each cell's glyphs in TeX's order.
func textsFor(glyphs []nativeGlyph, cells map[int]string) map[string]string {
	texts := map[string]string{}
	for _, glyph := range glyphs {
		texts[cells[*glyph.Cell]] += glyph.Text
	}
	return texts
}

// unplacedCells are the leaves of the cells the fragment's own placement bound none of its glyphs
// for, before the cell and run fallbacks.
func unplacedCells(matched []bool, glyphs []nativeGlyph, cells map[int]string) map[string]bool {
	lost := map[string]bool{}
	for _, glyph := range glyphs {
		lost[cells[*glyph.Cell]] = true
	}
	for offset, glyph := range glyphs {
		if matched[offset] {
			delete(lost, cells[*glyph.Cell])
		}
	}
	return lost
}

// blockGlyphs are the owners' glyph indices of a page that the owners give a block, in TeX's order.
func blockGlyphs(owners *glyphowner.Owners, page glyphowner.Page, block string) []int {
	var list []int
	for at := range page.X {
		if owners.Owner(page, at) == block {
			list = append(list, at)
		}
	}
	return list
}

// gridBlock is the block a grid's cells were cut from: the block of any of its cells, "" when the
// model cut no block at this grid.
func gridBlock(b *builder, cells map[int]string) string {
	for _, leaf := range cells {
		if block := b.cutBlock[leaf]; block != "" {
			return block
		}
	}
	return ""
}
