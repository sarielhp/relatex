package pdftag

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseContentKeepsEveryByte(t *testing.T) {
	content := []byte("BT /F1 9 Tf 1 0 0 1 10 20 Tm [(Ab)-250(c\\)d)]TJ ET\nq 0 0 1 1 re f Q\n" +
		"/P <</MCID 3>> BDC (x) Tj EMC BI /W 1 /H 1 /BPC 8 /CS /G ID \x00 EI Q")
	operations, err := parseContent(content)
	if err != nil {
		t.Fatal(err)
	}
	var operators []string
	var joined []byte
	for _, op := range operations {
		operators = append(operators, op.operator)
		joined = append(joined, op.raw...)
	}
	want := "BT Tf Tm TJ ET q re f Q BDC Tj EMC BI Q"
	if got := strings.Join(operators, " "); got != want {
		t.Fatalf("operators %q, want %q", got, want)
	}
	if mcidOf(operations[9]) != 3 {
		t.Fatalf("MCID of %s", operations[9].raw)
	}
	if !bytes.Contains(joined, []byte("[(Ab)-250(c\\)d)]TJ")) {
		t.Fatalf("raw TJ lost: %s", joined)
	}
}

func TestLiteralBytes(t *testing.T) {
	cases := map[string]string{`a\(b\)`: "a(b)", `\101\60`: "A0", "x\\\ny": "xy", `\\n`: `\n`, "a\r\nb": "a\nb"}
	for body, want := range cases {
		if got := string(literalBytes([]byte(body))); got != want {
			t.Errorf("literalBytes(%q) = %q, want %q", body, got, want)
		}
	}
	if got := string(hexBytes([]byte("<41 42 4>"))); got != "AB@" {
		t.Errorf("hexBytes = %q", got)
	}
}

// interpretTest interprets content with one simple font F1 and one Type0 font F2.
func interpretTest(t *testing.T, content string) []item {
	t.Helper()
	resources := map[string]any{"/Font": map[string]any{"/F1": map[string]any{"/Subtype": "/Type1"},
		"/F2": map[string]any{"/Subtype": "/Type0", "/Encoding": "/Identity-H"}}}
	operations, err := parseContent([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	items, err := newInterpreter(&pdfFile{objects: map[string]any{}}, resources).run(operations)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestInterpreterCountsAndPlacesGlyphs(t *testing.T) {
	items := interpretTest(t, "1 0 0 1 5 7 cm BT /F1 10 Tf 2 0 0 2 100 200 Tm [-100(ab)50(c)]TJ "+
		"/F2 10 Tf 0 -12 Td <00410042> Tj ET")
	var shows []item
	for _, entry := range items {
		if entry.kind == kindShow {
			shows = append(shows, entry)
		}
	}
	if len(shows) != 2 || shows[0].count != 3 || shows[1].count != 2 || shows[1].first != 3 {
		t.Fatalf("shows %+v", shows)
	}
	// The leading kern moves the first glyph by 100/1000 × 10 in text space, twice in user space.
	if !shows[0].placed || shows[0].origin != [2]float64{107, 207} {
		t.Fatalf("first origin %v", shows[0].origin)
	}
	if !shows[1].placed || shows[1].origin != [2]float64{105, 183} {
		t.Fatalf("second origin %v", shows[1].origin)
	}
}

func TestRewriteSplitsAShowOperatorBetweenLeaves(t *testing.T) {
	items := interpretTest(t, "q 0 0 5 5 re f Q BT /F1 10 Tf 1 0 0 1 0 0 Tm [(ab)-250(cd)]TJ ET")
	leaves := []string{"p1", "p1", "", "f1"}
	tags := &pageTags{leafOf: func(ordinal int) string { return leaves[ordinal] }, objects: map[int]string{},
		roleOf: func(leaf string) string { return map[string]string{"p1": "P", "f1": "Formula"}[leaf] }}
	content, marks := rewrite(items, tags)
	text := string(content)
	for _, want := range []string{"q\n/Artifact BMC\n0 0 5 5 re\nf\nEMC\nQ", "/P <</MCID 0>> BDC\n[<6162> -250] TJ",
		"EMC\n/Artifact BMC\n[<63>] TJ\nEMC\n/Formula <</MCID 1>> BDC\n[<64>] TJ\nEMC\nET"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rewritten content lacks %q:\n%s", want, text)
		}
	}
	if len(marks) != 2 || marks[1].leaf != "f1" || marks[1].first != 3 {
		t.Fatalf("marks %+v", marks)
	}
	again := interpretTest(t, text)
	total := 0
	for _, entry := range again {
		total += entry.count
	}
	if total != 4 {
		t.Fatalf("the rewritten content paints %d glyphs", total)
	}
}

func TestDrawingFontGlyphsAreDrawingWithoutText(t *testing.T) {
	file := &pdfFile{objects: map[string]any{objectKey("9 0 R"): map[string]any{
		"value": map[string]any{"/Subtype": "/Type1", "/BaseFont": "/LINEW10"}}}}
	resources := map[string]any{"/Font": map[string]any{"/F1": map[string]any{"/Subtype": "/Type1"}, "/F3": "9 0 R"}}
	operations, err := parseContent([]byte("BT /F1 10 Tf (ab)Tj /F3 10 Tf (c)Tj (d)Tj ET"))
	if err != nil {
		t.Fatal(err)
	}
	items, err := newInterpreter(file, resources).run(operations)
	if err != nil {
		t.Fatal(err)
	}
	// linew10's c is bound to the paragraph (a picture in running text): an Artifact; its d to a
	// formula, whose drawing it is. Neither reads as text.
	leaves := []string{"p1", "p1", "p1", "f1"}
	tags := &pageTags{leafOf: func(ordinal int) string { return leaves[ordinal] }, objects: map[int]string{},
		roleOf: func(leaf string) string { return map[string]string{"p1": "P", "f1": "Formula"}[leaf] },
		drawn:  map[string]bool{"9 0 R": true}, owns: func(leaf string) bool { return leaf == "f1" }}
	content, _ := rewrite(items, tags)
	want := "/P <</MCID 0>> BDC\n(ab)Tj\n/F3 10 Tf\nEMC\n/Artifact BMC\n" + drawingText + "(c)Tj\nEMC\nEMC\n" +
		"/Formula <</MCID 1>> BDC\n" + drawingText + "(d)Tj\nEMC\nEMC\nET"
	if !strings.Contains(string(content), want) {
		t.Fatalf("rewritten content lacks %q:\n%s", want, content)
	}
}

func TestBindSkipsHyphensAndFindsFloatsBehind(t *testing.T) {
	painted := map[int][]paintedGlyph{1: glyphsOf("FIG.1.Acaption", "Thecap-tured", "x=1", "word.")}
	pieces := []piece{
		{leaf: "p", kind: pieceText, text: "The captured", first: true},
		{leaf: "m", kind: pieceMath, chars: []rune("x=1")},
		{leaf: "p", kind: pieceText, text: "word."},
		{leaf: "c", kind: pieceText, text: "FIG. 1. A caption", first: true},
	}
	result := bind(pieces, newGlyphStream(painted, 1))
	if result.wordsBound != result.words || result.bound != 1 {
		t.Fatalf("bound %d of %d words, %d formulas: %v", result.wordsBound, result.words, result.bound, result.misses)
	}
	if result.leafOf[glyphRef{1, 0}] != "c" || result.leafOf[glyphRef{1, 17}] != "p" || result.leafOf[glyphRef{1, 26}] != "m" {
		t.Fatalf("leaves %v", result.leafOf)
	}
}

// glyphsOf makes one painted glyph per character.
func glyphsOf(parts ...string) []paintedGlyph {
	var glyphs []paintedGlyph
	for _, part := range parts {
		for _, r := range part {
			glyphs = append(glyphs, paintedGlyph{text: string(r)})
		}
	}
	return glyphs
}

func TestMapGlyphsAllowsAFormTheLayoutLeftOut(t *testing.T) {
	box := [4]float64{0, 0, 100, 100}
	items := []item{
		{kind: kindShow, first: 0, count: 2, placed: true, origin: [2]float64{10, 90}},
		{kind: kindObject, first: 2, count: 3, xobject: "Form"},
		{kind: kindShow, first: 5, count: 1, placed: true, origin: [2]float64{20, 80}},
	}
	painted := []paintedGlyph{{baseline: [2]float64{10, 10}}, {baseline: [2]float64{15, 10}}, {baseline: [2]float64{20, 20}}}
	mapper, err := mapGlyphs(items, painted, box, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got := mapper.mapping; got[0] != 0 || got[1] != 1 || got[2] != -1 || got[4] != -1 || got[5] != 2 {
		t.Fatalf("mapping %v", got)
	}
	items[2].origin = [2]float64{30, 80}
	if _, err := mapGlyphs(items, painted, box, 6); err == nil {
		t.Fatal("a glyph away from the layout's position was accepted")
	}
}

func TestInterpreterKnowsWhichAdvancesPlaceAGlyph(t *testing.T) {
	file := &pdfFile{objects: map[string]any{"obj:5 0 R": map[string]any{"value": map[string]any{"/Subtype": "/Type1"}}}}
	resources := map[string]any{"/Font": map[string]any{"/F1": "5 0 R"}}
	operations, err := parseContent([]byte("BT /F1 10 Tf 1 0 0 1 10 20 Tm (AB) Tj 0 -12 Td (C) Tj [(D) 50] TJ (E) Tj " +
		"1 0 0 1 0 0 Tm (F) Tj (G) ' ET BT (H) Tj ET"))
	if err != nil {
		t.Fatal(err)
	}
	in := newInterpreter(file, resources)
	if _, err := in.run(operations); err != nil {
		t.Fatal(err)
	}
	// A places B, C places D (no new position between them), D places E across its kern; B, E and F
	// are followed by a new text position (Td, Tm, ' and BT), G and H by nothing.
	used, advanced := in.codes["5 0 R"], in.advanced["5 0 R"]
	if len(used) != 8 || len(advanced) != 3 || !advanced['A'] || !advanced['C'] || !advanced['D'] {
		t.Fatalf("used %v, advanced %v", used, advanced)
	}
}

// mapGlyphs builds the page's glyph mapping for glyphs painted once each.
func mapGlyphs(items []item, painted []paintedGlyph, box [4]float64, total int) (*glyphMapper, error) {
	return mapPaints(items, painted, pageFrame{box: box}, identityPaints(total))
}
