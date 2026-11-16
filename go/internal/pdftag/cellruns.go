package pdftag

import "math"

// A rich table fragment's glyphs are placed by the fragment's own geometry (cells.go). What a
// fragment's placement leaves over is placed by the cell that set the glyphs, then by the font run
// within the cell: a cell's runs do not always keep the fragment's placement. A styled run is set
// in another font whose metrics differ (papers4 1003's small capitals), a table's columns are
// stretched beyond the fragment's natural widths (papers2 0390), a run is set sideways (papers4
// 0942's "Scalars"), and a \resizebox scale is only found from a row's own span (papers5 1246).
//
// A run placed on its own still needs its text: a glyph only joins the owners' glyph that spells
// the same text. A single glyph never places a run on its own (any occurrence of its text would
// do): it is placed only where its cell's own placement, or the fragment's, puts it. A cell is
// tried on its own only when the fragment's placement bound none of its glyphs; a run whose glyphs
// partly matched keeps its placement for the rest.

// cellRun is a fragment's glyphs that share a placement: one cell's, or one font's run in a cell.
type cellRun struct {
	cell   int
	glyphs []nativeGlyph
	at     []int // indices into the fragment's glyphs
}

// fragmentCells groups a fragment's glyphs by the cell that set them.
func fragmentCells(glyphs []nativeGlyph) []cellRun {
	return groupGlyphs(glyphs, func(before, glyph nativeGlyph) bool {
		return *before.Cell == *glyph.Cell
	})
}

// fragmentFontRuns groups a fragment's glyphs into consecutive glyphs of one cell and one font.
func fragmentFontRuns(glyphs []nativeGlyph) []cellRun {
	return groupGlyphs(glyphs, func(before, glyph nativeGlyph) bool {
		return *before.Cell == *glyph.Cell && before.Font == glyph.Font
	})
}

// groupGlyphs is the runs of consecutive fragment glyphs whose same reports them as one run.
func groupGlyphs(glyphs []nativeGlyph, same func(before, glyph nativeGlyph) bool) []cellRun {
	var runs []cellRun
	for index, glyph := range glyphs {
		if index > 0 && same(glyphs[index-1], glyph) {
			run := &runs[len(runs)-1]
			run.glyphs = append(run.glyphs, glyph)
			run.at = append(run.at, index)
			continue
		}
		runs = append(runs, cellRun{cell: *glyph.Cell, glyphs: []nativeGlyph{glyph}, at: []int{index}})
	}
	return runs
}

// placeRuns places the glyphs a fragment's placement left over by their cell, then by their run.
// lostOnly takes only the runs whose glyphs the placement bound none of; near is where that
// placement put the fragment. A run with a span places first, so that its offset places its cell's
// glyphs set alone (a font of one glyph: papers2 0390's "()" after the formula's subscript, a ")"
// after "(gcm"); a run of one glyph has no span to place it by (any occurrence of its text would
// do), so it is placed where its cell's run put it, else where the fragment's placement did.
func placeRuns(placed map[int]string, matched []bool, runs []cellRun, page *positionIndex, cells map[int]string, lostOnly bool, near *placement) {
	furniture := unowned(page)
	cellAt := map[int]placement{}
	for _, run := range runs {
		if len(run.glyphs) < 2 || !wanted(matched, run.at, lostOnly) {
			continue
		}
		skip := skipOthers(placed, cells[run.cell], furniture)
		at, ok := page.placement(run.glyphs, skip, near)
		if !ok {
			continue
		}
		cellAt[run.cell] = at
		bindRun(placed, matched, run, at, skip, page, cells)
	}
	for _, run := range runs {
		if !wanted(matched, run.at, lostOnly) {
			continue
		}
		at, known := cellAt[run.cell]
		if !known {
			if near == nil {
				continue
			}
			at = *near
		}
		bindRun(placed, matched, run, at, skipOthers(placed, cells[run.cell], furniture), page, cells)
	}
}

// unowned is the owners' glyphs no node owns: the page's furniture (a Beamer head or footline, a
// running header, the page number). A cell's glyph is the text of a model node, so a run must never
// be placed on furniture: slides/0029's header cell read "proposizioneproposizion", its run having
// been placed on the frame's footline.
func unowned(page *positionIndex) map[int]bool {
	var skip map[int]bool
	for at, leaf := range page.page.Leaf {
		if leaf > 0 {
			continue
		}
		if skip == nil {
			skip = map[int]bool{}
		}
		skip[at] = true
	}
	return skip
}

// bindRun binds a run's unmatched glyphs where at puts them, each onto the owners' glyph of its
// text.
func bindRun(placed map[int]string, matched []bool, run cellRun, at placement, skip map[int]bool, page *positionIndex, cells map[int]string) {
	for offset, glyph := range run.glyphs {
		if matched[run.at[offset]] {
			continue
		}
		if found, same := page.find(glyph, at, skip); same {
			placed[found] = cells[run.cell]
			matched[run.at[offset]] = true
		}
	}
}

// wanted reports whether a run has glyphs to place: any of them, or all of them when lostOnly.
func wanted(matched []bool, at []int, lostOnly bool) bool {
	if !lostOnly {
		return anyUnmatched(matched, at)
	}
	for _, index := range at {
		if matched[index] {
			return false
		}
	}
	return true
}

// anyUnmatched reports whether one of a run's glyphs is still to be placed.
func anyUnmatched(matched []bool, at []int) bool {
	for _, index := range at {
		if !matched[index] {
			return true
		}
	}
	return false
}

// missedTolerance is how far, in bp, an owner-less glyph of the page may lie from where the
// fragment's own placement puts a glyph its passes missed: a raised note mark (papers6/0744's
// author-table stars) is set in a font whose metrics differ from the recorded baseline's.
const missedTolerance = 2.0

// bindMissed binds a fragment glyph the placement and run passes missed onto the page's own glyph
// of its spelling nearest where the placement puts it, when that glyph has no owner and no other
// cell has it, so cellLeaf decides it from the cell's leaf.
func bindMissed(placed map[int]string, matched []bool, glyphs []nativeGlyph, page *positionIndex, cells map[int]string, near *placement) {
	if near == nil {
		return
	}
	for offset, glyph := range glyphs {
		if matched[offset] || glyph.Text == "" {
			continue
		}
		point := near.apply(glyph.Baseline)
		var candidates []int
		for _, index := range page.spelling(glyph, nil) {
			if _, taken := placed[index]; !taken && page.ownerless(index) &&
				math.Abs(page.page.X[index]-point[0]) <= missedTolerance && math.Abs(page.page.Y[index]-point[1]) <= missedTolerance {
				candidates = append(candidates, index)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		page.nearest(candidates, point)
		placed[candidates[0]] = cells[*glyph.Cell]
		matched[offset] = true
	}
}

// ownerless reports whether no node owns the page's glyph.
func (p *positionIndex) ownerless(index int) bool {
	return index < len(p.page.Leaf) && p.page.Leaf[index] <= 0
}

// skipOthers is the owners' glyphs already bound to another cell, and the page's furniture: a run
// must not take a glyph that another cell's text has, so a table of equal cells keeps each row's own
// glyphs, nor one that no node owns.
func skipOthers(placed map[int]string, leaf string, furniture map[int]bool) map[int]bool {
	skip := make(map[int]bool, len(furniture))
	for index := range furniture {
		skip[index] = true
	}
	for index, other := range placed {
		if other == leaf {
			continue
		}
		skip[index] = true
	}
	return skip
}
