package pdftag

import "testing"

// A run-in heading's title is an H one level below the last heading, before its paragraph's P; the
// owners' title glyphs go to the title's leaf and the paragraph's other glyphs to the body's.
func TestRuninTitleIsAHeadingBeforeItsParagraph(t *testing.T) {
	paragraph := node{ID: "p", Type: "paragraph", Content: []contentItem{{Type: "text"}, {Type: "text"}}}
	paragraph.Semantics.Runin, paragraph.Semantics.TitleSpans, paragraph.Semantics.Level = true, 1, 4
	heading := node{ID: "h", Type: "heading"}
	heading.Semantics.Level = 1
	plain := node{ID: "q", Type: "paragraph", Content: []contentItem{{Type: "text"}}}
	doc := &document{Root: "d", Nodes: map[string]node{
		"d": {ID: "d", Type: "document", Children: []string{"h", "p", "q"}}, "h": heading, "p": paragraph, "q": plain}}
	b := newBuilder(doc)
	root := b.build("d")[0]
	roles := []string{}
	for _, kid := range root.kids {
		roles = append(roles, kid.role)
	}
	if len(roles) != 4 || roles[0] != "H1" || roles[1] != "H2" || roles[2] != "P" || roles[3] != "P" {
		t.Fatalf("roles %v", roles)
	}
	for owner, want := range map[string]string{"p#title": "p#r0", "p": "p#r1", "q": "q", "q#title": "q", "x": "x"} {
		if got := b.titleLeaf(owner); got != want {
			t.Errorf("%s: %s, want %s", owner, got, want)
		}
	}
}

// A paragraph that is all title is one H.
func TestRuninTitleAlone(t *testing.T) {
	paragraph := node{ID: "p", Type: "paragraph", Content: []contentItem{{Type: "text"}}}
	paragraph.Semantics.Runin, paragraph.Semantics.TitleSpans = true, 1
	runs := splitRuns(paragraph)
	if len(runs) != 1 || !runs[0].title || runID(paragraph, len(runs), 0) != "p" {
		t.Fatalf("runs %v", runs)
	}
}
