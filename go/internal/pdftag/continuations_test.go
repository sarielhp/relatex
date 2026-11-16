package pdftag

import "testing"

// continuationTagger is a tagger whose page has one Type0 font (5 0 R) with a ToUnicode map that
// gives the ligature code 0477 two characters.
func continuationTagger(painted []paintedGlyph) (*tagger, *prepared) {
	const cmap = "1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n2 beginbfchar\n" +
		"<0046> <0066>\n<0477> <00660066>\nendbfchar\n"
	file := &pdfFile{objects: map[string]any{}}
	file.store("6 0 R", []byte(cmap))
	file.objects[objectKey("5 0 R")] = map[string]any{"value": map[string]any{"/ToUnicode": "6 0 R"}}
	t := &tagger{file: file, painted: map[int][]paintedGlyph{1: painted}}
	page := &prepared{number: 1, mapper: &glyphMapper{mapping: []int{0, 1}}, items: []item{{
		kind: kindShow, font: "5 0 R", first: 0, count: 2,
		elements: []element{{codes: [][]byte{{0x00, 0x46}, {0x04, 0x77}}}},
	}}}
	return t, page
}

// MuPDF leaves the characters after a glyph's first out of text shown in a clip rendering mode, so
// a ligature's layout text is "f" where the font's ToUnicode gives "ff": the page's glyphs are
// completed from the font, which is what a reader of the tagged PDF reads (books/0006's contents).
func TestCompleteContinuationsRestoresClipLigature(t *testing.T) {
	tag, page := continuationTagger([]paintedGlyph{{text: "f"}, {text: "f"}})
	tag.completeContinuations(page)
	if got := tag.painted[1]; got[0].text != "f" || got[1].text != "ff" {
		t.Fatalf("glyph texts %q %q, want f ff", got[0].text, got[1].text)
	}
}

// A layout text that is not the start of the font's value is a real disagreement the check must
// see, and so is an unknown glyph.
func TestCompleteContinuationsLeavesDisagreements(t *testing.T) {
	tag, page := continuationTagger([]paintedGlyph{{text: "x"}, {unknown: true}})
	tag.completeContinuations(page)
	if got := tag.painted[1]; got[0].text != "x" || got[1].text != "" {
		t.Fatalf("glyph texts %q %q, want x and nothing", got[0].text, got[1].text)
	}
	tag, page = continuationTagger([]paintedGlyph{{text: "f"}, {text: "ff"}})
	tag.completeContinuations(page)
	if got := tag.painted[1]; got[1].text != "ff" {
		t.Fatalf("a complete ligature became %q", got[1].text)
	}
}

// A glyph inside an /ActualText sequence reads that text: it is not completed.
func TestCompleteContinuationsSkipsActualText(t *testing.T) {
	tag, page := continuationTagger([]paintedGlyph{{text: "f"}, {text: "f"}})
	page.items[0].actual = actualText{sequence: 1, text: "f"}
	tag.completeContinuations(page)
	if got := tag.painted[1]; got[1].text != "f" {
		t.Fatalf("an ActualText glyph became %q", got[1].text)
	}
}
