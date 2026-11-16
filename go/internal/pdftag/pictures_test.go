package pdftag

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

func TestInterpreterBoundsPathsAndClippedShadings(t *testing.T) {
	items := interpretTest(t, "q 1 0 0 1 100 100 cm 0 0 m 50 0 l 50 20 l S 0 0 20 10 re W n /Sh0 sh Q /Sh0 sh")
	var boxes []bounds
	var boxed []bool
	for _, entry := range items {
		if entry.kind == kindPaint || entry.kind == kindObject {
			boxes, boxed = append(boxes, entry.box), append(boxed, entry.boxed)
		}
	}
	want := []bounds{{100, 100, 150, 120}, {100, 100, 120, 110}, {100, 100, 120, 110}}
	if len(boxes) != 4 || boxes[0] != want[0] || boxes[1] != want[1] || boxes[2] != want[2] {
		t.Fatalf("boxes %v", boxes)
	}
	// The clip ends with its Q: a shading painted after it fills the page and has no box.
	if !boxed[2] || boxed[3] {
		t.Fatalf("boxed %v", boxed)
	}
}

func TestDrawnPicturePaintingIsItsFigures(t *testing.T) {
	items := interpretTest(t, "q 1 0 0 1 100 100 cm 0 0 m 50 0 l S -1 -1 m 51 31 l S Q "+
		"300 300 m 310 300 l S 0 50 m 600 50 l S")
	tags := &pageTags{leafOf: func(int) string { return "" }, objects: map[int]string{},
		roleOf:   func(leaf string) string { return map[string]string{"fig": "Figure"}[leaf] },
		pictures: []picture{{leaf: "fig", box: bounds{100, 100, 150, 130}}}}
	content, marks := rewrite(items, tags)
	text := string(content)
	// Both paths in the box (the second reaching 1 bp past it, a line width) are the Figure's; the
	// path elsewhere and the rule across the page stay Artifacts.
	for _, want := range []string{"/Figure <</MCID 0>> BDC\n0 0 m\n50 0 l\nS\n-1 -1 m\n51 31 l\nS\nEMC\nQ",
		"/Artifact BMC\n300 300 m\n310 300 l\nS\n0 50 m\n600 50 l\nS\nEMC"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rewritten content lacks %q:\n%s", want, text)
		}
	}
	if len(marks) != 1 || marks[0].leaf != "fig" {
		t.Fatalf("marks %+v", marks)
	}
}

func TestDrawnPictureGlyphsAndPlacement(t *testing.T) {
	owners := &glyphowner.Owners{Pictures: []glyphowner.Picture{
		{Node: "fig", Page: 1, Box: [4]float64{100, 662, 150, 692}},
		{Node: "deco", Page: 1, Box: [4]float64{0, 0, 10, 10}},
		{Node: "fig", Page: 2, Box: [4]float64{0, 0, 10, 10}},
	}}
	tagger := &tagger{owners: owners, builder: &builder{leaves: map[string]*selem{"fig": {role: "Figure", leaf: "fig"}}},
		painted: map[int][]paintedGlyph{1: {{text: "x", baseline: [2]float64{110, 680}}, {text: "y", baseline: [2]float64{110, 700}},
			{unknown: true}}}}
	// The box is turned from the layout's frame into device space; a decorative image (no Figure)
	// and another page's picture are left out.
	pictures := tagger.pagePictures(1, pageFrame{box: [4]float64{0, 0, 612, 792}})
	if len(pictures) != 1 || pictures[0].box != (bounds{100, 100, 150, 130}) {
		t.Fatalf("pictures %+v", pictures)
	}
	page := &prepared{number: 1, mapper: &glyphMapper{mapping: []int{0, 1, 2, -1}, frame: pageFrame{box: [4]float64{0, 0, 612, 792}}}}
	for ordinal, want := range []string{"fig", "", "", ""} {
		if got := tagger.glyphPicture(page, pictures, ordinal); got != want {
			t.Errorf("glyph %d: picture %q, want %q", ordinal, got, want)
		}
	}
	// A drawn picture never claims an image object of its size: its box places it.
	pages := []*prepared{{number: 1, objects: map[int]string{},
		items: []item{{kind: kindObject, xobject: "Form", size: [2]float64{50, 30}}}}}
	placeImages(pages, []imagePiece{{leaf: "fig", page: 1, size: [2]float64{50, 30}}}, tagger.drawnLeaves())
	if len(pages[0].objects) != 0 {
		t.Fatalf("the drawn picture claimed %v", pages[0].objects)
	}
	placeImages(pages, []imagePiece{{leaf: "img", page: 1, size: [2]float64{50, 30}}}, tagger.drawnLeaves())
	if pages[0].objects[0] != "img" {
		t.Fatalf("an included image was not placed: %v", pages[0].objects)
	}
}
