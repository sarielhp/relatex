package pdftag

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"slices"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// The binding by the core's glyph owners (decision 105 item 2): every glyph TeX shipped carries the
// node it belongs to (glyphowner); each painted glyph of the layout is matched with TeX's glyph at
// its baseline origin, in order, and takes that glyph's node. The text alignment (binding.go) runs
// beside it as the check, and binds only what the owners leave open: glyphs TeX did not ship (an
// included figure's text) and the glyphs of a node that is no structure leaf of its own (a block cut
// at its tables).

// Owner matching bounds: how far, in bp, TeX's baseline origin may lie from the layout's, and how
// many glyphs either side may skip to find the next match (a form's glyphs TeX never saw; glyphs of a
// picture the layout leaves out).
const (
	ownerTolerance = 0.5
	paintSkip      = 600
	texSkip        = 80
)

// OwnerCheck reports the binding by owners against the text alignment.
type OwnerCheck struct {
	Source        string     `json:"source"` // "owners", or "alignment" when the export has none
	Glyphs        int        `json:"glyphs"` // painted glyphs of the layout
	Matched       int        `json:"matched"`
	Owned         int        `json:"owned"`
	Agree         int        `json:"agree"`
	Disagree      int        `json:"disagree"`
	OwnersOnly    int        `json:"owners_only"`
	AlignmentOnly int        `json:"alignment_only"`
	Filled        int        `json:"filled"`          // open glyphs between two of one leaf (a virtual font's ligature)
	Fallback      int        `json:"fallback"`        // glyphs the alignment bound where the owners could not
	Stray         int        `json:"stray,omitempty"` // alignment fallbacks dropped: pages away from the glyphs the owners gave their leaf
	Cells         TableCells `json:"table_cells"`     // table glyphs bound to their cell by the table geometry
	Samples       []string   `json:"disagreements,omitempty"`
}

// readOwners reads the export's glyph owners; nil when it has none.
func readOwners(output string) (*glyphowner.Owners, error) {
	owners, err := glyphowner.Read(output)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return owners, err
}

// ownerMatcher matches one page's TeX glyphs with its painted glyphs.
type ownerMatcher struct {
	page    glyphowner.Page
	painted []paintedGlyph
	frame   pageFrame
	unknown map[int]bool
	// positions are the painted glyphs by positionKey, built when farPaint first needs them.
	positions map[[2]int][]int
}

// matches reports whether TeX's glyph i is the painted glyph j: at its position, or, where TeX does
// not know the position, with its text or still at TeX's position. A box with a literal is flagged
// whole, though most literals (a colour, a Beamer block's background) move nothing: there a glyph
// the font spells otherwise than TeX (slides 0027's ℕ, painted as U+FFFD) was matched by its text
// alone, failed, and took its paragraph's leaf instead of its formula's (decision 121, a11y-102).
func (m *ownerMatcher) matches(i, j int) bool {
	glyph := m.painted[j]
	if glyph.text == "" && glyph.baseline == [2]float64{} {
		return false
	}
	if m.unknown[i] && glyph.text != "" && slices.Equal(ownerText(glyph.text), ownerText(m.page.Text[i])) {
		return true
	}
	x, y := m.frame.tex(glyph.baseline[0], glyph.baseline[1])
	return math.Abs(x-m.page.X[i]) <= ownerTolerance && math.Abs(y-m.page.Y[i]) <= ownerTolerance
}

// ownerText is a glyph's text as the owners compare it where TeX does not know the position: the
// alignment's, without variation selectors. The layout spells a size variant of a delimiter with
// one (cmex's parenleftbig is "(" with U+FE01, glyphnames); TeX's character is the plain "(". In a
// box TeX placed later (a tcolorbox, papers2 0236) every glyph is matched by its text, so a large
// delimiter stopped the matching and the rest of the page resumed at a later "(".
func ownerText(text string) []rune {
	return slices.DeleteFunc(normalized(text), func(r rune) bool { return r >= 0xFE00 && r <= 0xFE0F })
}

// match pairs the page's glyphs in order; the result maps paint indices to TeX glyph indices.
func (m *ownerMatcher) match() map[int]int {
	pairs := map[int]int{}
	i, j := 0, 0
	for i < len(m.page.X) && j < len(m.painted) {
		if m.matches(i, j) {
			pairs[j] = i
			i, j = i+1, j+1
			continue
		}
		if k := m.skippedRun(i, j); k > 0 {
			i, j = i+k, j+k
			continue
		}
		if k := m.nextPaint(i, j); k >= 0 {
			j = k
			continue
		}
		if k := m.nextTeX(i, j); k >= 0 {
			i = k
			continue
		}
		if k := m.farPaint(i, j+paintSkip); k >= 0 {
			j = k
			continue
		}
		i, j = i+1, j+1
	}
	m.accentBases(pairs)
	return pairs
}

// accentBases pairs the painted base glyph of a TeX accent with the accent's owner. TeX's \accent
// paints the accent first and the base over it, while the owners record the composed letter alone,
// at the accent's origin (papers 0069: "Smolčić" reads "Smolˇci´", its last "c" unbindable and set
// as an artifact). When the owner glyph paired with a lone accent glyph is a composed letter whose
// base is the glyph painted over the accent, that glyph takes the accent's owner too.
func (m *ownerMatcher) accentBases(pairs map[int]int) {
	added := map[int]int{}
	for index, at := range pairs {
		if index+1 >= len(m.painted) || accentOver(m.painted, index) == 0 {
			continue
		}
		if _, taken := pairs[index+1]; taken {
			continue
		}
		base, mark := composedLetter(string(ownerText(m.page.Text[at])))
		if base == "" || mark != accentMark(m.painted[index].text) {
			continue
		}
		if base != accentBase(string(normalized(m.painted[index+1].text))) {
			continue
		}
		added[index+1] = at
	}
	for index, at := range added {
		if _, taken := pairs[index]; !taken {
			pairs[index] = at
		}
	}
}

// composedLetter is a glyph's text as the base letter and the one combining mark it composes; ""
// when it is not a composed letter.
func composedLetter(text string) (string, rune) {
	runes := []rune(norm.NFD.String(text))
	if len(runes) != 2 || unicode.Is(unicode.Mn, runes[0]) || !unicode.Is(unicode.Mn, runes[1]) {
		return "", 0
	}
	return string(runes[0]), runes[1]
}

// resumeRun is how many glyphs are looked at where the matching resumes after a skip: one letter
// matched by its text alone would resume at the wrong place, so the first must match and one of the
// next ones too (unless the page ends).
const resumeRun = 3

// matchesRun reports whether the matching can resume at TeX's glyph i and the painted glyph j.
func (m *ownerMatcher) matchesRun(i, j int) bool {
	if !m.matches(i, j) {
		return false
	}
	looked := false
	for d := 1; d < resumeRun && i+d < len(m.page.X) && j+d < len(m.painted); d++ {
		if m.matches(i+d, j+d) {
			return true
		}
		looked = true
	}
	return !looked
}

// skippedRun is the length of a run of paint indices the layout skipped (no text, no position)
// facing as many TeX glyphs whose position TeX does not know either, when the glyphs after both runs
// match; 0 otherwise. Neither side can be compared there, and the search for the next match would
// pair the unknown TeX glyphs by their text alone with a later glyph that spells the same: papers2
// 0236's figure labels (r₂, r₄, r₃, which the layout skipped) took the r₂ of the caption below, and
// the caption's first line was left to the alignment.
func (m *ownerMatcher) skippedRun(i, j int) int {
	k := 0
	for i+k < len(m.page.X) && j+k < len(m.painted) && m.unknown[i+k] && m.painted[j+k].unknown {
		k++
	}
	if k == 0 || i+k >= len(m.page.X) || j+k >= len(m.painted) || !m.matchesRun(i+k, j+k) {
		return 0
	}
	return k
}

func (m *ownerMatcher) nextPaint(i, j int) int {
	for k := j + 1; k < min(len(m.painted), j+paintSkip); k++ {
		if m.matchesRun(i, k) {
			return k
		}
	}
	return -1
}

// farPaint is the first painted glyph from index from on at TeX's glyph i's position where the matching
// can resume; -1 when there is none or TeX does not know the position (decision 121). An included
// figure paints its own text, which TeX never shipped, and may paint more than paintSkip glyphs
// (papers5 1438 page 7: a float's plots, 800 paint indices before its caption): the window ended
// inside the figure, the matching stepped through it pairing nothing, and the caption and the
// paragraphs after it lost their owners to the TeX glyphs it stepped past. Positions are looked up,
// so a page's matching stays linear.
func (m *ownerMatcher) farPaint(i, from int) int {
	if m.unknown[i] || from >= len(m.painted) {
		return -1
	}
	if m.positions == nil {
		m.positions = map[[2]int][]int{}
		for k, glyph := range m.painted {
			if glyph.text == "" && glyph.baseline == [2]float64{} {
				continue
			}
			key := positionKey(m.frame.tex(glyph.baseline[0], glyph.baseline[1]))
			m.positions[key] = append(m.positions[key], k)
		}
	}
	key := positionKey(m.page.X[i], m.page.Y[i])
	best := -1
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for _, k := range m.positions[[2]int{key[0] + dx, key[1] + dy}] {
				if k >= from && (best < 0 || k < best) && m.matchesRun(i, k) {
					best = k
				}
			}
		}
	}
	return best
}

// positionKey is the 1 bp grid cell of a position: a glyph within ownerTolerance of it lies in that
// cell or a neighbour.
func positionKey(x, y float64) [2]int {
	return [2]int{int(math.Floor(x)), int(math.Floor(y))}
}

func (m *ownerMatcher) nextTeX(i, j int) int {
	for k := i + 1; k < min(len(m.page.X), i+texSkip); k++ {
		if m.matchesRun(k, j) {
			return k
		}
	}
	return -1
}

// pageFrames are the pages' frames by page number.
func (t *tagger) pageFrames() map[int]pageFrame {
	frames := map[int]pageFrame{}
	for index, p := range t.file.pages() {
		frames[index+1] = t.file.frameOf(p)
	}
	return frames
}

// bindOwners replaces the alignment's binding by the owners', keeping the alignment where the owners
// leave a glyph open, and reports how the two agree.
func (t *tagger) bindOwners(owners *glyphowner.Owners) OwnerCheck {
	check := OwnerCheck{Source: "owners"}
	aligned := t.binding.leafOf
	bound := map[glyphRef]string{}
	pages := map[int]glyphowner.Page{}
	for _, p := range owners.Pages {
		pages[p.Number] = p
	}
	cells := t.tableCells(owners, &check.Cells)
	cuts := map[string]bool{}
	for _, block := range t.builder.cutBlock {
		cuts[block] = true
	}
	frames := t.pageFrames()
	spans := pageSpans{}
	var fallbacks []glyphRef
	fallbackLeaf := map[glyphRef]string{}
	placed := map[string]int{} // characters the tables' geometry bound, by cell
	t.placedOwners = map[glyphRef]string{}
	cursors := map[string]*runCursor{}
	for _, number := range slices.Sorted(maps.Keys(t.painted)) {
		painted, page := t.painted[number], pages[number]
		matcher := &ownerMatcher{page: page, painted: painted, frame: frames[number], unknown: map[int]bool{}}
		for _, index := range page.Unknown {
			matcher.unknown[index] = true
		}
		pairs := matcher.match()
		leaves, decisions, blocks := make([]string, len(painted)), make([]bool, len(painted)), make([]string, len(painted))
		for index := range painted {
			var node string
			leaves[index], decisions[index], node = t.ownerLeaf(owners, page, pairs, index, &check)
			if at, ok := pairs[index]; ok && leaves[index] != "" && !matcher.unknown[at] {
				t.placedOwners[glyphRef{number, index}] = leaves[index]
			}
			if cell := t.cellLeaf(cells[number], pairs, index, node); cell != "" {
				leaves[index], decisions[index] = cell, true
				if t.builder.artifacts[cell] {
					leaves[index] = ""
				}
				check.Cells.Glyphs++
				placed[cell] += len(normalized(painted[index].text))
			} else if cuts[node] {
				blocks[index] = node
			}
		}
		t.shareRuns(cursors, painted, pairs, blocks, leaves, decisions)
		filled := completeLigatures(matcher, pairs, leaves, decisions)
		check.Filled += len(filled)
		t.placeLigatures(number, leaves, filled)
		check.Filled += fillGaps(leaves, decisions)
		open := t.openLeaves(number, blocks, aligned)
		for index := range painted {
			ref := glyphRef{number, index}
			check.Glyphs++
			leaf, decided := leaves[index], decisions[index]
			if !decided {
				leaf = open[index]
			}
			check.compare(ref, leaf, aligned[ref], decided)
			if leaf == "" {
				continue
			}
			if decided {
				bound[ref] = leaf
				spans.add(leaf, number)
			} else {
				fallbacks = append(fallbacks, ref)
				fallbackLeaf[ref] = leaf
			}
		}
	}
	for _, ref := range fallbacks {
		if leaf := fallbackLeaf[ref]; spans.near(leaf, ref.page) {
			bound[ref] = leaf
			check.Fallback++
		} else {
			check.Stray++
		}
	}
	check.Fallback -= t.dropSurplus(bound, fallbacks, placed)
	t.binding.leafOf = bound
	return check
}

// dropSurplus unbinds the glyphs the alignment gave a cell whose own glyphs the tables' geometry
// already bound in full: the alignment, finding a word anywhere, took the tail of another cell's
// text ("27.5 min" for the cell "5min": papers3 0829, papers 0166) and the cell read back twice. It
// returns how many glyphs it unbound.
func (t *tagger) dropSurplus(bound map[glyphRef]string, fallback []glyphRef, placed map[string]int) int {
	dropped := 0
	for _, ref := range fallback {
		leaf := bound[ref]
		if have := placed[leaf]; have > 0 && have >= len([]rune(t.leafText[leaf].text)) {
			delete(bound, ref)
			dropped++
		}
	}
	return dropped
}

// ownerLeaf is the structure leaf the owners give a painted glyph; decided is false when they leave
// it open (no TeX glyph matched, or a node that is no leaf, which is returned as the node).
func (t *tagger) ownerLeaf(owners *glyphowner.Owners, page glyphowner.Page, pairs map[int]int, index int,
	check *OwnerCheck) (leaf string, decided bool, node string) {
	at, matched := pairs[index]
	if !matched {
		return "", false, ""
	}
	check.Matched++
	node = t.builder.titleLeaf(owners.Owner(page, at))
	if node == "" {
		return "", true, ""
	}
	check.Owned++
	if _, isLeaf := t.builder.leaves[node]; isLeaf {
		return node, true, node
	}
	return "", false, node
}

// openLeaves are the leaves of a page's glyphs the owners leave open: the alignment's, except that a
// glyph the owners give a block cut at its tables (blocks: that paragraph, whose runs and cells are
// the leaves) stays in its block. The alignment looks for the text anywhere and put a table's
// figures in a formula or a paragraph that spelled the same (papers 0052, 0113, 0188); such a glyph
// takes the leaf of the nearest glyph of its block the alignment placed in the block, the previous
// one first (a table paints cell by cell), and stays unbound when there is none. A glyph the
// alignment left unbound stays so (text no cell of the model holds). A table's glyphs come here
// only when its geometry did not place them in their cells (tableCells): a table the model holds
// no native fragments of, or one whose fragment met no placement.
func (t *tagger) openLeaves(number int, blocks []string, aligned map[glyphRef]string) []string {
	leaves := make([]string, len(blocks))
	for index, block := range blocks {
		leaves[index] = aligned[glyphRef{number, index}]
		if block != "" && t.builder.cutBlock[leaves[index]] != block {
			leaves[index] = ""
		}
	}
	placed := slices.Clone(leaves)
	for index, block := range blocks {
		if block != "" && leaves[index] == "" && aligned[glyphRef{number, index}] != "" {
			leaves[index] = nearestInBlock(placed, blocks, index)
		}
	}
	return leaves
}

// nearestInBlock is the leaf of the nearest placed glyph of glyph index's block: before it, else
// after it; "" when the block has none.
func nearestInBlock(placed, blocks []string, index int) string {
	for before := index - 1; before >= 0; before-- {
		if blocks[before] == blocks[index] && placed[before] != "" {
			return placed[before]
		}
	}
	for after := index + 1; after < len(blocks); after++ {
		if blocks[after] == blocks[index] && placed[after] != "" {
			return placed[after]
		}
	}
	return ""
}

// completeLigatures gives the painted glyphs a ligature leaves after its first piece the leaf of that
// piece. TeX ships a virtual font's ff as one glyph whose text is "ff"; the PDF paints f and f, and only
// the first lies at the TeX glyph's origin. Where the ligature ends a word at a cell's edge, the other
// piece lies between two leaves and fillGaps cannot tell whose it is (papers3 0727, "libtiff"). It
// returns the indices of the glyphs it gave, which the drawn-picture check needs: a piece the owners
// placed by their text stays its leaf's even where a picture's box covers it (papers6 1124).
func completeLigatures(m *ownerMatcher, pairs map[int]int, leaves []string, decided []bool) []int {
	var filled []int
	for index := range leaves {
		at, matched := pairs[index]
		if !matched || m.unknown[at] || leaves[index] == "" || !decided[index] {
			continue
		}
		rest := ownerText(m.page.Text[at])
		piece := ownerText(m.painted[index].text)
		if len(piece) == 0 || len(piece) >= len(rest) || !slices.Equal(rest[:len(piece)], piece) {
			continue
		}
		rest = rest[len(piece):]
		for next := index + 1; len(rest) > 0 && next < len(leaves); next++ {
			text := ownerText(m.painted[next].text)
			if _, taken := pairs[next]; taken || decided[next] || len(text) == 0 || len(text) > len(rest) || !slices.Equal(rest[:len(text)], text) {
				break
			}
			leaves[next], decided[next] = leaves[index], true
			filled = append(filled, next)
			rest = rest[len(text):]
		}
	}
	return filled
}

// placeLigatures records the glyphs completeLigatures gave as their leaf's, so that a drawn picture's
// box does not take them: the second f of a ligature can lie inside the box though the owners placed
// it by the leaf's text (papers6 1124).
func (t *tagger) placeLigatures(number int, leaves []string, filled []int) {
	for _, index := range filled {
		t.placedOwners[glyphRef{number, index}] = leaves[index]
	}
}

// gapLength bounds the painted glyphs TeX shipped as fewer (a virtual font's ffi painted as f and
// fi) that take the leaf of the matched glyphs around them.
const gapLength = 4

// fillGaps gives a short run of open glyphs between two glyphs of one leaf that leaf; it returns how
// many it gave.
func fillGaps(leaves []string, decided []bool) int {
	filled := 0
	for start := 0; start < len(leaves); start++ {
		if decided[start] || start == 0 || !decided[start-1] || leaves[start-1] == "" {
			continue
		}
		end := start
		for end < len(leaves) && !decided[end] && end-start < gapLength {
			end++
		}
		if end == len(leaves) || !decided[end] || leaves[end] != leaves[start-1] {
			continue
		}
		for at := start; at < end; at++ {
			leaves[at], decided[at] = leaves[start-1], true
			filled++
		}
	}
	return filled
}

// boundFormulas counts the formulas with glyphs bound to them.
func (t *tagger) boundFormulas() int {
	seen := map[string]bool{}
	for _, leaf := range t.binding.leafOf {
		if t.roleOf(leaf) == "Formula" {
			seen[leaf] = true
		}
	}
	return len(seen)
}

// compare counts one glyph's binding against the alignment's.
func (c *OwnerCheck) compare(ref glyphRef, leaf, aligned string, decided bool) {
	switch {
	case !decided:
	case leaf == aligned && leaf != "":
		c.Agree++
	case leaf != "" && aligned == "":
		c.OwnersOnly++
	case leaf == "" && aligned != "":
		c.AlignmentOnly++
	case leaf != aligned:
		c.Disagree++
		if len(c.Samples) < 20 {
			c.Samples = append(c.Samples, fmt.Sprintf("page %d glyph %d: owners %s, alignment %s", ref.page, ref.index, leaf, aligned))
		}
	}
}

// strayPages is how many pages away from the glyphs the owners gave a leaf the alignment may still
// place one more of its glyphs (a paragraph continues on the next page).
const strayPages = 1

// pageSpans are the first and last page of the glyphs the owners gave each leaf.
type pageSpans map[string][2]int

func (s pageSpans) add(leaf string, page int) {
	span, ok := s[leaf]
	if !ok {
		s[leaf] = [2]int{page, page}
		return
	}
	s[leaf] = [2]int{min(span[0], page), max(span[1], page)}
}

// near reports whether the alignment may place a glyph of leaf on page: the owners gave the leaf no
// glyph at all (the alignment binds it alone), or one within strayPages of the page. The alignment
// searches the document for a word the leaf prints and found the same word in a figure or in the
// bibliography pages away (papers5 1438: a paragraph read its own words again from pages 7, 11 and 15).
func (s pageSpans) near(leaf string, page int) bool {
	span, ok := s[leaf]
	return !ok || (page >= span[0]-strayPages && page <= span[1]+strayPages)
}
