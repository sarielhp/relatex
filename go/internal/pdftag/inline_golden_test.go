package pdftag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// inlineGolden is the golden of TestInlineElementsOwnTheirMarkedContent.
const inlineGolden = "inline-elements.golden"

// The inline elements of decision 121 from binding to parent tree: a paragraph "see x [12] now" whose
// "x" the owners give a formula and whose "[12]" a link annotation covers. The rewritten content holds
// the formula's and the link's glyphs in marked content of their own, the paragraph's /K interleaves
// its MCRs with the Formula and the Link (which holds its MCR and its OBJR), and the parent tree names
// the Formula and the Link, not the P, for their MCIDs. RELATEX_UPDATE_GOLDEN=1 rewrites the golden.
func TestInlineElementsOwnTheirMarkedContent(t *testing.T) {
	text := "see x [12] now"
	glyphs := linkLine(text, 10, 100)
	leaves := sameLeaves(glyphs, "p")
	leaves[4] = "f"
	tagger := linkTagger(glyphs, leaves, map[string]string{"p": "P", "f": "Formula"})
	p, formula := tagger.builder.leaves["p"], tagger.builder.leaves["f"]
	p.kids = []*selem{formula}
	link := tagger.bindLink(1, []bounds{box(glyphs, 6, 4)})
	if link == nil {
		t.Fatal("the link over [12] holds no text")
	}
	link.objr, link.key = map[string]any{"/Type": "/OBJR", "/Obj": "20 0 R"}, 1

	items := interpretTest(t, fmt.Sprintf("BT /F1 10 Tf 1 0 0 1 0 0 Tm [(%s)]TJ ET", text))
	tags := &pageTags{number: 1, objects: map[int]string{}, roleOf: tagger.roleOf,
		leafOf: func(ordinal int) string { return tagger.binding.leafOf[glyphRef{1, ordinal}] }}
	content, marks := rewrite(items, tags)
	tagger.collect(1, pageResult{content: content, marks: marks})
	order(p)
	w := &treeWriter{pages: []page{{ref: "8 0 R"}}, parents: map[int][]string{}, annots: make([]string, 1),
		refs: map[*selem]string{p: "10 0 R", formula: "11 0 R", link: "12 0 R"}}
	var out strings.Builder
	out.WriteString("# content\n" + string(content))
	fmt.Fprintf(&out, "# P /K\n%v\n", w.kids(p))
	fmt.Fprintf(&out, "# Formula /K\n%v\n", w.kids(formula))
	fmt.Fprintf(&out, "# Link /K\n%v\n", w.kids(link))
	fmt.Fprintf(&out, "# ParentTree\n%v\n", w.parentTree()["/Nums"])
	compareGolden(t, inlineGolden, out.String())
}

// compareGolden compares got with testdata/name; RELATEX_UPDATE_GOLDEN=1 rewrites it first.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("RELATEX_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("differs from %s:\n%s", path, got)
	}
}
