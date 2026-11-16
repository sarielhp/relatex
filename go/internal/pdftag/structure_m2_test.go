package pdftag

import (
	"slices"
	"testing"
)

// A note the tree does not reach is an FENote for the Document's end, its paragraph a leaf whose
// text begins with the note's number, as the glyphs show it; a note made once is not made again.
func TestNotesAreFENotes(t *testing.T) {
	note := node{ID: "n", Type: "note", Children: []string{"q"}}
	note.Labels = append(note.Labels, struct {
		Block string `json:"block"`
		Text  string `json:"text"`
	}{Block: "q", Text: "1"})
	doc := &document{Root: "d", NoteOrder: []string{"n"}, BlockOrder: []string{"p", "q"}, Nodes: map[string]node{
		"d": {ID: "d", Type: "document", Children: []string{"p"}},
		"p": {ID: "p", Type: "paragraph", Content: []contentItem{{Type: "text", Text: "Body"}}},
		"n": note,
		"q": {ID: "q", Type: "paragraph", Content: []contentItem{{Type: "text", Text: "A note."}}}}}
	b := newBuilder(doc)
	root := b.build("d")
	notes := b.notes()
	if len(root) != 1 || len(notes) != 1 || notes[0].role != "FENote" || len(notes[0].kids) != 1 || notes[0].kids[0].leaf != "q" {
		t.Fatalf("notes %v", notes)
	}
	text := ""
	for _, p := range b.pieces() {
		if p.leaf == "q" {
			text += p.text
		}
	}
	if text != "1A note." {
		t.Fatalf("note text %q", text)
	}
	if again := b.notes(); len(again) != 0 {
		t.Fatalf("a note already in the tree is made again: %v", again)
	}
}

// A float with two captions holds a Sect per caption, each with what it captions; one caption
// stays the Aside's own.
func TestFloatCaptionsGrouped(t *testing.T) {
	element := func(role string) *selem { return &selem{role: role} }
	trailing := &selem{role: "Aside", kids: []*selem{element("P"), element("Caption"), element("P"), element("Caption")}}
	groupCaptions(trailing)
	if got := roles(trailing.kids); !slices.Equal(got, []string{groupRole, groupRole}) || !slices.Equal(roles(trailing.kids[1].kids), []string{"P", "Caption"}) {
		t.Fatalf("trailing captions: %v", got)
	}
	leading := &selem{role: "Aside", kids: []*selem{element("Caption"), element("P"), element("Caption"), element("P")}}
	groupCaptions(leading)
	if got := roles(leading.kids); !slices.Equal(got, []string{groupRole, groupRole}) || !slices.Equal(roles(leading.kids[1].kids), []string{"Caption", "P"}) {
		t.Fatalf("leading captions: %v", got)
	}
	one := &selem{role: "Aside", kids: []*selem{element("P"), element("Caption")}}
	groupCaptions(one)
	if got := roles(one.kids); !slices.Equal(got, []string{"P", "Caption"}) {
		t.Fatalf("one caption: %v", got)
	}
}

// A paint index the layout skipped (an extensible delimiter's top piece) takes the glyph shown in
// its place: the page binds instead of being left all artifact.
func TestSkippedPaintIndexBinds(t *testing.T) {
	box := [4]float64{0, 0, 100, 100}
	painted := []paintedGlyph{{text: "|", baseline: [2]float64{10, 20}}, {unknown: true}, {text: "x", baseline: [2]float64{30, 20}}}
	items := []item{{kind: kindShow, first: 0, count: 1, placed: true, origin: [2]float64{10, 80}},
		{kind: kindShow, first: 1, count: 1, placed: true, origin: [2]float64{10, 70}},
		{kind: kindShow, first: 2, count: 1, placed: true, origin: [2]float64{30, 80}}}
	m, err := mapGlyphs(items, painted, box, 3)
	if err != nil || !slices.Equal(m.mapping, []int{0, 1, 2}) {
		t.Fatalf("mapping %v: %v", m.mapping, err)
	}
}

// The leaves of a block cut at its table (its runs and cells) are recorded with the block.
func TestCutBlockLeaves(t *testing.T) {
	grid := &tableGrid{Cells: []tableCell{{ID: "c1", Row: 1, Column: 1}, {ID: "c2", Row: 1, Column: 2}}}
	doc := &document{Nodes: map[string]node{"p": {ID: "p", Type: "paragraph",
		Content: []contentItem{{Type: "text", Text: "Before"}, {Type: "table_grid", Grid: grid}}}}}
	b := newBuilder(doc)
	b.build("p")
	for _, leaf := range []string{"p#r0", "c1", "c2"} {
		if b.cutBlock[leaf] != "p" {
			t.Fatalf("leaf %s: block %q (%v)", leaf, b.cutBlock[leaf], b.cutBlock)
		}
	}
}

// A glyph the owners give a block cut at its table stays in the block: the alignment's leaf outside
// it is replaced by the nearest placed glyph's of the block; one the alignment left unbound stays
// unbound, and glyphs of no such block keep the alignment's leaf.
func TestOpenLeavesStayInTheirBlock(t *testing.T) {
	tg := &tagger{builder: &builder{cutBlock: map[string]string{"c1": "p", "c2": "p"}}}
	aligned := map[glyphRef]string{{1, 0}: "c1", {1, 1}: "formula", {1, 2}: "c2", {1, 4}: "q"}
	got := tg.openLeaves(1, []string{"p", "p", "p", "p", ""}, aligned)
	if want := []string{"c1", "c1", "c2", "", "q"}; !slices.Equal(got, want) {
		t.Fatalf("leaves %v, want %v", got, want)
	}
}
