package pdftag

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// The structure the model holds without naming it (decision 105, labels and navigation): a list
// item's printed label as Lbl, the document's title as Title, and a table of contents as TOC and
// TOCI. The model keeps an item's label as the first characters of its first paragraph, its title in
// the front matter beside the block that prints it, and a contents list as paragraphs that repeat the
// headings; route 1 (LaTeX's own tagging) tagged all three, and PDF/UA-2 readers navigate by them.

var (
	// bulletLabel opens an item of any list: a bullet, dash or star alone, as a word of its own.
	bulletLabel = regexp.MustCompile(`\A\s*[•·∙◦▪▸►‣⋆★∗–—*-](\s+|\z)`)
	// numberLabel opens an item of an ordered list: a number, roman numeral or letter, in
	// parentheses or before a period, parenthesis or colon ("1.", "(a)", "iv)", "3:").
	numberLabel = regexp.MustCompile(`\A\s*(\([0-9]+\)|\([ivxlcIVXLC]+\)|\([a-zA-Z]\)|([0-9]+|[ivxlcIVXLC]+|[a-zA-Z])[.):])(\s+|\z)`)
	// pageNumber ends a contents entry: its page, after leader dots or spaces.
	pageNumber = regexp.MustCompile(`[\s.·]+([0-9]+|[ivxlcdmIVXLCDM]+)\z`)
)

// itemLabel is the label an item's first paragraph begins with: the label's text and the bytes it
// takes of the paragraph's first text, spaces after it included.
type itemLabel struct {
	text string
	cut  int
}

// labelLeaf is the leaf of a paragraph's item label.
func labelLeaf(paragraph string) string { return paragraph + "#lbl" }

// textLabel finds the label an item's first paragraph begins with; "" when it has none. An ordered
// list's item may begin with a number, any item with a bullet. The paragraph must be one leaf (no
// table cuts it) and begin with text.
func (b *builder) textLabel(item node) string {
	if len(item.Children) == 0 {
		return ""
	}
	first := b.doc.Nodes[item.Children[0]]
	if first.Type != "paragraph" || len(first.Content) == 0 || first.Content[0].Type != "text" || len(splitRuns(first)) != 1 {
		return ""
	}
	text := first.Content[0].Text
	match := bulletLabel.FindStringIndex(text)
	if match == nil && b.doc.Nodes[b.parents[item.ID]].Type == "ordered_list" {
		match = numberLabel.FindStringIndex(text)
	}
	if match == nil {
		return ""
	}
	b.textLabels[first.ID] = itemLabel{text: strings.TrimSpace(text[:match[1]]), cut: match[1]}
	return first.ID
}

// labelPieces splits a labelled paragraph's first text into its label's piece and the rest.
func (b *builder) labelPieces(leaf string, p piece) []piece {
	label, ok := b.textLabels[leaf]
	if !ok {
		return []piece{p}
	}
	rest := p
	rest.text = p.text[label.cut:]
	return []piece{{leaf: labelLeaf(leaf), kind: pieceText, text: label.text, caps: p.caps}, rest}
}

// bindLabels gives each item label the first glyphs its paragraph's binding holds, in paint order,
// when they spell it: the core owns an item's label as its paragraph's text (TeX sets the label's
// box first on the item's first line). A paragraph whose first glyphs do not spell its label keeps
// them, and its label returns to its expected text. It reports whether any label returned.
func (t *tagger) bindLabels() bool {
	labelled := t.builder.textLabels
	refs := map[string][]glyphRef{}
	for ref, leaf := range t.binding.leafOf {
		if _, ok := labelled[leaf]; ok {
			refs[leaf] = append(refs[leaf], ref)
		}
	}
	returned := false
	for leaf, label := range labelled {
		held := refs[leaf]
		slices.SortFunc(held, func(a, b glyphRef) int { return cmp.Or(a.page-b.page, a.index-b.index) })
		take := t.spells(held, normalized(label.text))
		if take == 0 {
			delete(labelled, leaf)
			returned = true
			continue
		}
		for _, ref := range held[:take] {
			t.binding.leafOf[ref] = labelLeaf(leaf)
		}
	}
	return returned
}

// spells is how many of the glyphs, from the first, spell the label; 0 when they do not.
func (t *tagger) spells(glyphs []glyphRef, label []rune) int {
	var read []rune
	for index, ref := range glyphs {
		painted := t.painted[ref.page]
		if ref.index >= len(painted) {
			return 0
		}
		read = append(read, normalized(painted[ref.index].text)...)
		if len(read) >= len(label) {
			if slices.Equal(read, label) {
				return index + 1
			}
			return 0
		}
	}
	return 0
}

// referenceLabel opens a bibliography entry: its key or number in brackets ("[Gup00]", "[12]").
var referenceLabel = regexp.MustCompile(`\A\s*\[[^\]\s]{1,16}\](\s+|\z)`)

// referenceEntry is the label of a bibliography entry's paragraph: the bracketed label its text opens
// with, else, for a paragraph the model marks as an entry, its number in brackets, which the text
// does not hold (cut 0; bindLabels drops it unless the glyphs spell it). ok is false for another
// paragraph.
func (b *builder) referenceEntry(kid *selem) (label itemLabel, ok bool) {
	if kid.role != "P" || kid.leaf == "" {
		return itemLabel{}, false
	}
	n := b.doc.Nodes[kid.leaf]
	if len(n.Content) == 0 || n.Content[0].Type != "text" || len(splitRuns(n)) != 1 {
		return itemLabel{}, false
	}
	text := n.Content[0].Text
	if match := referenceLabel.FindStringIndex(text); match != nil {
		return itemLabel{text: strings.TrimSpace(text[:match[1]]), cut: match[1]}, true
	}
	if entry := n.Content[0].Entry; entry > 0 {
		return itemLabel{text: "[" + strconv.Itoa(entry) + "]"}, true
	}
	return itemLabel{}, false
}

// markReferences makes each run of bibliography entries (paragraphs the model marks as entries or
// that open with a bracketed label) an L, each paragraph an LI with its label as Lbl. A run is at
// least two paragraphs long.
func (b *builder) markReferences(element *selem) {
	label := b.referenceEntry
	for _, kid := range element.kids {
		b.markReferences(kid)
	}
	var out []*selem
	for start := 0; start < len(element.kids); {
		end := start
		for ; end < len(element.kids); end++ {
			if _, ok := label(element.kids[end]); !ok {
				break
			}
		}
		if end-start < 2 {
			out = append(out, element.kids[start])
			start++
			continue
		}
		list := &selem{role: "L"}
		for _, kid := range element.kids[start:end] {
			b.textLabels[kid.leaf], _ = label(kid)
			list.kids = append(list.kids, &selem{role: "LI", kids: []*selem{b.leaf("Lbl", labelLeaf(kid.leaf)),
				{role: "LBody", kids: []*selem{kid}}}})
		}
		out = append(out, list)
		start = end
	}
	element.kids = out
}

// markTitle makes the paragraph that prints the front matter's title, a kid of the Document before
// its first section, a Title holding it.
func (b *builder) markTitle(root *selem) {
	title, _ := b.doc.Frontmatter.Title.(string)
	want := letters(title)
	if len(want) == 0 {
		return
	}
	for index, kid := range root.kids {
		if kid.role != "P" && kid.role != "Title" {
			return
		}
		if kid.leaf == "" || !strings.HasPrefix(letters(b.leafText(kid.leaf)), want) {
			continue
		}
		if extra := len([]rune(letters(b.leafText(kid.leaf)))) - len([]rune(want)); extra <= titleMarks {
			root.kids[index] = &selem{role: "Title", kids: []*selem{kid}}
		}
		return
	}
}

// titleMarks bounds the characters a title paragraph may print after the title (\thanks marks).
const titleMarks = 3

// letters is a text's letters and digits, lower case.
func letters(text string) string {
	var out strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(unicode.ToLower(r))
		}
	}
	return out.String()
}

// leafText is the text a block leaf's content prints.
func (b *builder) leafText(leaf string) string {
	var text strings.Builder
	for _, item := range b.doc.Nodes[leaf].Content {
		text.WriteString(item.Text)
	}
	return text.String()
}

// markContents wraps each run of contents entries among an element's kids, and its descendants', in
// a TOC with a TOCI for each: paragraphs that print a heading of the document and a page number.
// A run is at least two entries long. Each TOCI refers to its heading (/Ref, PDF/UA-2 8.2.5.8): the
// first heading of its name no entry took yet.
func (b *builder) markContents(element *selem) {
	headings := b.headingTexts()
	if len(headings) == 0 {
		return
	}
	taken := map[string]int{}
	entry := func(kid *selem) string {
		if kid.role != "P" || kid.leaf == "" {
			return ""
		}
		return namesHeading(headings, b.leafText(kid.leaf))
	}
	target := func(name string) *selem {
		ids := headings[name]
		index := min(taken[name], len(ids)-1)
		taken[name]++
		return b.leaves[ids[index]]
	}
	var walk func(*selem)
	walk = func(parent *selem) {
		for _, kid := range parent.kids {
			walk(kid)
		}
		parent.kids = contentsRuns(parent.kids, entry, target)
	}
	walk(element)
}

// contentsRuns replaces each run of at least two entries in kids by a TOC.
func contentsRuns(kids []*selem, entry func(*selem) string, target func(string) *selem) []*selem {
	var out []*selem
	for start := 0; start < len(kids); {
		end := start
		for end < len(kids) && entry(kids[end]) != "" {
			end++
		}
		if end-start < 2 {
			out = append(out, kids[start])
			start++
			continue
		}
		contents := &selem{role: "TOC"}
		for _, kid := range kids[start:end] {
			contents.kids = append(contents.kids, &selem{role: "TOCI", kids: []*selem{kid}, ref: target(entry(kid))})
		}
		out = append(out, contents)
		start = end
	}
	return out
}

// settleContents runs after pruning: a TOCI whose heading was pruned (no glyph of the heading was
// bound to it; papers3/0629's "5 Superfluid transport ..." heading) can name no /Ref, which PDF/UA-2
// requires of every TOCI (8.2.5.8). Such an entry leaves its TOC: its paragraph stands in the TOC's
// place, between TOCs of the entries before and after it that keep their targets.
func settleContents(root *selem) {
	kept := map[*selem]bool{}
	var mark func(*selem)
	mark = func(element *selem) {
		kept[element] = true
		for _, kid := range element.kids {
			mark(kid)
		}
	}
	mark(root)
	var walk func(*selem)
	walk = func(parent *selem) {
		var out []*selem
		for _, kid := range parent.kids {
			if kid.role != "TOC" {
				walk(kid)
				out = append(out, kid)
				continue
			}
			out = append(out, splitContents(kid, kept)...)
		}
		parent.kids = out
	}
	walk(root)
}

// splitContents is a TOC without its entries whose target is not kept: those entries' paragraphs,
// and TOCs of the runs of entries between them.
func splitContents(contents *selem, kept map[*selem]bool) []*selem {
	var out []*selem
	var run *selem
	for _, entry := range contents.kids {
		if entry.role != "TOCI" || (entry.ref != nil && kept[entry.ref]) {
			if run == nil {
				run = &selem{role: "TOC"}
				out = append(out, run)
			}
			run.kids = append(run.kids, entry)
			continue
		}
		run = nil
		out = append(out, entry.kids...)
	}
	if len(out) == 1 && out[0].role == "TOC" {
		return []*selem{contents}
	}
	return out
}

// headingTexts are the document's headings, in reading order, by their names as namesHeading
// compares them: with their numbers and without.
func (b *builder) headingTexts() map[string][]string {
	texts := map[string][]string{}
	for _, id := range b.doc.BlockOrder {
		if b.doc.Nodes[id].Type != "heading" {
			continue
		}
		text := b.leafText(id)
		full, bare := letters(text), letters(sectionNumber.ReplaceAllString(text, ""))
		for _, form := range []string{full, bare} {
			if form != "" && (form == full || bare != full) {
				texts[form] = append(texts[form], id)
			}
		}
	}
	return texts
}

// sectionNumber opens a contents entry or a heading: its number ("3.6", "A", "IV").
var sectionNumber = regexp.MustCompile(`\A\s*([0-9]+|[A-Z]|[IVXLC]+)(\.[0-9]+)*\.?\s+`)

// namesHeading is the name of the heading a contents entry names, with its number or without; ""
// when the paragraph names none or ends with no page number.
func namesHeading(headings map[string][]string, text string) string {
	name := entryHeading(text)
	if !strings.ContainsFunc(name, unicode.IsLetter) {
		return ""
	}
	if full := letters(name); len(headings[full]) > 0 {
		return full
	}
	if bare := letters(sectionNumber.ReplaceAllString(name, "")); len(headings[bare]) > 0 {
		return bare
	}
	return ""
}

// entryHeading is the heading a contents entry names: its text without the page number; "" when
// it ends with none.
func entryHeading(text string) string {
	text = strings.TrimSpace(text)
	match := pageNumber.FindStringIndex(text)
	if match == nil || match[0] == 0 {
		return ""
	}
	return text[:match[0]]
}
