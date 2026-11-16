package pdffonts

import (
	"math"
	"testing"
)

// A figure's Tahoma without a program, embedded as the Helvetica (Nimbus Sans) MuPDF draws it with:
// MuPDF stretched its A from the program's 667 to the Widths' 700, so the code is reported to be
// scaled back by 700/667 (decision 148); a font the PDF embedded itself is drawn at its own advance
// and never stretched.
func TestSubstitutedGlyphsReportTheirStretch(t *testing.T) {
	program := loadStandard(t, "Helvetica")
	store := newMemoryStore()
	store.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 32.0, "/FontName": "/Tahoma"}
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/TrueType", "/BaseFont": "/Tahoma",
		"/Encoding": "/WinAnsiEncoding", "/FontDescriptor": "2 0 R", "/FirstChar": 65.0, "/LastChar": 66.0,
		"/Widths": []any{700.0, 667.0}}
	fixer := NewFixer(store, testTable())
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{65: true, 66: true}, Advanced: map[int]bool{65: true}, Compensated: true})
	s, ok := fixer.Stretched["1 0 R"][65]
	if !ok || len(fixer.Stretched["1 0 R"]) != 1 || s.Old != 700 || s.New != 667 || s.Own != 667 ||
		math.Abs(s.Factor-700.0/667) > 1e-12 || fixer.Report.Stretched != 1 {
		t.Fatalf("stretched %v, report %+v", fixer.Stretched, fixer.Report)
	}
	embedded := newMemoryStore()
	streamDict, data := fontFileStream(program)
	embedded.objects["3 0 R"], embedded.data["3 0 R"] = streamDict, data
	embedded.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 32.0, "/FontFile": "3 0 R"}
	embedded.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type1", "/BaseFont": "/ABCDEF+NimbusSans-Regular",
		"/FontDescriptor": "2 0 R", "/FirstChar": 65.0, "/LastChar": 65.0, "/Widths": []any{700.0}}
	own := NewFixer(embedded, testTable())
	own.Fix("1 0 R", Use{Codes: map[int]bool{65: true}, Advanced: map[int]bool{65: true}, Compensated: true})
	if len(own.Stretched) != 0 || own.Moved["1 0 R"][65] != -33 {
		t.Fatalf("an embedded font stretched %v, moved %v", own.Stretched, own.Moved)
	}
}

// A compensated use: a width a later glyph is placed after changes by whole units (MuPDF reads
// Widths as integers) beyond the tolerance, and the change is reported to be undone by a kern; one
// within it stays; Widths grow over the codes shown, a code outside them having the MissingWidth.
func TestCompensatedWidthsMoveByWholeUnits(t *testing.T) {
	program := loadStandard(t, "Helvetica")
	store := newMemoryStore()
	streamDict, data := fontFileStream(program)
	store.objects["3 0 R"], store.data["3 0 R"] = streamDict, data
	store.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 32.0, "/FontFile": "3 0 R", "/MissingWidth": 250.0}
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type1", "/BaseFont": "/ABCDEF+NimbusSans-Regular",
		"/FontDescriptor": "2 0 R", "/FirstChar": 65.0, "/LastChar": 66.0, "/Widths": []any{600.3, 666.6}}
	fixer := NewFixer(store, testTable())
	all := map[int]bool{65: true, 66: true, 67: true}
	fixer.Fix("1 0 R", Use{Codes: all, Advanced: all, Compensated: true})
	font := store.objects["1 0 R"].(map[string]any)
	widths := font["/Widths"].([]any)
	// A is 667 in the program: 600.3 + 67; B 666.6 stays (0.4 off); C (722) was the MissingWidth 250.
	if font["/LastChar"] != 67 || len(widths) != 3 || widths[0] != 667.3 || widths[1] != 666.6 || widths[2] != 722 {
		t.Fatalf("widths %v, last %v", widths, font["/LastChar"])
	}
	if moved := fixer.Moved["1 0 R"]; len(moved) != 2 || moved[65] != 67 || moved[67] != 472 {
		t.Fatalf("moved %v", fixer.Moved)
	}
}

// Word's Type 3 Times New Roman: glyphs 2048 units wide (d1), Widths written in thousandths; the
// Widths become the glyphs' own, the kern in thousandths of the font size through the FontMatrix.
func TestType3WidthsAreTheGlyphProcedures(t *testing.T) {
	store := newMemoryStore()
	store.data["4 0 R"] = []byte("983 0 -1019 -628 2730 2096 d1\n/ExtGState1 gs\n")
	store.data["5 0 R"] = []byte{}
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type3", "/FirstChar": 1.0, "/LastChar": 2.0,
		"/Widths": []any{479.0, 250.0}, "/FontMatrix": []any{0.000488, 0.0, 0.0, 0.000488, 0.0, 0.0},
		"/CharProcs": map[string]any{"/s961": "4 0 R", "/s32": "5 0 R"},
		"/Encoding":  map[string]any{"/Differences": []any{1.0, "/s961", "/s32"}}}
	fixer := NewFixer(store, testTable())
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{1: true, 2: true}, Advanced: map[int]bool{1: true}, Compensated: true})
	widths := store.objects["1 0 R"].(map[string]any)["/Widths"].([]any)
	if widths[0] != 983 || widths[1] != 0 {
		t.Fatalf("widths %v (983 from d1, 0 for the empty procedure)", widths)
	}
	if kern := fixer.Moved["1 0 R"][1]; kern < 245.9 || kern > 246.0 || len(fixer.Moved["1 0 R"]) != 1 {
		t.Fatalf("moved %v (504 units of 0.000488: 245.95 thousandths)", fixer.Moved)
	}
}

func TestSubstituteFontsAsMuPDFChoosesThem(t *testing.T) {
	cases := map[string]struct {
		flags float64
		want  string
	}{"Tahoma": {32, "Helvetica"}, "Tahoma-Bold": {32, "Helvetica-Bold"}, "Georgia": {34, "Times-Roman"},
		"Consolas": {33, "Courier"}, "Verdana": {96, "Helvetica-Oblique"}, "Wingdings": {4, ""}}
	for name, c := range cases {
		if got := SubstituteFont(name, map[string]any{"/Flags": c.flags}); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

// A composite font's map without a code the pages show (Mathematica's fonts, a Word Symbol subset)
// gets it as U+FFFD: veraPDF cannot read a code its map lacks.
func TestCompositeFontsMapEveryCodeShown(t *testing.T) {
	store := newMemoryStore()
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type0", "/ToUnicode": "2 0 R"}
	store.data["2 0 R"] = []byte("1 begincodespacerange <0000> <FFFF> endcodespacerange 1 beginbfchar <0002> <03B1> endbfchar")
	fixer := NewFixer(store, testTable())
	if !fixer.Fix("1 0 R", Use{Codes: map[int]bool{1: true, 2: true}}) {
		t.Fatal("the code shown stayed unmapped")
	}
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, _ := ReadCMap(string(data))
	if mapped[1] != replacement || mapped[2] != "03B1" || fixer.Report.Codes != 1 {
		t.Fatalf("mapped %v, report %+v", mapped, fixer.Report)
	}
}

// A figure's Helvetica without a program is a standard 14 font: MuPDF draws it with its built-in
// Nimbus Sans at the program's own advance, never stretched to the Widths entry (papers/0115's
// Mathematica figures: Widths 625 against 556), so the width change is undone by a kern alone and
// the glyph is not scaled back (a Tz would put the tagged page a few pixels off the ordinary one).
func TestStandardFontGlyphsAreNotStretched(t *testing.T) {
	if (&programs{}).load("Helvetica") == nil {
		t.Skip("no URW standard-font program (TeX Live or urw-base35) for Helvetica")
	}
	store := newMemoryStore()
	store.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 32.0, "/FontName": "/Helvetica"}
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/TrueType", "/BaseFont": "/Helvetica",
		"/Encoding": "/WinAnsiEncoding", "/FontDescriptor": "2 0 R", "/FirstChar": 65.0, "/LastChar": 66.0,
		"/Widths": []any{800.0, 722.0}}
	fixer := NewFixer(store, testTable())
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{65: true, 66: true}, Advanced: map[int]bool{65: true}, Compensated: true})
	if len(fixer.Stretched) != 0 || fixer.Report.Stretched != 0 || fixer.Moved["1 0 R"][65] == 0 {
		t.Fatalf("a standard font stretched %v, moved %v", fixer.Stretched, fixer.Moved)
	}
}
