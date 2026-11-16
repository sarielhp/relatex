package pdftag

import (
	"slices"
)

// The text runs of a block cut at its tables (a11y-106). The owners name the block, not its runs:
// TeX shipped the panel labels of papers5 1333 ("(a) Ovarian", then a table, "(b) Pancreatic", …)
// and the caption, table and note of a deluxetable (papers2 0352) as one paragraph's flow items. Its
// table cells are placed by the tables' geometry (cells.go); the rest of its glyphs were left to the
// text alignment, which looked for each label anywhere and took the "(" of the next one or lost a
// label to another paragraph that spelled it. TeX ships a block's content in order, so its glyphs
// that no cell took spell its text runs one after the other: they are shared out in TeX's order, each
// glyph to the run whose text it continues.

// runSkip is how many characters of a run a glyph may skip to continue it (a character the layout
// did not paint, such as an accent TeX built from two glyphs).
const runSkip = 4

// runCursor is where the sharing out of one block stands: its run and the characters taken of it.
type runCursor struct {
	runs  []string
	texts [][]rune
	run   int
	at    int
}

// runTexts are the text runs' characters, as the text check normalizes them.
func (t *tagger) runTexts(runs []string) [][]rune {
	texts := make([][]rune, len(runs))
	for index, leaf := range runs {
		texts[index] = []rune(t.leafText[leaf].text)
	}
	return texts
}

// shareRuns gives the open glyphs of a page that the owners give a cut block (blocks) the block's text
// run they continue, in TeX's order (pairs: paint index to TeX glyph); cursors carry each block from
// page to page. A glyph that continues no run is left open.
func (t *tagger) shareRuns(cursors map[string]*runCursor, painted []paintedGlyph, pairs map[int]int,
	blocks, leaves []string, decided []bool) {
	var order []int
	for index, block := range blocks {
		if block != "" && !decided[index] && len(t.builder.textRuns[block]) > 0 {
			order = append(order, index)
		}
	}
	slices.SortFunc(order, func(a, b int) int { return pairs[a] - pairs[b] })
	for _, index := range order {
		block := blocks[index]
		cursor := cursors[block]
		if cursor == nil {
			runs := t.builder.textRuns[block]
			cursor = &runCursor{runs: runs, texts: t.runTexts(runs)}
			cursors[block] = cursor
		}
		if leaf := cursor.take([]rune(withoutHyphens(string(normalized(painted[index].text))))); leaf != "" {
			leaves[index], decided[index] = leaf, true
		}
	}
}

// take moves the cursor past a glyph's characters and returns the run they continue, "" for none
// (a hyphen, which the text check drops, continues the current run):
// the current run where it goes on with them (within runSkip characters), else the first later run
// that begins with them.
func (c *runCursor) take(glyph []rune) string {
	if len(glyph) == 0 {
		return c.runs[c.run]
	}
	texts := c.texts
	current := texts[c.run]
	for skip := 0; skip <= runSkip && c.at+skip+len(glyph) <= len(current); skip++ {
		if slices.Equal(current[c.at+skip:c.at+skip+len(glyph)], glyph) {
			c.at += skip + len(glyph)
			return c.runs[c.run]
		}
	}
	for next := c.run + 1; next < len(texts); next++ {
		if len(texts[next]) >= len(glyph) && slices.Equal(texts[next][:len(glyph)], glyph) {
			c.run, c.at = next, len(glyph)
			return c.runs[c.run]
		}
	}
	return ""
}
