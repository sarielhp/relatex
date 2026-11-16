package pdftag

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablelayout"
)

// The structure tree from the model's tree (decision 105 item 3), and the pieces each block binds.

// selem is a structure element before it is written.
type selem struct {
	role      string
	leaf      string // the glyphs bound to this leaf are its marked content
	kids      []*selem
	alt       string
	long      string // a Figure's long description, its associated file (decision 110)
	mathml    string
	tex       string   // the formula's TeX, its second associated file (decision 141)
	artifacts []string // a table's cells longtable repeats on each page: no elements, their glyphs are Artifacts
	mcrs      []mcr
	objr      map[string]any // a Link's or Annot's object reference
	key       int            // its annotation's StructParent
	span      [2]int         // a table cell's column and row span
	scope     string         // a TH's /Scope (/Column or /Row)
	first     glyphRef       // where a Link lies in its host's text
	ref       *selem         // the element a TOCI refers to (/Ref)
}

// mcr is a marked-content reference: a page's MCID.
type mcr struct {
	page  int
	mcid  int
	first int // the first glyph ordinal it holds, for ordering
}

// containerRoles maps container node types to structure roles.
var containerRoles = map[string]string{
	"document": "Document", "section": "Sect", "unordered_list": "L", "ordered_list": "L",
	"list_item": "LI", "note": "FENote", "footnote": "FENote", "quote": "BlockQuote",
	"figure_float": "Aside", "table_float": "Aside", "float": "Aside",
}

// floatRoles: a float is an Aside, as LaTeX's own tagging makes it (latex-lab's float role), and its
// caption a Caption, the Aside's first or last kid (ISO 32005: Caption belongs in Aside, Figure,
// Table, L, Formula…, never in Document; Div is transparent, so a caption in a Div was Document's).
var floatRoles = map[string]bool{"Aside": true}

// leafRoles maps content-bearing node types to structure roles.
var leafRoles = map[string]string{"paragraph": "P", "float_caption": "Caption", "caption": "Caption"}

// bulletChars are the labels a Beamer item typesets as a formula.
var bulletChars = map[rune]bool{'▶': true, '•': true, '◆': true, '■': true, '▸': true, '►': true,
	'‣': true, '∙': true, '⋆': true, '★': true, '–': true, '-': true, '∗': true, '◦': true}

// builder makes the structure and the pieces from the document.
type builder struct {
	doc        *document
	leaves     map[string]*selem
	labels     map[string]bool      // math nodes that are list labels
	undescribe []string             // images without a description
	grids      []*tableGrid         // the tables, as their cells were built
	artifacts  map[string]bool      // the cells of repeated longtable rows (repeats.go)
	sources    map[string]string    // each image leaf's file in the source (FigureFonts)
	cutBlock   map[string]string    // the leaves of a block cut at its tables (runs, cells, their formulas): the block
	parents    map[string]string    // each node's parent
	textLabels map[string]itemLabel // the paragraphs that begin with their list item's label
	hosts      map[string]string    // each inline figure's host leaf
	textRuns   map[string][]string  // the text runs of a block cut at its tables, in order (no run-in title)
	heading    int                  // the level of the last heading built, for a run-in title's (runin.go)
	// splitBlocks are the blocks splitRuns has split, once each: a cut block shares its content items
	// with every run, so splitting a huge one again for each of its leaves took gigabytes (papers4 0950).
	splitBlocks map[string][]run
}

func newBuilder(doc *document) *builder {
	parents := map[string]string{}
	for id, n := range doc.Nodes {
		for _, child := range n.Children {
			parents[child] = id
		}
	}
	return &builder{doc: doc, leaves: map[string]*selem{}, labels: map[string]bool{}, cutBlock: map[string]string{}, artifacts: map[string]bool{},
		sources: map[string]string{},
		parents: parents, textLabels: map[string]itemLabel{}, hosts: map[string]string{}, textRuns: map[string][]string{},
		splitBlocks: map[string][]run{}}
}

func (b *builder) leaf(role, id string) *selem {
	element := &selem{role: role, leaf: id}
	b.leaves[id] = element
	return element
}

// build makes the elements of a node.
func (b *builder) build(id string) []*selem {
	n, ok := b.doc.Nodes[id]
	if !ok {
		return nil
	}
	switch n.Type {
	case "heading":
		b.heading = n.Semantics.Level
		return []*selem{b.inline(b.leaf(headingRole(n.Semantics.Level), id), n.Content)}
	case "display_math":
		return b.formulas(n.Content)
	case "list_item":
		return []*selem{b.listItem(n)}
	case "math", "math_row":
		return nil
	case "float_caption", "caption":
		if len(n.Content) == 0 {
			return []*selem{b.container("Caption", n)}
		}
	}
	if role, ok := containerRoles[n.Type]; ok {
		return []*selem{b.container(role, n)}
	}
	elements := b.runs(n)
	for _, child := range n.Children {
		if b.doc.Nodes[child].Type != "math" {
			elements = append(elements, b.build(child)...)
		}
	}
	return elements
}

// notes are the document's footnotes and endnotes as FENote elements, in note order, for the end of
// the Document: the model keeps them beside the tree (note_order), so the tree does not reach them,
// and their glyphs, which the core's owners give their paragraphs, were bound by the text
// alignment to whichever paragraph spelled the same words (papers 0112). A note whose paragraphs
// the tree already holds is left where it is. FENote may be a Document's kid (ISO 32005), and
// without /Ref on either side veraPDF's 8.2.5.14 holds.
func (b *builder) notes() []*selem {
	var notes []*selem
	for _, id := range b.doc.NoteOrder {
		n, ok := b.doc.Nodes[id]
		if !ok || slices.ContainsFunc(n.Children, func(child string) bool { return b.leaves[child] != nil }) {
			continue
		}
		element := b.container("FENote", n)
		if len(element.kids) > 0 {
			notes = append(notes, element)
		}
	}
	return notes
}

func headingRole(level int) string {
	return fmt.Sprintf("H%d", min(max(level, 1), 6))
}

func (b *builder) container(role string, n node) *selem {
	element := &selem{role: role}
	for _, child := range n.Children {
		element.kids = append(element.kids, b.build(child)...)
	}
	if floatRoles[role] {
		groupCaptions(element)
		captionAtEnd(element)
	}
	return element
}

// groupRole is the element that holds one caption of a float with several: a Sect, which veraPDF
// counts as a grouping element of its own (Part and Div are transparent: their captions were the
// Aside's again).
const groupRole = "Sect"

// groupCaptions gives each caption of a float with several (two figures side by side, each with its
// \caption: papers 0097, 0167, 1174) a Sect of its own with what it captions: ISO 32005 allows an
// Aside one Caption. Captions after their figures close a Sect; captions before them (the float
// begins with one) open it. What follows the last caption that closes a Sect stays the Aside's.
func groupCaptions(float *selem) {
	isCaption := func(kid *selem) bool { return kid.role == "Caption" }
	captions := 0
	for _, kid := range float.kids {
		if isCaption(kid) {
			captions++
		}
	}
	if captions < 2 {
		return
	}
	leading := isCaption(float.kids[0])
	var kids, group []*selem
	for _, kid := range float.kids {
		if leading && isCaption(kid) && len(group) > 0 {
			kids, group = append(kids, &selem{role: groupRole, kids: group}), nil
		}
		group = append(group, kid)
		if !leading && isCaption(kid) {
			kids, group = append(kids, &selem{role: groupRole, kids: group}), nil
		}
	}
	if leading && len(group) > 0 {
		group = []*selem{{role: groupRole, kids: group}}
	}
	float.kids = append(kids, group...)
}

// captionAtEnd moves a float's caption to its nearer end when the float holds more after it (a
// caption between two pictures): ISO 32005 wants it first or last.
func captionAtEnd(float *selem) {
	at := slices.IndexFunc(float.kids, func(kid *selem) bool { return kid.role == "Caption" })
	if at <= 0 || at == len(float.kids)-1 {
		return
	}
	caption := float.kids[at]
	kids := slices.Delete(slices.Clone(float.kids), at, at+1)
	if at < len(float.kids)/2 {
		float.kids = append([]*selem{caption}, kids...)
		return
	}
	float.kids = append(kids, caption)
}

// runs splits a block's content at its tables: paragraphs and tables in turn.
func (b *builder) runs(n node) []*selem {
	role := leafRoles[n.Type]
	if role == "" {
		role = "P"
	}
	var elements []*selem
	var texts []string
	runs := b.split(n)
	for index, run := range runs {
		if run.grid != nil {
			elements = append(elements, b.table(run.grid))
			continue
		}
		elements = append(elements, b.inline(b.leaf(b.runRole(role, run), runID(n, len(runs), index)), run.items))
		if !run.title {
			texts = append(texts, runID(n, len(runs), index))
		}
	}
	if len(elements) > 1 || (len(elements) == 1 && elements[0].leaf != n.ID) {
		for _, element := range elements {
			b.markCut(element, n.ID)
		}
		b.textRuns[n.ID] = texts
	}
	return elements
}

// markCut records the leaves under element as parts of the cut block.
func (b *builder) markCut(element *selem, block string) {
	if element.leaf != "" {
		b.cutBlock[element.leaf] = block
	}
	for _, id := range element.artifacts {
		b.cutBlock[id] = block
	}
	for _, kid := range element.kids {
		b.markCut(kid, block)
	}
}

// run is a stretch of a block's content: inline items, or one table.
type run struct {
	items []contentItem
	grid  *tableGrid
	title bool // a run-in heading's title
}

func splitRuns(n node) []run {
	runs, rest := titleRun(n)
	var current []contentItem
	for _, item := range rest {
		if item.Type == "table_grid" && item.Grid != nil {
			if len(current) > 0 {
				runs = append(runs, run{items: current})
			}
			runs, current = append(runs, run{grid: item.Grid}), nil
			continue
		}
		current = append(current, item)
	}
	if len(current) > 0 || len(runs) == 0 {
		runs = append(runs, run{items: current})
	}
	return runs
}

// split is a block's runs, split once: the caller keeps them (a second split copies the block's
// content items again), so a later change of the split must not reach a leaf already built.
func (b *builder) split(n node) []run {
	if runs, ok := b.splitBlocks[n.ID]; ok {
		return runs
	}
	runs := splitRuns(n)
	b.splitBlocks[n.ID] = runs
	return runs
}

// runID is a run's leaf: the block itself when it is one run (of count runs).
func runID(n node, count, index int) string {
	if count == 1 {
		return n.ID
	}
	return fmt.Sprintf("%s#r%d", n.ID, index)
}

// inline adds an element's inline formulas and figures as its kids.
func (b *builder) inline(element *selem, items []contentItem) *selem {
	for index, item := range items {
		item = markImage(item)
		switch item.Type {
		case "math":
			if b.labels[item.Node] {
				continue
			}
			element.kids = append(element.kids, b.formula(item.Node))
		case "image":
			if figure := b.figure(element.leaf, index, item); figure != nil {
				element.kids = append(element.kids, figure)
				b.hosts[figure.leaf] = element.leaf
			}
		}
	}
	return element
}

func (b *builder) formulas(items []contentItem) []*selem {
	var elements []*selem
	for _, item := range items {
		if item.Type == "math" {
			elements = append(elements, b.formula(item.Node))
		}
	}
	return elements
}

func (b *builder) formula(id string) *selem {
	n := b.doc.Nodes[id]
	element := b.leaf("Formula", id)
	element.alt, element.mathml, element.tex = formulaAlt(n.MathML), n.MathML, formulaTeX(n)
	return element
}

// formulaTeX is a formula's TeX as the HTML annotates it (mathexport.AnnotationTeX): the author's
// verified TeX, else the formula TeX the round trip proved; "" for neither.
func formulaTeX(n node) string {
	fields := rec.Map{}
	if n.TeX != nil {
		fields["tex"] = rec.Map{"status": n.TeX.Status, "text": n.TeX.Text}
	}
	if n.FormulaTeX != nil {
		formula := rec.Map{"text": n.FormulaTeX.Text}
		if n.FormulaTeX.Environment != "" {
			formula["environment"] = n.FormulaTeX.Environment
		}
		fields["formula_tex"] = formula
	}
	return mathexport.AnnotationTeX(fields)
}

// formulaAlt is a formula's linear text, as the HTML's alttext (decision 088); its characters
// when the MathML does not parse.
func formulaAlt(mathml string) (alt string) {
	defer func() {
		if recover() != nil {
			alt = string(mathChars(mathml))
		}
	}()
	return mathexport.AltText(mathexport.ParsePaper(mathml))
}

// markImage is the image a QED mark shows (decision 176), tagged as any image, its alt "End of proof"; any
// other item as it is.
func markImage(item contentItem) contentItem {
	if item.Type == "symbol" && item.Image != nil {
		return *item.Image
	}
	return item
}

// imageLeaf is an image's leaf id.
func imageLeaf(block string, index int, item contentItem) string {
	if item.ID != "" {
		return item.ID
	}
	return fmt.Sprintf("%s#i%d", block, index)
}

// figure is an image's Figure, nil for a decorative one (an artifact).
func (b *builder) figure(block string, index int, item contentItem) *selem {
	if item.Decorative {
		return nil
	}
	element := b.leaf("Figure", imageLeaf(block, index, item))
	// The short text is /Alt; a long description is the Figure's associated file (decision 110).
	element.alt, element.long = strings.TrimSpace(item.Alt), strings.TrimSpace(item.LongDescription)
	if element.alt == "" {
		element.alt, element.long = element.long, ""
	}
	if element.alt == "" {
		element.alt = "Image " + strings.TrimSuffix(filepath.Base(item.Source), filepath.Ext(item.Source))
		b.undescribe = append(b.undescribe, item.Source)
	}
	return element
}

// headerScopes are the PDF /Scope names of the page's header scopes.
var headerScopes = map[string]string{tablelayout.ScopeColumn: "/Column", tablelayout.ScopeRow: "/Row"}

// table is a table grid as Table, TR and TH/TD.
func (b *builder) table(grid *tableGrid) *selem {
	table := &selem{role: "Table"}
	b.grids = append(b.grids, grid)
	var row *selem
	last := -1
	// A grid may list a row's cells apart (a cell set after the next row's); a row is one TR.
	kept, dropped := withoutRepeats(grid)
	table.artifacts = dropped
	for _, id := range dropped {
		b.artifacts[id] = true
	}
	cells := compactRows(kept)
	scopes := tablelayout.HeaderScopes(grid.raw, nil)
	for _, cell := range cells {
		if row == nil || cell.Row != last {
			row, last = &selem{role: "TR"}, cell.Row
			table.kids = append(table.kids, row)
		}
		// The page's header cells (decisions 088, 111, 131): a TH with the scope its <th> has.
		role, scope := "TD", headerScopes[scopes[cell.ID]]
		if scope != "" {
			role = "TH"
		}
		element := b.inline(b.leaf(role, cell.ID), cell.Content)
		element.span, element.scope = [2]int{cell.Colspan, cell.Rowspan}, scope
		if role == "TH" && formulaOnly(element) {
			element.alt = formulaNames(element)
		}
		row.kids = append(row.kids, element)
	}
	padRows(table, cells)
	return table
}

// formulaOnly: the element's children are formulas only, and some (a header cell of formulas alone).
func formulaOnly(element *selem) bool {
	return len(element.kids) > 0 && !slices.ContainsFunc(element.kids, func(kid *selem) bool { return kid.role != "Formula" })
}

// formulaNames is the formulas' alternative texts in reading order, the TH's own /Alt as the page's
// <th> has its aria-label (decision 095, amendment 2).
func formulaNames(element *selem) string {
	names := []string{}
	for _, kid := range element.kids {
		if kid.alt != "" {
			names = append(names, kid.alt)
		}
	}
	return strings.Join(names, " ")
}

// compactRows sorts a grid's cells by row and column and drops the rows no cell sets: a row the
// spans above cover wholly (the second line of a \multirow cell) is no row of the structure, and
// veraPDF counts it as a row of no cells (8.2.5.26-3, 8.2.5.26-4, paper 1182). Rows are numbered
// from 0 without gaps and each row span covers the rows it still covers.
func compactRows(grid []tableCell) []tableCell {
	cells := slices.Clone(grid)
	slices.SortStableFunc(cells, func(a, b tableCell) int { return cmp.Or(a.Row-b.Row, a.Column-b.Column) })
	present := []int{}
	for _, cell := range cells {
		if len(present) == 0 || present[len(present)-1] != cell.Row {
			present = append(present, cell.Row)
		}
	}
	for index := range cells {
		first, _ := slices.BinarySearch(present, cells[index].Row)
		end, _ := slices.BinarySearch(present, cells[index].Row+max(cells[index].Rowspan, 1))
		cells[index].Row, cells[index].Rowspan = first, max(end-first, 1)
	}
	return cells
}

// padRows ends each row that covers fewer columns than the table (spans counted) with empty
// cells: PDF/UA-2 wants regular tables (8.2.5.26), and a grid may leave a row's last cells out.
func padRows(table *selem, cells []tableCell) {
	width, occupied := 0, map[int]int{}
	for _, cell := range cells {
		columns, lines := max(cell.Colspan, 1), max(cell.Rowspan, 1)
		for line := cell.Row; line < cell.Row+lines; line++ {
			occupied[line] += columns
			width = max(width, occupied[line])
		}
	}
	for index, row := range table.kids {
		for missing := width - occupied[index]; missing > 0; missing-- {
			row.kids = append(row.kids, &selem{role: "TD"})
		}
	}
}

// listItem is LI with Lbl (a leading bullet formula, as Beamer sets it, or the label its first
// paragraph begins with) and LBody.
func (b *builder) listItem(n node) *selem {
	item := &selem{role: "LI"}
	if label := b.bulletLabel(n); label != "" {
		b.labels[label] = true
		item.kids = append(item.kids, b.leaf("Lbl", label))
	} else if paragraph := b.textLabel(n); paragraph != "" {
		item.kids = append(item.kids, b.leaf("Lbl", labelLeaf(paragraph)))
	}
	body := &selem{role: "LBody"}
	for _, child := range n.Children {
		body.kids = append(body.kids, b.build(child)...)
	}
	item.kids = append(item.kids, body)
	return item
}

// bulletLabel is the math node a list item's first paragraph begins with when it is one bullet.
func (b *builder) bulletLabel(n node) string {
	if len(n.Children) == 0 {
		return ""
	}
	first := b.doc.Nodes[n.Children[0]]
	if len(first.Content) == 0 || first.Content[0].Type != "math" {
		return ""
	}
	chars := mathChars(b.doc.Nodes[first.Content[0].Node].MathML)
	if len(chars) == 1 && bulletChars[chars[0]] {
		return first.Content[0].Node
	}
	return ""
}

// pieces lists what each block binds, in reading order.
func (b *builder) pieces() []piece {
	var pieces []piece
	labels := b.noteLabels()
	for _, id := range b.doc.BlockOrder {
		n := b.doc.Nodes[id]
		begin := len(pieces)
		runs := b.split(n)
		if label := labels[id]; label != "" && len(runs) > 0 {
			pieces = append(pieces, piece{leaf: runID(n, len(runs), 0), kind: pieceText, text: label})
		}
		for index, r := range runs {
			if r.grid != nil {
				pieces = append(pieces, b.gridPieces(r.grid)...)
				continue
			}
			pieces = append(pieces, b.itemPieces(runID(n, len(runs), index), r.items)...)
		}
		if begin < len(pieces) {
			pieces[begin].first = true
		}
	}
	return pieces
}

// noteLabels are the notes' numbers by the block that typesets them: part of that block's text, as
// the glyphs show it.
func (b *builder) noteLabels() map[string]string {
	labels := map[string]string{}
	for _, id := range b.doc.NoteOrder {
		for _, label := range b.doc.Nodes[id].Labels {
			labels[label.Block] += label.Text
		}
	}
	return labels
}

func (b *builder) gridPieces(grid *tableGrid) []piece {
	var pieces []piece
	kept, _ := withoutRepeats(grid)
	for _, cell := range kept {
		pieces = append(pieces, b.itemPieces(cell.ID, cell.Content)...)
	}
	return pieces
}

// itemPieces are a leaf's pieces: its text, formulas and images.
func (b *builder) itemPieces(leaf string, items []contentItem) []piece {
	var pieces []piece
	for index, item := range items {
		item = markImage(item)
		switch item.Type {
		case "text", "reference", "note_reference", "note_number":
			text := piece{leaf: leaf, kind: pieceText, text: item.Text, caps: smallCaps(item.Style.Shape)}
			if index == 0 {
				pieces = append(pieces, b.labelPieces(leaf, text)...)
				continue
			}
			pieces = append(pieces, text)
		case "math":
			pieces = append(pieces, piece{leaf: item.Node, kind: pieceMath, chars: mathChars(b.doc.Nodes[item.Node].MathML)})
		case "image":
			id := imageLeaf(leaf, index, item)
			if item.Source != "" {
				b.sources[id] = item.Source
			}
			pieces = append(pieces, piece{leaf: id, kind: pieceImage,
				size: [2]float64{bigPoints(item.WidthSp), bigPoints(item.HeightSp)}})
		}
	}
	return pieces
}

// bigPoints converts scaled points to PDF points.
func bigPoints(sp float64) float64 { return sp / 65536 * 72 / 72.27 }
