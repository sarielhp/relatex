package pdftag

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/pdffonts"
)

// The font spells the document's ‖ (cmsy bardbl) as ∥ (papers3 0629) and its en dash with a hyphen
// glyph (papers4 0995); the leaf's glyphs, aligned with its text, get the document's characters. A
// hyphen the check drops (a line-break hyphen) stays as it is, a small capital matches in either
// case, and a letter never stands for another: a leaf whose words differ gets nothing.
func TestSpellingsTakeTheDocumentsSymbols(t *testing.T) {
	glyphs := []paintedGlyph{{text: "D"}, {text: "∥"}, {text: "-"}, {text: "x"}, {text: "-"}, {text: "y"}, {text: "."}}
	painted := map[int][]paintedGlyph{2: glyphs}
	var refs []glyphRef
	for index := range glyphs {
		refs = append(refs, glyphRef{2, index})
	}
	want := expectedText{text: "d‖x–y.", caps: []bool{true, false, false, false, false, false}}
	got := spellLeaf(painted, refs, want)
	if len(got) != 2 || got[glyphRef{2, 1}] != "‖" || got[glyphRef{2, 4}] != "–" {
		t.Fatalf("spelled %v", got)
	}
	if got := spellLeaf(painted, refs, expectedText{text: "d‖z–y."}); got != nil {
		t.Fatalf("a different letter respelled: %v", got)
	}
	if got := spellLeaf(painted, refs, expectedText{text: "D∥xy."}); got != nil {
		t.Fatalf("an agreeing leaf respelled: %v", got)
	}
	if linkHost("n-1#a12") != "n-1" || linkHost("n-1#title") != "n-1#title" || linkHost("n-1#a") != "n-1#a" {
		t.Fatal("linkHost")
	}
}

// A genuine symbol mismatch is not respelled: a + glyph bound where the model says −, a < where it
// says ≤ are different characters to Unicode (UTS #39 skeletons), so the leaf keeps the font's
// spelling and still fails the order check.
func TestSpellingsLeaveDifferentSymbolsAlone(t *testing.T) {
	for _, c := range []struct{ glyph, model string }{{"+", "−"}, {"<", "≤"}} {
		glyphs := []paintedGlyph{{text: "a"}, {text: c.glyph}, {text: "b"}}
		refs := []glyphRef{{1, 0}, {1, 1}, {1, 2}}
		want := expectedText{text: "a" + c.model + "b"}
		if got := spellLeaf(map[int][]paintedGlyph{1: glyphs}, refs, want); got != nil {
			t.Errorf("%s for %s respelled: %v", c.glyph, c.model, got)
		}
		read := glyphText(glyphs, newGlyphRoles(glyphs), []int{0, 1, 2})
		if kind := matchKind(read, want, false); kind != "" {
			t.Errorf("%s for %s passes the order check as %q", c.glyph, c.model, kind)
		}
	}
}

// The tagged content writes the glyph inside an /ActualText of its own, and the order check, reading
// the marked content as a reader does, reads the document's ‖ where the layout's glyph reads ∥.
func TestActualTextIsWrittenAndReadBack(t *testing.T) {
	items := interpretTest(t, "BT /F1 10 Tf 100 700 Td (a|b) Tj ET")
	tags := &pageTags{leafOf: func(int) string { return "p" }, objects: map[int]string{},
		roleOf:  func(string) string { return "P" },
		spelled: func(ordinal int) string { return map[int]string{1: "‖"}[ordinal] }}
	content, marks := rewrite(items, tags)
	want := "/P <</MCID 0>> BDC\n[<61>] TJ\n/Span <</ActualText <FEFF2016>>> BDC\n[<7C> ] TJ\nEMC\n[<62>] TJ\nEMC\n"
	if !strings.Contains(string(content), want) || len(marks) != 1 {
		t.Fatalf("content %q, marks %v", content, marks)
	}
	operations, err := parseContent(content)
	if err != nil {
		t.Fatal(err)
	}
	written, err := newInterpreter(&pdfFile{objects: map[string]any{}}, interpretResources()).run(operations)
	if err != nil {
		t.Fatal(err)
	}
	glyphs := []paintedGlyph{{text: "a"}, {text: "∥"}, {text: "b"}}
	texts := pageTexts(written, glyphs, []int{0, 1, 2})
	if got := texts[0].plain; got != "a‖b" {
		t.Fatalf("read %q", got)
	}
	// The same page without the /ActualText reads the font's spelling: the check is not forgiving.
	plain, _ := rewrite(items, &pageTags{leafOf: tags.leafOf, objects: map[int]string{}, roleOf: tags.roleOf})
	operations, _ = parseContent(plain)
	unspelled, _ := newInterpreter(&pdfFile{objects: map[string]any{}}, interpretResources()).run(operations)
	if got := pageTexts(unspelled, glyphs, []int{0, 1, 2})[0].plain; got != "a∥b" {
		t.Fatalf("read %q without ActualText", got)
	}
}

// An /ActualText reads as the whole of its sequence: its first glyph has the text, the others
// none; an empty one (a drawing font's pieces) reads as nothing.
func TestReadGlyphsFollowActualText(t *testing.T) {
	items := interpretTest(t, "BT /F1 10 Tf /P <</MCID 0>> BDC /Span <</ActualText (fi)>> BDC (xy) Tj EMC "+
		"/Span <</ActualText ()>> BDC (z) Tj EMC (w) Tj EMC ET")
	glyphs := []paintedGlyph{{text: "x"}, {text: "y"}, {text: "z"}, {text: "w"}}
	read := readGlyphs(items, glyphs, []int{0, 1, 2, 3})
	var texts []string
	for _, glyph := range read {
		texts = append(texts, glyph.text)
	}
	if strings.Join(texts, "|") != "fi|||w" || glyphs[0].text != "x" {
		t.Fatalf("read %v", texts)
	}
}

// A glyph of a font embedded in place of the viewer's substitute whose width changed (SegoeUI drawn
// with Nimbus Roman: 900 in Widths, 889.4 in the program; papers5 1362) is shown alone at the
// horizontal scaling that stretches it as the viewer did, and the kern after it ends it where the
// stretched glyph ended, character and word spacing included.
func TestStretchedGlyphIsScaledToItsOldAdvance(t *testing.T) {
	items := interpretTest(t, "BT /F1 12 Tf 0.5 Tc 2 Tw 110 Tz [(A M)-50(A)] TJ ET")
	var show item
	for _, entry := range items {
		if entry.kind == kindShow {
			show = entry
		}
	}
	s := pdffonts.Stretch{Factor: 900 / 889.4, Old: 900, New: 889, Own: 889.4}
	stretched := map[int]pdffonts.Stretch{'M': s, ' ': {Factor: 250 / 260.0, Old: 250, New: 260, Own: 260}}
	var out bytes.Buffer
	writeCompensated(&out, show, nil, stretched)
	got := out.String()
	if !strings.HasPrefix(got, "[<41> ] TJ\n105.769231 Tz\n[<20> -8.333333 ] TJ\n110 Tz\n") || !strings.Contains(got, "111.310996 Tz\n[<4D> 0.090741 ] TJ\n110 Tz\n") ||
		!strings.HasSuffix(got, "110 Tz\n[-50 <41> ] TJ\n") {
		t.Fatalf("content %q", got)
	}
	for _, code := range []byte{'M', ' '} {
		s := stretched[int(code)]
		word := 0.0
		if code == ' ' {
			word = show.text.wordSpace
		}
		k := stretchKern(show, []byte{code}, s)
		advance := (float64((s.New-k)*show.text.size)/1000 + show.text.charSpace + word) * s.Factor
		old := float64(s.Old*show.text.size)/1000 + show.text.charSpace + word
		if math.Abs(advance-old) > 1e-9 {
			t.Errorf("code %q advances %v, the viewer %v", code, advance, old)
		}
	}
}

func interpretResources() map[string]any {
	return map[string]any{"/Font": map[string]any{"/F1": map[string]any{"/Subtype": "/Type1"}}}
}
