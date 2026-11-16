package pdftag

import (
	"fmt"
	"slices"
	"testing"
)

// A Link that holds marked content is ordered by its own MCRs, not by element.first: that is a
// layout paint index, while an MCR's first is a content ordinal, and the layout skips or adds
// paints (a figure's text, form XObjects, fill and stroke) so the two numberings differ.
func TestLinkHoldingTextSortsByItsMarkedContent(t *testing.T) {
	p := &selem{role: "P", leaf: "p", mcrs: []mcr{{page: 1, mcid: 0, first: 0}, {page: 1, mcid: 20, first: 20}}}
	link := &selem{role: "Link", leaf: "p#a0", objr: map[string]any{}, first: glyphRef{1, 3},
		mcrs: []mcr{{page: 1, mcid: 10, first: 10}}}
	p.kids = []*selem{link}
	if page, first := position(link); page != 1 || first != 10 {
		t.Fatalf("the Link begins at %d:%d, want its MCR at 1:10, not element.first (a paint index) 1:%d",
			page, first, link.first.index)
	}
	order(p)
	w := &treeWriter{pages: []page{{ref: "1 0 R"}}, parents: map[int][]string{},
		refs: map[*selem]string{p: "1 0 R", link: "2 0 R"}}
	want := []string{"MCR0", "Link", "MCR20"}
	if got := kidOrder(w.kids(p)); !slices.Equal(got, want) {
		t.Fatalf("the P's /K order %v, want %v", got, want)
	}
}

// A Link without marked content (an OBJR alone, a Beamer navigation symbol) has no content
// ordinal, so it still sorts by element.first.
func TestLinkWithoutMarkedContentSortsByItsPaintIndex(t *testing.T) {
	p := &selem{role: "P", leaf: "p", mcrs: []mcr{{page: 1, mcid: 0, first: 0}, {page: 1, mcid: 5, first: 5}}}
	link := &selem{role: "Link", leaf: "p#a0", objr: map[string]any{}, first: glyphRef{1, 3}}
	p.kids = []*selem{link}
	if page, first := position(link); page != 1 || first != 3 {
		t.Fatalf("the OBJR-only Link begins at %d:%d, want 1:3", page, first)
	}
	order(p)
	w := &treeWriter{pages: []page{{ref: "1 0 R"}}, parents: map[int][]string{},
		refs: map[*selem]string{p: "1 0 R", link: "2 0 R"}}
	want := []string{"MCR0", "Link", "MCR5"}
	if got := kidOrder(w.kids(p)); !slices.Equal(got, want) {
		t.Fatalf("the P's /K order %v, want %v", got, want)
	}
}

// kidOrder names the written /K entries: an MCR by its MCID, a kid element as Link.
func kidOrder(values []any) []string {
	var order []string
	for _, value := range values {
		switch entry := value.(type) {
		case string:
			order = append(order, "Link")
		case map[string]any:
			order = append(order, "MCR"+fmt.Sprint(entry["/MCID"]))
		}
	}
	return order
}

// A structure destination reads an MCR's first as a content ordinal and maps it to the layout's
// paint index before taking the glyph's baseline; an ordinal the layout lacks places nothing.
func TestPlacementsReadAnMCRsFirstAsAContentOrdinal(t *testing.T) {
	p := &selem{role: "P", leaf: "p", mcrs: []mcr{{page: 1, mcid: 0, first: 1}, {page: 1, mcid: 1, first: 2}}}
	root := &selem{role: "Document", kids: []*selem{p}}
	glyphs := []paintedGlyph{
		{baseline: [2]float64{10, 42}}, // paint index 0, where content ordinal 2 maps
		{baseline: [2]float64{10, 99}}, // paint index 1
		{baseline: [2]float64{10, 55}}, // paint index 2, the ordinal itself
	}
	tagger := &tagger{painted: map[int][]paintedGlyph{1: glyphs},
		prepared: []*prepared{{number: 1, mapper: &glyphMapper{mapping: []int{1, -1, 0}}}}}
	w := &treeWriter{t: tagger, refs: map[*selem]string{p: "1 0 R"}}
	placed := w.placements(root)[1]
	if len(placed) != 1 || placed[0].top != 42 || placed[0].ref != "1 0 R" {
		t.Fatalf("the P is placed at %v, want its glyph at paint index 0 (top 42)", placed)
	}
}
