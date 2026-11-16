package pdftag

import (
	"slices"
	"testing"
)

func textItems(texts ...string) []contentItem {
	items := []contentItem{}
	for _, text := range texts {
		items = append(items, contentItem{Type: "text", Text: text})
	}
	return items
}

// An item's label, the first characters of its first paragraph, is the LI's Lbl before its LBody,
// and its own piece: the paragraph keeps the rest. A number opens an ordered list's item only; a
// bullet any item's.
func TestListItemLabelIsLbl(t *testing.T) {
	doc := &document{Root: "d", Nodes: map[string]node{
		"d":  {ID: "d", Type: "document", Children: []string{"ol", "ul"}},
		"ol": {ID: "ol", Type: "ordered_list", Children: []string{"i1", "i2"}},
		"i1": {ID: "i1", Type: "list_item", Children: []string{"p1"}},
		"p1": {ID: "p1", Type: "paragraph", Content: textItems("(a) What is ", "it?")},
		"i2": {ID: "i2", Type: "list_item", Children: []string{"p2"}},
		"p2": {ID: "p2", Type: "paragraph", Content: textItems("Plain words")},
		"ul": {ID: "ul", Type: "unordered_list", Children: []string{"i3", "i4"}},
		"i3": {ID: "i3", Type: "list_item", Children: []string{"p3"}},
		"p3": {ID: "p3", Type: "paragraph", Content: textItems("•", " point")},
		"i4": {ID: "i4", Type: "list_item", Children: []string{"p4"}},
		"p4": {ID: "p4", Type: "paragraph", Content: textItems("2. not a label here")},
	}, BlockOrder: []string{"p1", "p2", "p3", "p4"}}
	b := newBuilder(doc)
	built := b.build("d")
	lists := built[0].kids
	if got := roles(lists[0].kids[0].kids); !slices.Equal(got, []string{"Lbl", "LBody"}) {
		t.Fatalf("numbered item: %v", got)
	}
	if got := roles(lists[0].kids[1].kids); !slices.Equal(got, []string{"LBody"}) {
		t.Fatalf("item without a label: %v", got)
	}
	if got := roles(lists[1].kids[0].kids); !slices.Equal(got, []string{"Lbl", "LBody"}) {
		t.Fatalf("bullet item: %v", got)
	}
	if got := roles(lists[1].kids[1].kids); !slices.Equal(got, []string{"LBody"}) {
		t.Fatalf("a number in an unordered list: %v", got)
	}
	var texts []string
	for _, p := range b.pieces() {
		texts = append(texts, p.leaf+"="+p.text)
	}
	want := []string{"p1#lbl=(a)", "p1=What is ", "p1=it?", "p2=Plain words", "p3#lbl=•", "p3=", "p3= point",
		"p4=2. not a label here"}
	if !slices.Equal(texts, want) {
		t.Fatalf("pieces %q", texts)
	}
}

// The owners give an item's label to its paragraph: the paragraph's first glyphs, in paint order,
// become the label's when they spell it; otherwise the label returns to the paragraph.
func TestLabelGlyphsFromOwners(t *testing.T) {
	b := newBuilder(&document{})
	b.textLabels = map[string]itemLabel{"p": {text: "1.", cut: 3}, "q": {text: "•", cut: 4}}
	painted := []paintedGlyph{{text: "1"}, {text: "."}, {text: "W"}, {text: "x"}, {text: "y"}}
	tagger := &tagger{builder: b, painted: map[int][]paintedGlyph{1: painted}, binding: &binding{leafOf: map[glyphRef]string{
		{1, 2}: "p", {1, 0}: "p", {1, 1}: "p", {1, 3}: "q", {1, 4}: "q"}}}
	if !tagger.bindLabels() {
		t.Fatal("q's label does not spell its glyphs and returns")
	}
	got := []string{}
	for index := range painted {
		got = append(got, tagger.binding.leafOf[glyphRef{1, index}])
	}
	if !slices.Equal(got, []string{"p#lbl", "p#lbl", "p", "q", "q"}) {
		t.Fatalf("leaves %v", got)
	}
	if _, kept := b.textLabels["q"]; kept {
		t.Fatal("q's label returned")
	}
}

// The paragraph that prints the front matter's title (a \thanks mark after it) is a Title holding
// it; a later one is not.
func TestTitleParagraph(t *testing.T) {
	doc := &document{Nodes: map[string]node{
		"t": {ID: "t", Type: "paragraph", Content: textItems("A Study of  Things", "∗")},
		"a": {ID: "a", Type: "paragraph", Content: textItems("A Study of Things")},
	}}
	doc.Frontmatter.Title = "A study of things"
	b := newBuilder(doc)
	root := &selem{role: "Document", kids: []*selem{b.leaf("P", "t"), b.leaf("P", "a")}}
	b.markTitle(root)
	if got := roles(root.kids); !slices.Equal(got, []string{"Title", "P"}) || root.kids[0].kids[0].leaf != "t" {
		t.Fatalf("roles %v", got)
	}
}

// Paragraphs that name the document's headings and end with a page number are a TOC of TOCI, each
// holding its paragraph; one such paragraph alone is not.
func TestContentsEntries(t *testing.T) {
	doc := &document{Nodes: map[string]node{
		"h1": {ID: "h1", Type: "heading", Content: textItems("1 Introduction")},
		"h2": {ID: "h2", Type: "heading", Content: textItems("3.6 Equation groups")},
		"e1": {ID: "e1", Type: "paragraph", Content: textItems("1 Introduction 1")},
		"e2": {ID: "e2", Type: "paragraph", Content: textItems("3.6 Equation groups . . . . 12")},
		"e3": {ID: "e3", Type: "paragraph", Content: textItems("Introduction iv")},
		"x":  {ID: "x", Type: "paragraph", Content: textItems("Some text 3")},
		"e4": {ID: "e4", Type: "paragraph", Content: textItems("1 Introduction 1")},
	}, BlockOrder: []string{"h1", "h2"}}
	b := newBuilder(doc)
	h1, h2 := b.leaf("H1", "h1"), b.leaf("H2", "h2")
	section := &selem{role: "Sect"}
	for _, id := range []string{"e1", "e2", "e3", "x", "e4"} {
		section.kids = append(section.kids, b.leaf("P", id))
	}
	b.markContents(&selem{role: "Document", kids: []*selem{section}})
	if got := roles(section.kids); !slices.Equal(got, []string{"TOC", "P", "P"}) {
		t.Fatalf("roles %v", got)
	}
	if got := roles(section.kids[0].kids); !slices.Equal(got, []string{"TOCI", "TOCI", "TOCI"}) {
		t.Fatalf("entries %v", got)
	}
	entries := section.kids[0].kids
	if entries[1].kids[0].leaf != "e2" {
		t.Fatal("a TOCI holds its paragraph")
	}
	if entries[0].ref != h1 || entries[1].ref != h2 || entries[2].ref != h1 {
		t.Fatal("each TOCI refers to its heading")
	}
}

// A TOCI whose heading was pruned (papers3/0629: no glyph of the heading bound to it) can name no
// /Ref (PDF/UA-2 8.2.5.8): its paragraph leaves the TOC, which splits around it; a TOC whose
// entries all keep their headings stays as it is.
func TestContentsEntryWithoutHeadingLeavesItsTOC(t *testing.T) {
	h1, h2, h3 := &selem{role: "H1", mcrs: []mcr{{1, 0, 0}}}, &selem{role: "H1"}, &selem{role: "H1", mcrs: []mcr{{1, 1, 1}}}
	entry := func(target *selem) *selem {
		return &selem{role: "TOCI", ref: target, kids: []*selem{{role: "P", mcrs: []mcr{{1, 2, 2}}}}}
	}
	lost := entry(h2)
	contents := &selem{role: "TOC", kids: []*selem{entry(h1), lost, entry(h3), entry(h1)}}
	whole := &selem{role: "TOC", kids: []*selem{entry(h1), entry(h3)}}
	root := &selem{role: "Document", kids: []*selem{h1, contents, h2, h3, whole}}
	if !prune(root) {
		t.Fatal("pruned all")
	}
	settleContents(root)
	if got := roles(root.kids); !slices.Equal(got, []string{"H1", "TOC", "P", "TOC", "H1", "TOC"}) {
		t.Fatalf("roles %v", got)
	}
	if root.kids[2] != lost.kids[0] || len(root.kids[1].kids) != 1 || len(root.kids[3].kids) != 2 || root.kids[5] != whole {
		t.Fatal("the lost entry's paragraph stands between the TOCs of the others")
	}
}

// A run of paragraphs that open with a bracketed key (a bibliography) is an L, each paragraph an LI
// with its key as Lbl; its label is a piece of its own.
func TestReferencesAreAList(t *testing.T) {
	doc := &document{Nodes: map[string]node{
		"r1": {ID: "r1", Type: "paragraph", Content: textItems("[Gup00] A. Gupta.")},
		"r2": {ID: "r2", Type: "paragraph", Content: textItems("[12] B. Chazelle.")},
		"x":  {ID: "x", Type: "paragraph", Content: textItems("[a b] not a key")},
		"r3": {ID: "r3", Type: "paragraph", Content: textItems("[CRT05] alone")},
	}, BlockOrder: []string{"r1", "r2", "x", "r3"}}
	b := newBuilder(doc)
	section := &selem{role: "Sect"}
	for _, id := range doc.BlockOrder {
		section.kids = append(section.kids, b.leaf("P", id))
	}
	b.markReferences(section)
	if got := roles(section.kids); !slices.Equal(got, []string{"L", "P", "P"}) {
		t.Fatalf("roles %v", got)
	}
	if got := roles(section.kids[0].kids[0].kids); !slices.Equal(got, []string{"Lbl", "LBody"}) {
		t.Fatalf("item %v", got)
	}
	pieces := b.pieces()
	if pieces[0].leaf != "r1#lbl" || pieces[0].text != "[Gup00]" || pieces[1].text != "A. Gupta." {
		t.Fatalf("pieces %v", pieces[:2])
	}
	entry := newBuilder(&document{Nodes: map[string]node{
		"e": {ID: "e", Type: "paragraph", Content: []contentItem{{Type: "text", Text: " M. Bernhardt.", Entry: 34}}}}})
	if label, ok := entry.referenceEntry(&selem{role: "P", leaf: "e"}); !ok || label.text != "[34]" || label.cut != 0 {
		t.Fatalf("a marked entry's label is its number, which its text does not hold: %v", label)
	}
}
